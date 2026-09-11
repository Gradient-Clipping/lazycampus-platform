package campus

import (
	"errors"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	Address, Origin, Issuer, ClientID, ClientSecret, RedisURL, DatabaseDSN string
	CampusURL, CampusSecret, Revision, StaticDir                           string
	SenderAPIKey, SenderFrom, SenderName                                   string
	SecureCookies                                                          bool
	StatusMonitorToken                                                     string
}

func LoadConfig() (Config, error) {
	c := Config{
		StatusMonitorToken: os.Getenv("STATUS_MONITOR_TOKEN"),
		Address:            env("LISTEN_ADDR", ":3000"), Origin: env("PLATFORM_ORIGIN", "https://platform.lazycampus.com"),
		Issuer:   env("OIDC_ISSUER", "https://auth.lazycampus.com/realms/lazycampus"),
		ClientID: env("OIDC_CLIENT_ID", "lazycampus-platform"), ClientSecret: os.Getenv("OIDC_CLIENT_SECRET"),
		RedisURL: os.Getenv("REDIS_URL"), DatabaseDSN: os.Getenv("DATABASE_DSN"),
		CampusURL: os.Getenv("CAMPUS_SERVICE_URL"), CampusSecret: os.Getenv("CAMPUS_SERVICE_SECRET"),
		SenderAPIKey: os.Getenv("SENDER_API_KEY"), SenderFrom: os.Getenv("SENDER_FROM_EMAIL"), SenderName: env("SENDER_FROM_NAME", "Lazy Campus"),
		Revision: env("APP_REVISION", "development"), StaticDir: env("STATIC_DIR", "web/dist"), SecureCookies: true,
	}
	if c.DatabaseDSN == "" {
		c.DatabaseDSN = os.Getenv("MYSQL_USER") + ":" + os.Getenv("MYSQL_PASSWORD") + "@tcp(" + env("MYSQL_HOST", "localhost") + ":" + env("MYSQL_PORT", "3306") + ")/" + os.Getenv("MYSQL_DATABASE") + "?charset=utf8mb4&parseTime=True&loc=UTC"
	}
	if len(c.ClientSecret) < 32 || len(c.CampusSecret) < 32 || c.RedisURL == "" || c.DatabaseDSN == "" {
		return c, errors.New("missing required runtime configuration")
	}
	if (c.SenderAPIKey == "") != (c.SenderFrom == "") || !validEmail(c.SenderFrom) || strings.ContainsAny(c.SenderName, "\r\n") {
		return c, errors.New("invalid Sender configuration")
	}
	for _, address := range []string{c.Origin, c.Issuer} {
		u, err := url.Parse(address)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
			return c, errors.New("identity and platform URLs must use HTTPS")
		}
	}
	u, err := url.Parse(c.CampusURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || strings.Trim(u.Path, "/") != "" {
		return c, errors.New("invalid campus service origin")
	}
	c.Origin = strings.TrimRight(c.Origin, "/")
	c.CampusURL = strings.TrimRight(c.CampusURL, "/")
	return c, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
