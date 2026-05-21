package db

import (
	"fmt"
	"strings"
	"time"
	"zenmind/internal/config"
	"zenmind/internal/crypto"
)

type UserCalendarFeed struct {
	ID           int64     `json:"id" gorm:"primaryKey;column:id"`
	SystemUserID int64     `json:"system_user_id" gorm:"column:system_user_id"`
	Name         string    `json:"name" gorm:"column:name"`
	FeedHost     string    `json:"feed_host" gorm:"column:feed_host"`
	ICalURLEnc   string    `json:"-" gorm:"column:ical_url_enc"`
	Color        string    `json:"color" gorm:"column:color"`
	CreatedAt    time.Time `json:"created_at" gorm:"column:created_at"`
	UpdatedAt    time.Time `json:"updated_at" gorm:"column:updated_at"`
}

func (UserCalendarFeed) TableName() string { return "user_calendar_feeds" }

func calendarURLSecret() string {
	return strings.TrimSpace(config.Global.JWTSecret)
}

const maxCalendarFeedsPerUser = 15

// ListUserCalendarFeeds returns feeds for a system user (without decrypted URL).
func ListUserCalendarFeeds(systemUserID int64) ([]UserCalendarFeed, error) {
	var rows []UserCalendarFeed
	err := PG.Where("system_user_id = ?", systemUserID).Order("id ASC").Find(&rows).Error
	return rows, err
}

// ListUserCalendarFeedsForFetch returns id, name, color and decrypted ical URL for sync.
func ListUserCalendarFeedsForFetch(systemUserID int64) ([]struct {
	ID      int64
	Name    string
	Color   string
	ICalURL string
}, error) {
	secret := calendarURLSecret()
	if secret == "" {
		return nil, fmt.Errorf("jwt secret not configured")
	}
	var rows []UserCalendarFeed
	if err := PG.Where("system_user_id = ?", systemUserID).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]struct {
		ID      int64
		Name    string
		Color   string
		ICalURL string
	}, 0, len(rows))
	for _, r := range rows {
		u, err := crypto.DecryptString(r.ICalURLEnc, secret)
		if err != nil {
			continue
		}
		out = append(out, struct {
			ID      int64
			Name    string
			Color   string
			ICalURL string
		}{ID: r.ID, Name: r.Name, Color: r.Color, ICalURL: u})
	}
	return out, nil
}

// InsertUserCalendarFeed stores an encrypted ICS URL.
func InsertUserCalendarFeed(systemUserID int64, name, feedHost, plainURL, color string) (int64, error) {
	var n int64
	if err := PG.Model(&UserCalendarFeed{}).Where("system_user_id = ?", systemUserID).Count(&n).Error; err != nil {
		return 0, err
	}
	if n >= maxCalendarFeedsPerUser {
		return 0, fmt.Errorf("calendar feed limit reached (%d)", maxCalendarFeedsPerUser)
	}
	enc, err := crypto.EncryptString(plainURL, calendarURLSecret())
	if err != nil {
		return 0, err
	}
	row := UserCalendarFeed{
		SystemUserID: systemUserID,
		Name:         name,
		FeedHost:     feedHost,
		ICalURLEnc:   enc,
		Color:        color,
	}
	if err := PG.Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

// DeleteUserCalendarFeed removes a feed owned by the user.
func DeleteUserCalendarFeed(systemUserID, feedID int64) (bool, error) {
	tx := PG.Where("id = ? AND system_user_id = ?", feedID, systemUserID).Delete(&UserCalendarFeed{})
	if tx.Error != nil {
		return false, tx.Error
	}
	return tx.RowsAffected > 0, nil
}
