package campus

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Gradient-Clipping/lazycampus-platform/common"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestSchoolAndAdministratorAllowlist(t *testing.T) {
	for _, tc := range []struct {
		name    string
		claims  Claims
		allowed bool
		role    string
	}{
		{"ordinary local account", Claims{Subject: "local", Username: "visitor"}, false, ""},
		{"forged school attribute", Claims{Subject: "local", IdentityID: "some-account", StudentNumber: "123"}, false, ""},
		{"school", Claims{Subject: "campus", IdentityID: "11111111-2222-4333-8444-555555555555", StudentNumber: "20260001"}, true, "user"},
		{"admin without role", Claims{Subject: "admin", Username: "ystemsrx"}, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u, err := AuthorizedIdentity(tc.claims)
			if tc.allowed {
				require.NoError(t, err)
				assert.Equal(t, tc.role, u.Role)
				assert.EqualValues(t, 1000, u.DailyQuota)
				assert.EqualValues(t, 20, u.RateLimit)
			} else {
				assert.Error(t, err)
			}
		})
	}
	admin := Claims{Subject: "admin", Username: "ystemsrx"}
	admin.RealmAccess.Roles = []string{"platform-admin"}
	u, err := AuthorizedIdentity(admin)
	require.NoError(t, err)
	assert.Equal(t, "admin", u.Role)
	admin.Username = "another-admin"
	_, err = AuthorizedIdentity(admin)
	assert.Error(t, err)
}

func TestDatabaseQuotaAdmissionAndIdempotentMigration(t *testing.T) {
	db := testDatabase(t)
	var err error
	require.NoError(t, Migrate(db))
	require.NoError(t, Migrate(db))
	u := User{Subject: randomKey(20), Username: "quota-test", Enabled: true, DailyQuota: 3, RateLimit: 60}
	require.NoError(t, db.Create(&u).Error)
	tokens := []Token{{UserID: u.ID, KeyHash: digest(randomKey(32)), Scopes: "timetable:read", Enabled: true, Quota: 100, DailyQuota: 100, ExpiresAt: timePointer(time.Now().Add(time.Hour))}, {UserID: u.ID, KeyHash: digest(randomKey(32)), Scopes: "timetable:read", Enabled: true, Quota: 100, DailyQuota: 100, ExpiresAt: timePointer(time.Now().Add(time.Hour))}}
	require.NoError(t, db.Create(&tokens).Error)
	// An existing populated database must survive startup and retain uniqueness.
	require.NoError(t, Migrate(db))
	var stored User
	require.NoError(t, db.First(&stored, u.ID).Error)
	assert.Equal(t, u.Subject, stored.Subject)
	duplicate := User{Subject: u.Subject}
	assert.Error(t, db.Create(&duplicate).Error)
	app := &App{DB: db}
	results := make(chan error, 8)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			_, err := app.reserve(context.Background(), u.ID, tokens[i%2].ID, &Log{UserID: u.ID, TokenID: tokens[i%2].ID, RequestID: randomKey(12), Endpoint: "/teaching/timetable"})
			results <- err
		})
	}
	wg.Wait()
	close(results)
	accepted := 0
	for err := range results {
		if err == nil {
			accepted++
		} else {
			assert.ErrorIs(t, err, errQuota)
		}
	}
	assert.Equal(t, 3, accepted, "creating another key cannot increase the account quota")
	var daily DailyUsage
	require.NoError(t, db.Where("bucket = ? AND day = ?", fmt.Sprintf("user:%d", u.ID), time.Now().UTC().Format("2006-01-02")).First(&daily).Error)
	assert.EqualValues(t, 3, daily.Requests)
	var logs int64
	require.NoError(t, db.Model(&Log{}).Where("user_id = ?", u.ID).Count(&logs).Error)
	assert.EqualValues(t, 3, logs)
	// A lower application quota must roll back the account counter too.
	require.NoError(t, db.Model(&u).Update("daily_quota", 100).Error)
	require.NoError(t, db.Model(&tokens[0]).Updates(map[string]any{"quota": 1, "used_quota": 1}).Error)
	_, err = app.reserve(context.Background(), u.ID, tokens[0].ID, &Log{UserID: u.ID})
	assert.ErrorIs(t, err, errQuota)
	require.NoError(t, db.Where("bucket = ? AND day = ?", daily.Bucket, daily.Day).First(&daily).Error)
	assert.EqualValues(t, 3, daily.Requests)
	require.NoError(t, db.Model(&tokens[1]).Update("enabled", false).Error)
	_, err = app.reserve(context.Background(), u.ID, tokens[1].ID, &Log{UserID: u.ID})
	assert.ErrorIs(t, err, errDisabled)
}

