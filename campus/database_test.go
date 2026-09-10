package campus

import (
	"database/sql"
	"os"
	"testing"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Each test gets a real, isolated MySQL schema. Only the disposable test DSN is accepted.
func testDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("PLATFORM_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("PLATFORM_TEST_MYSQL_DSN is required for database integration tests")
	}
	config, err := mysqlDriver.ParseDSN(dsn)
	require.NoError(t, err)
	require.Equal(t, "platform_test", config.DBName, "refusing to use a non-test database")
	admin, err := sql.Open("mysql", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { admin.Close() })
	name := "platform_test_" + digest(randomKey(16))[:16]
	_, err = admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := admin.Exec("DROP DATABASE `" + name + "`")
		if err != nil {
			t.Errorf("test database cleanup: %v", err)
		}
	})
	config.DBName = name
	db, err := OpenDatabase(config.FormatDSN())
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })
	return db
}

func timePointer(value time.Time) *time.Time { return &value }
