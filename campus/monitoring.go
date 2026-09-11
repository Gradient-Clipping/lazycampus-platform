package campus

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"sort"
	"time"
)

func monitoringTraffic(rows []Log) gin.H {
	if len(rows) >= 20000 {
		return gin.H{"status": "no_data"}
	}
	result := gin.H{"status": "no_data", "requests": 0, "errors": 0, "limited": 0, "windowSeconds": 300}
	durations := []int64{}
	failures, limited := 0, 0
	for _, row := range rows {
		if row.Status == 429 {
			limited++
		}
		if row.Status >= 400 && row.Status < 500 {
			continue
		}
		if row.ErrorCode == "ENDPOINT_UNAVAILABLE" {
			continue
		}
		durations = append(durations, row.DurationMS)
		if row.Status >= 500 {
			failures++
		}
	}
	result["limited"] = limited
	result["requests"], result["errors"] = len(durations), failures
	if len(durations) == 0 {
		return result
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p95 := durations[(len(durations)*95+99)/100-1]
	ratio := float64(failures) / float64(len(durations))
	result["p95Ms"], result["successPercentage"], result["status"] = p95, 100*(1-ratio), "operational"
	significant := len(durations) >= 20 || (failures >= 3 && failures == len(durations))
	if significant && ratio >= 0.5 {
		result["status"] = "partial_outage"
	} else if significant && (ratio >= 0.05 || p95 >= 5000) {
		result["status"] = "degraded_performance"
	}
	return result
}

// The anonymous readiness request never authenticates a campus user or spends quota.
func monitoringCampusReady(ctx context.Context, client *http.Client, baseURL string) bool {
	ctx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/system/ready", nil)
	if err != nil {
		return false
	}
	readonly := *client
	readonly.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	res, err := readonly.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	return res.StatusCode == http.StatusOK && json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&body) == nil && body.Success && body.Data.Status == "ready"
}

func monitoringWithHealth(traffic gin.H, health string) gin.H {
	traffic["healthStatus"] = health
	if health != "operational" {
		traffic["status"] = health
	} else if traffic["requests"] == 0 {
		traffic["status"] = "operational"
	}
	return traffic
}

func (a *App) monitoringState(c *gin.Context) {
	want := sha256.Sum256([]byte("Bearer " + a.Config.StatusMonitorToken))
	got := sha256.Sum256([]byte(c.GetHeader("Authorization")))
	if len(a.Config.StatusMonitorToken) < 32 || subtle.ConstantTimeCompare(want[:], got[:]) != 1 {
		c.Status(404)
		return
	}
	ctx, done := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer done()
	c.Header("Cache-Control", "no-store")
	ready := make(chan bool, 1)
	go func() { ready <- monitoringCampusReady(ctx, a.HTTP, a.Config.CampusURL) }()
	db := a.DB.WithContext(ctx)
	var value int
	authStatus := "operational"
	if db.Raw("SELECT 1 FROM platform_endpoints LIMIT 1").Scan(&value).Error != nil || value != 1 || a.Redis.Ping(ctx).Err() != nil {
		authStatus = "full_outage"
	}
	rows := []Log{}
	queryErr := db.Select("status,duration_ms,error_code").Where("created_at >= ?", time.Now().UTC().Add(-5*time.Minute)).Order("created_at DESC").Limit(20000).Find(&rows).Error
	traffic := monitoringTraffic(rows)
	if queryErr != nil {
		traffic = gin.H{"status": "no_data"}
	}
	mailStatus := "operational"
	var broken int64
	err := db.Model(&Notification{}).Where("(email_status IN ? AND next_attempt_at < ?) OR (email_status IN ? AND created_at > ?)", []string{"queued", "retry", "sending"}, time.Now().UTC().Add(-10*time.Minute), []string{"failed", "uncertain"}, time.Now().UTC().Add(-24*time.Hour)).Count(&broken).Error
	if !a.emailAvailable() || err != nil || a.monitorWorkerAt.Load() == 0 {
		mailStatus = "no_data"
	} else if broken > 0 || time.Since(time.Unix(a.monitorWorkerAt.Load(), 0)) > 6*time.Minute {
		mailStatus = "degraded_performance"
	}
	health := authStatus
	if !<-ready && health == "operational" {
		health = "partial_outage"
	}
	traffic = monitoringWithHealth(traffic, health)
	c.JSON(200, gin.H{"version": 1, "observedAt": time.Now().UTC().Format(time.RFC3339Nano), "components": gin.H{
		"authorization": gin.H{"status": authStatus}, "campus": traffic, "mail": gin.H{"status": mailStatus},
	}})
}