func testApp(t *testing.T) (*App, User, string) {
	t.Helper()
	redisURL := os.Getenv("PLATFORM_TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("PLATFORM_TEST_REDIS_URL is required for HTTP integration tests")
	}
	db := testDatabase(t)
	require.NoError(t, Migrate(db))
	options, err := redis.ParseURL(redisURL)
	require.NoError(t, err)
	r := redis.NewClient(options)
	require.NoError(t, r.Ping(context.Background()).Err())
	app := &App{DB: db, Redis: r, HTTP: &http.Client{Timeout: time.Second}, Config: Config{Origin: "https://platform.example", CampusSecret: strings.Repeat("s", 32)}}
	t.Cleanup(app.Close)
	u := User{ID: uint64(time.Now().UnixMicro()), Subject: randomKey(20), IdentityID: "11111111-2222-4333-8444-555555555555", Username: "test-user", Role: "user", Enabled: true, DailyQuota: 5000, RateLimit: 60}
	require.NoError(t, db.Create(&u).Error)
	s := browserSession{UserID: u.ID, Subject: u.Subject, CSRF: randomKey(32), ExpiresAt: timePointer(time.Now().Add(time.Hour)).Unix(), RefreshAt: time.Now().Add(time.Hour).Unix()}
	id := randomKey(32)
	data, err := common.Marshal(s)
	require.NoError(t, err)
	require.NoError(t, r.Set(context.Background(), "platform:v1:session:"+digest(id), data, time.Hour).Err())
	return app, u, id
}

func perform(app *App, method, path, cookie, csrf, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = "198.51.100.11:1234"
	r.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	}
	if csrf != "" {
		r.Header.Set("X-CSRF-Token", csrf)
		r.Header.Set("Origin", app.Config.Origin)
	}
	w := httptest.NewRecorder()
	app.Router().ServeHTTP(w, r)
	return w
}

func TestConsoleIsolationCSRFKeyRotationAndDisable(t *testing.T) {
	a, u, cookie := testApp(t)
	w := perform(a, "GET", "/api/me", cookie, "", "")
	require.Equal(t, 200, w.Code)
	var me struct {
		Data struct {
			CSRF string `json:"csrf_token"`
		}
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &me))
	body := `{"name":"test script","scopes":["timetable:read"],"daily_quota":100,"rate_limit":10,"quota":1000,"expires_in_days":30}`
	assert.Equal(t, 403, perform(a, "POST", "/api/tokens", cookie, "", body).Code)
	assert.Equal(t, 401, perform(a, "GET", "/api/tokens", "", "", "").Code)
	assert.Equal(t, 403, perform(a, "GET", "/api/admin/users", cookie, "", "").Code)
	response := perform(a, "POST", "/api/tokens", cookie, me.Data.CSRF, body)
	require.Equal(t, 200, response.Code, response.Body.String())
	var created struct {
		Data struct {
			Token Token
			Key   string
		}
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &created))
	assert.Len(t, created.Data.Key, 46)
	list := perform(a, "GET", "/api/tokens", cookie, "", "")
	assert.NotContains(t, list.Body.String(), created.Data.Key)
	var token Token
	require.NoError(t, a.DB.First(&token, created.Data.Token.ID).Error)
	assert.Equal(t, digest(created.Data.Key), token.KeyHash)
	other := User{Subject: randomKey(20), Enabled: true}
	require.NoError(t, a.DB.Create(&other).Error)
	foreign := Token{UserID: other.ID, KeyHash: digest(randomKey(32)), Enabled: true, Quota: 10, ExpiresAt: timePointer(time.Now().Add(time.Hour))}
	require.NoError(t, a.DB.Create(&foreign).Error)
	assert.Equal(t, 404, perform(a, "DELETE", fmt.Sprintf("/api/tokens/%d", foreign.ID), cookie, me.Data.CSRF, "").Code)
	rotated := perform(a, "POST", fmt.Sprintf("/api/tokens/%d/rotate", token.ID), cookie, me.Data.CSRF, "")
	require.Equal(t, 200, rotated.Code)
	require.NoError(t, a.DB.First(&token, token.ID).Error)
	assert.NotEqual(t, digest(created.Data.Key), token.KeyHash)
	require.NoError(t, a.DB.Model(&u).Update("enabled", false).Error)
	assert.Equal(t, 403, perform(a, "GET", "/api/tokens", cookie, "", "").Code)
	for _, path := range []string{"/api/user/login", "/api/user/register", "/api/topup", "/v1/chat/completions"} {
		assert.Equal(t, 404, perform(a, "POST", path, "", "", "{}").Code)
	}
}

