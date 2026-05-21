package db

import (
	"errors"
	"strconv"
	"strings"

	"zenmind/internal/config"

	"gorm.io/gorm"
)

const settingKeySyncInterval = "sync_interval_minutes"
const settingKeyZentaoBaseURL = "zentao_base_url"
const settingKeyZentaoLoginURL = "zentao_login_url"
const settingKeyDailyStandardHours = "daily_standard_hours"

// EnsureAppSettings creates app_settings if missing and seeds default sync interval.
func EnsureAppSettings() error {
	if err := PG.Exec(`
CREATE TABLE IF NOT EXISTS app_settings (
  setting_key TEXT PRIMARY KEY,
  value       TEXT NOT NULL,
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`).Error; err != nil {
		return err
	}
	defSync := strconv.Itoa(config.ClampSyncIntervalMinutes(config.Global.SyncIntervalMinutes))
	if err := PG.Exec(`
INSERT INTO app_settings (setting_key, value) VALUES (?, ?)
ON CONFLICT (setting_key) DO NOTHING`, settingKeySyncInterval, defSync).Error; err != nil {
		return err
	}

	defBaseURL := strings.TrimSpace(config.Global.ZentaoBaseURL)
	if defBaseURL == "" {
		defBaseURL = "http://localhost"
	}
	if err := PG.Exec(`
INSERT INTO app_settings (setting_key, value) VALUES (?, ?)
ON CONFLICT (setting_key) DO NOTHING`, settingKeyZentaoBaseURL, defBaseURL).Error; err != nil {
		return err
	}

	defLoginURL := strings.TrimSpace(config.Global.ZentaoLoginURL)
	if defLoginURL == "" {
		defLoginURL = strings.TrimRight(defBaseURL, "/") + "/user-login.html"
	}
	if err := PG.Exec(`
INSERT INTO app_settings (setting_key, value) VALUES (?, ?)
ON CONFLICT (setting_key) DO NOTHING`, settingKeyZentaoLoginURL, defLoginURL).Error; err != nil {
		return err
	}

	// Default daily standard hours = 8
	if err := PG.Exec(`
INSERT INTO app_settings (setting_key, value) VALUES (?, ?)
ON CONFLICT (setting_key) DO NOTHING`, settingKeyDailyStandardHours, "8").Error; err != nil {
		return err
	}

	return nil
}

// GetSyncIntervalMinutes reads persisted interval or falls back to config.Global (env default).
func GetSyncIntervalMinutes() int {
	var row struct {
		Value string `gorm:"column:value"`
	}
	err := PG.Table("app_settings").Select("value").Where("setting_key = ?", settingKeySyncInterval).Scan(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return config.ClampSyncIntervalMinutes(config.Global.SyncIntervalMinutes)
	}
	if strings.TrimSpace(row.Value) == "" {
		return config.ClampSyncIntervalMinutes(config.Global.SyncIntervalMinutes)
	}
	n, err := strconv.Atoi(strings.TrimSpace(row.Value))
	if err != nil {
		return config.ClampSyncIntervalMinutes(config.Global.SyncIntervalMinutes)
	}
	return config.ClampSyncIntervalMinutes(n)
}

// SetSyncIntervalMinutes persists the interval (minutes).
func SetSyncIntervalMinutes(n int) error {
	n = config.ClampSyncIntervalMinutes(n)
	return PG.Exec(`
INSERT INTO app_settings (setting_key, value, updated_at) VALUES (?, ?, NOW())
ON CONFLICT (setting_key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		settingKeySyncInterval, strconv.Itoa(n)).Error
}

// GetZentaoBaseURL reads persisted base url or falls back to env default.
func GetZentaoBaseURL() string {
	var row struct {
		Value string `gorm:"column:value"`
	}
	err := PG.Table("app_settings").Select("value").Where("setting_key = ?", settingKeyZentaoBaseURL).Scan(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return strings.TrimSpace(config.Global.ZentaoBaseURL)
	}
	return strings.TrimSpace(row.Value)
}

// SetZentaoBaseURL persists Zentao base url.
func SetZentaoBaseURL(s string) error {
	s = strings.TrimSpace(s)
	return PG.Exec(`
INSERT INTO app_settings (setting_key, value, updated_at) VALUES (?, ?, NOW())
ON CONFLICT (setting_key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		settingKeyZentaoBaseURL, s).Error
}

// GetZentaoLoginURL reads persisted login url or falls back to env default.
func GetZentaoLoginURL() string {
	var row struct {
		Value string `gorm:"column:value"`
	}
	err := PG.Table("app_settings").Select("value").Where("setting_key = ?", settingKeyZentaoLoginURL).Scan(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return strings.TrimSpace(config.Global.ZentaoLoginURL)
	}
	return strings.TrimSpace(row.Value)
}

// SetZentaoLoginURL persists Zentao login url.
func SetZentaoLoginURL(s string) error {
	s = strings.TrimSpace(s)
	return PG.Exec(`
INSERT INTO app_settings (setting_key, value, updated_at) VALUES (?, ?, NOW())
ON CONFLICT (setting_key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		settingKeyZentaoLoginURL, s).Error
}

// GetDailyStandardHours reads persisted daily standard hours or falls back to 8.
func GetDailyStandardHours() int {
	var row struct {
		Value string `gorm:"column:value"`
	}
	err := PG.Table("app_settings").Select("value").Where("setting_key = ?", settingKeyDailyStandardHours).Scan(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return 8
	}
	if strings.TrimSpace(row.Value) == "" {
		return 8
	}
	n, err := strconv.Atoi(strings.TrimSpace(row.Value))
	if err != nil || n < 1 || n > 24 {
		return 8
	}
	return n
}

// SetDailyStandardHours persists daily standard hours.
func SetDailyStandardHours(n int) error {
	if n < 1 {
		n = 1
	}
	if n > 24 {
		n = 24
	}
	return PG.Exec(`
INSERT INTO app_settings (setting_key, value, updated_at) VALUES (?, ?, NOW())
ON CONFLICT (setting_key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		settingKeyDailyStandardHours, strconv.Itoa(n)).Error
}
