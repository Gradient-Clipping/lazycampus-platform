package campus

import (
	"fmt"
	"testing"
	"time"

	"github.com/Gradient-Clipping/lazycampus-platform/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultLimitMigrationPreservesCustomSettingsAndLegacyNotice(t *testing.T) {
	db := testDatabase(t)
	pool, _ := db.DB()
	t.Cleanup(func() { pool.Close() })
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &Setting{}))
	users := []User{{Subject: "default-user", RateLimit: 60}, {Subject: "custom-user", RateLimit: 15}}
	require.NoError(t, db.Create(&users).Error)
	tokens := []Token{{UserID: users[0].ID, KeyHash: "default-key", RateLimit: 30}, {UserID: users[0].ID, KeyHash: "lower-key", RateLimit: 10}}
	require.NoError(t, db.Create(&tokens).Error)
	when := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, db.Create(&Setting{Name: "notice", Value: "Existing announcement", UpdatedAt: when}).Error)
	require.NoError(t, Migrate(db))
	var got User
	require.NoError(t, db.First(&got, users[0].ID).Error)
	assert.Equal(t, int64(20), got.RateLimit)
	got = User{}
	require.NoError(t, db.First(&got, users[1].ID).Error)
	assert.Equal(t, int64(15), got.RateLimit)
	var updated []Token
	require.NoError(t, db.Order("id").Find(&updated).Error)
	assert.Equal(t, int64(20), updated[0].RateLimit)
	assert.Equal(t, int64(10), updated[1].RateLimit)
	// A later deliberate administrator override must survive every subsequent start.
	require.NoError(t, db.Model(&users[0]).Update("rate_limit", 60).Error)
	require.NoError(t, Migrate(db))
	got = User{}
	require.NoError(t, db.First(&got, users[0].ID).Error)
	assert.Equal(t, int64(60), got.RateLimit)
	var notices []Announcement
	require.NoError(t, db.Find(&notices).Error)
	require.Len(t, notices, 1)
	assert.Equal(t, "Existing announcement", notices[0].Content)
	assert.True(t, when.Equal(notices[0].CreatedAt))
	identity, err := AuthorizedIdentity(Claims{Subject: "school", IdentityID: "11111111-2222-4333-8444-555555555555", StudentNumber: "test"})
	require.NoError(t, err)
	assert.Equal(t, int64(20), identity.RateLimit)
}

func TestAnnouncementPublishingPermissionsHistoryAndWithdrawal(t *testing.T) {
	a, user, cookie := testApp(t)
	me := perform(a, "GET", "/api/me", cookie, "", "")
	var session struct {
		Data struct {
			CSRF string `json:"csrf_token"`
		}
	}
	require.NoError(t, common.Unmarshal(me.Body.Bytes(), &session))
	csrf := session.Data.CSRF
	assert.Equal(t, 200, perform(a, "GET", "/api/announcements", "", "", "").Code)
	assert.Equal(t, 401, perform(a, "PUT", "/api/admin/notice", "", "", `{"content":"unauthorized"}`).Code)
	assert.Equal(t, 403, perform(a, "PUT", "/api/admin/notice", cookie, csrf, `{"content":"school user"}`).Code)
	require.NoError(t, a.DB.Model(&user).Update("role", "admin").Error)
	assert.Equal(t, 403, perform(a, "PUT", "/api/admin/notice", cookie, "wrong", `{"content":"csrf"}`).Code)
	assert.Equal(t, 400, perform(a, "PUT", "/api/admin/notice", cookie, csrf, `{"content":"  "}`).Code)
	assert.Equal(t, 200, perform(a, "PUT", "/api/admin/notice", cookie, csrf, `{"title":"First","content":"First content"}`).Code)
	assert.Equal(t, 200, perform(a, "PUT", "/api/admin/notice", cookie, csrf, `{"title":"Second","content":"<script>alert(1)</script>"}`).Code)
	response := perform(a, "GET", "/api/announcements", "", "", "")
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	var feed struct {
		Data struct {
			Items []Announcement `json:"items"`
		}
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &feed))
	require.Len(t, feed.Data.Items, 2)
	assert.Equal(t, "Second", feed.Data.Items[0].Title)
	assert.NotContains(t, response.Body.String(), "actor_id")
	assert.Equal(t, 400, perform(a, "DELETE", "/api/admin/announcements/1%20OR%201=1", cookie, csrf, "").Code)
	assert.Equal(t, 200, perform(a, "DELETE", fmt.Sprintf("/api/admin/announcements/%d", feed.Data.Items[0].ID), cookie, csrf, "").Code)
	legacy := perform(a, "GET", "/api/notice", cookie, "", "")
	assert.Contains(t, legacy.Body.String(), "First content")
	assert.NotContains(t, legacy.Body.String(), "script")
	var count int64
	require.NoError(t, a.DB.Model(&Audit{}).Where("action = ?", "notice.withdraw").Count(&count).Error)
	assert.Equal(t, int64(1), count)
}