func TestGatewayEnforcesScopeQuotaIdentityAndSignedPath(t *testing.T) {
	a, u, _ := testApp(t)
	key := "lc_" + randomKey(32)
	token := Token{UserID: u.ID, KeyHash: digest(key), Scopes: "timetable:read", Enabled: true, RateLimit: 30, DailyQuota: 2, Quota: 20, ExpiresAt: timePointer(time.Now().Add(time.Hour))}
	require.NoError(t, a.DB.Create(&token).Error)
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		assert.Empty(t, r.Header.Get("Authorization"))
		assert.Equal(t, u.IdentityID, r.Header.Get("X-Platform-Identity"))
		canonical := strings.Join([]string{"GET", r.URL.RequestURI(), r.Header.Get("X-Platform-Timestamp"), r.Header.Get("X-Platform-Nonce"), u.IdentityID, digest("")}, "\n")
		mac := hmac.New(sha256.New, []byte(a.Config.CampusSecret))
		mac.Write([]byte(canonical))
		assert.Equal(t, hex.EncodeToString(mac.Sum(nil)), r.Header.Get("X-Platform-Signature"))
		assert.Equal(t, "/internal/platform/v1/teaching/timetable?semester=2026-1", r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"data":{"courses":[]}}`)
	}))
	defer upstream.Close()
	a.Config.CampusURL = upstream.URL
	call := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.RemoteAddr = "198.51.100.1:12"
		r.Header.Set("Authorization", "Bearer "+key)
		w := httptest.NewRecorder()
		a.Router().ServeHTTP(w, r)
		return w
	}
	assert.Equal(t, 403, call("/v1/teaching/grades").Code)
	assert.Equal(t, 400, call("/v1/teaching/timetable?user_id=another-user").Code)
	assert.Equal(t, 400, call("/v1/teaching/timetable?refresh=true").Code)
	assert.Equal(t, 404, call("/v1/../api/admin/users").Code)
	assert.Equal(t, 200, call("/v1/teaching/timetable?semester=2026-1").Code)
	assert.Equal(t, 200, call("/v1/teaching/timetable?semester=2026-1").Code)
	limited := call("/v1/teaching/timetable?semester=2026-1")
	assert.Equal(t, 429, limited.Code)
	assert.NotEmpty(t, limited.Header().Get("Retry-After"))
	assert.Equal(t, 2, calls)
	require.NoError(t, a.DB.Model(&token).Update("enabled", false).Error)
	assert.Equal(t, 401, call("/v1/teaching/timetable").Code)
}

func TestDistributedRateAndConcurrencyLimits(t *testing.T) {
	a, u, _ := testApp(t)
	keys := []string{"platform:test:" + randomKey(20), "platform:test:" + randomKey(20)}
	first, err := rateScript.Run(context.Background(), a.Redis, keys, 2, 60, 1, 60).Int64Slice()
	require.NoError(t, err)
	assert.EqualValues(t, 1, first[0])
	second, err := rateScript.Run(context.Background(), a.Redis, keys, 2, 60, 1, 60).Int64Slice()
	require.NoError(t, err)
	assert.EqualValues(t, 0, second[0])
	assert.Positive(t, second[1])
	assert.Equal(t, "1", a.Redis.Get(context.Background(), keys[0]).Val(), "a rejected key must not consume the other bucket")
	concurrency := []string{fmt.Sprintf("platform:test:concurrent:%d", u.ID)}
	now := time.Now().UnixMilli()
	for i := range 3 {
		n, err := concurrentScript.Run(context.Background(), a.Redis, concurrency, now, now+95000, fmt.Sprint(i), 3).Int()
		require.NoError(t, err)
		assert.Equal(t, 1, n)
	}
	n, err := concurrentScript.Run(context.Background(), a.Redis, concurrency, now, now+95000, "fourth", 3).Int()
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.True(t, allowedIP("192.0.2.0/24", "192.0.2.7"))
	assert.False(t, allowedIP("192.0.2.0/24", "198.51.100.7"))
}

func TestSSOStateBindingPKCEIdentityAndReplay(t *testing.T) {
	a, _, _ := testApp(t)
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwk := jose.JSONWebKey{Key: &private.PublicKey, KeyID: "test-key", Algorithm: "RS256", Use: "sig"}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: private}, (&jose.SignerOptions{}).WithHeader("kid", "test-key"))
	require.NoError(t, err)
	var issuer, nonce, challenge string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			data, _ := common.Marshal(map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
			w.Write(data)
		case "/keys":
			data, _ := common.Marshal(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{jwk}})
			w.Write(data)
		case "/token":
			require.NoError(t, r.ParseForm())
			assert.Equal(t, challenge, oauth2.S256ChallengeFromVerifier(r.Form.Get("code_verifier")))
			claims := map[string]any{"iss": issuer, "aud": "platform-test", "sub": "oidc-test-user", "iat": time.Now().Unix(), "exp": time.Now().Add(5 * time.Minute).Unix(), "nonce": nonce, "preferred_username": "school-user", "identity_id": "11111111-2222-4333-8444-555555555555", "student_number": "20260001"}
			data, _ := common.Marshal(claims)
			signed, signErr := signer.Sign(data)
			require.NoError(t, signErr)
			compact, compactErr := signed.CompactSerialize()
			require.NoError(t, compactErr)
			body, _ := common.Marshal(map[string]any{"id_token": compact, "access_token": "test-access", "refresh_token": "test-refresh", "token_type": "Bearer", "expires_in": 300})
			w.Write(body)
		default:
			w.WriteHeader(404)
		}
	}))
	defer provider.Close()
	issuer = provider.URL
	a.Config.Issuer = issuer
	a.Config.ClientID = "platform-test"
	a.Provider, err = oidc.NewProvider(context.Background(), issuer)
	require.NoError(t, err)
	a.OAuth = oauth2.Config{ClientID: a.Config.ClientID, ClientSecret: strings.Repeat("x", 32), RedirectURL: a.Config.Origin + "/auth/callback", Endpoint: a.Provider.Endpoint(), Scopes: []string{"openid", "profile"}}
	login := perform(a, "GET", "/auth/start", "", "", "")
	require.Equal(t, 302, login.Code)
	target, err := url.Parse(login.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "none", target.Query().Get("prompt"))
	assert.Equal(t, "S256", target.Query().Get("code_challenge_method"))
	nonce = target.Query().Get("nonce")
	challenge = target.Query().Get("code_challenge")
	state := target.Query().Get("state")
	var browser string
	for _, cookie := range login.Result().Cookies() {
		if cookie.Name == flowCookie {
			browser = cookie.Value
		}
	}
	callback := func(binding string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/auth/callback?state="+url.QueryEscape(state)+"&code=test-code", nil)
		r.AddCookie(&http.Cookie{Name: flowCookie, Value: binding})
		w := httptest.NewRecorder()
		a.Router().ServeHTTP(w, r)
		return w
	}
	wrong := callback(randomKey(32))
	assert.Contains(t, wrong.Header().Get("Location"), "auth-error")
	valid := callback(browser)
	assert.Equal(t, "/", valid.Header().Get("Location"))
	assert.Contains(t, strings.Join(valid.Header().Values("Set-Cookie"), "\n"), sessionCookie)
	replay := callback(browser)
	assert.Contains(t, replay.Header().Get("Location"), "auth-error")
	var school User
	require.NoError(t, a.DB.Where("subject = ?", "oidc-test-user").First(&school).Error)
	assert.Equal(t, "user", school.Role)
}
