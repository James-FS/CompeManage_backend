package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old, err := os.Getwd()
	assert.NoError(t, err)
	assert.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(old) })
	return dir
}

func TestGetEnvironment(t *testing.T) {
	t.Setenv("ENV", "")
	t.Setenv("APP_ENV", "")
	assert.Equal(t, "dev", GetEnvironment())

	t.Setenv("APP_ENV", "development")
	assert.Equal(t, "dev", GetEnvironment())

	t.Setenv("APP_ENV", "production")
	assert.Equal(t, "prod", GetEnvironment())

	t.Setenv("APP_ENV", "staging")
	assert.Equal(t, "staging", GetEnvironment())

	t.Setenv("ENV", "test-env")
	assert.Equal(t, "test-env", GetEnvironment())
}

func TestGetConfigPath(t *testing.T) {
	dir := chdirTemp(t)
	got := GetConfigPath("config-dev.yaml")
	assert.Equal(t, filepath.Join(dir, "config", "config-dev.yaml"), got)
}

// Init：加载 config-dev.yaml 成功路径
func TestInit_LoadsDevConfig(t *testing.T) {
	dir := chdirTemp(t)
	assert.NoError(t, os.MkdirAll(filepath.Join(dir, "config"), 0o755))
	yaml := "database:\n  host: unit-test-host\njwt:\n  secret: unit-secret\nlogger:\n  level: info\n"
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "config", "config-dev.yaml"), []byte(yaml), 0o600))

	Init()

	assert.Equal(t, "unit-test-host", GetString("database.host"))
	assert.Equal(t, "unit-secret", GetString("jwt.secret"))
	assert.Equal(t, 3306, GetInt("database.port")) // 默认值
	assert.NotEmpty(t, AppConfig.Database.Host)
}

// Init：config-dev.yaml 缺失时回退 config.yaml
func TestInit_FallbackToDefaultConfig(t *testing.T) {
	dir := chdirTemp(t)
	assert.NoError(t, os.MkdirAll(filepath.Join(dir, "config"), 0o755))
	yaml := "database:\n  host: fallback-host\n"
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "config", "config.yaml"), []byte(yaml), 0o600))

	t.Setenv("ENV", "prod")
	defer t.Setenv("ENV", "")
	Init()

	assert.Equal(t, "fallback-host", GetString("database.host"))
}
