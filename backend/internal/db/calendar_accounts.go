package db

import (
	"fmt"
	"strings"
	"time"
	"zenmind/internal/config"
	"zenmind/internal/crypto"
)

type UserCalendarAccount struct {
	ID           int64     `json:"id" gorm:"primaryKey;column:id"`
	SystemUserID int64     `json:"system_user_id" gorm:"column:system_user_id"`
	Type         string    `json:"type" gorm:"column:type"`
	Server       string    `json:"server" gorm:"column:server"`
	Username     string    `json:"username" gorm:"column:username"`
	PasswordEnc  string    `json:"-" gorm:"column:password_enc"`
	Description  string    `json:"description" gorm:"column:description"`
	CreatedAt    time.Time `json:"created_at" gorm:"column:created_at"`
	UpdatedAt    time.Time `json:"updated_at" gorm:"column:updated_at"`
}

func (UserCalendarAccount) TableName() string { return "user_calendar_accounts" }

func calendarAccountSecret() string {
	return strings.TrimSpace(config.Global.JWTSecret)
}

const maxCalendarAccountsPerUser = 10

func ListUserCalendarAccounts(systemUserID int64) ([]UserCalendarAccount, error) {
	var rows []UserCalendarAccount
	err := PG.Where("system_user_id = ?", systemUserID).Order("id DESC").Find(&rows).Error
	return rows, err
}

func ListUserCalendarAccountsForFetch(systemUserID int64) ([]struct {
	ID          int64
	Type        string
	Server      string
	Username    string
	Password    string
	Description string
}, error) {
	secret := calendarAccountSecret()
	if secret == "" {
		return nil, fmt.Errorf("jwt secret not configured")
	}
	var rows []UserCalendarAccount
	if err := PG.Where("system_user_id = ?", systemUserID).Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]struct {
		ID          int64
		Type        string
		Server      string
		Username    string
		Password    string
		Description string
	}, 0, len(rows))
	for _, r := range rows {
		pt, err := crypto.DecryptString(r.PasswordEnc, secret)
		if err != nil {
			continue
		}
		out = append(out, struct {
			ID          int64
			Type        string
			Server      string
			Username    string
			Password    string
			Description string
		}{
			ID:          r.ID,
			Type:        r.Type,
			Server:      r.Server,
			Username:    r.Username,
			Password:    pt,
			Description: r.Description,
		})
	}
	return out, nil
}

func InsertUserCalendarAccount(systemUserID int64, typ, server, username, plainPassword, description string) (int64, error) {
	var n int64
	if err := PG.Model(&UserCalendarAccount{}).Where("system_user_id = ?", systemUserID).Count(&n).Error; err != nil {
		return 0, err
	}
	if n >= maxCalendarAccountsPerUser {
		return 0, fmt.Errorf("calendar account limit reached (%d)", maxCalendarAccountsPerUser)
	}
	enc, err := crypto.EncryptString(plainPassword, calendarAccountSecret())
	if err != nil {
		return 0, err
	}
	row := UserCalendarAccount{
		SystemUserID: systemUserID,
		Type:         typ,
		Server:       server,
		Username:     username,
		PasswordEnc:  enc,
		Description:  description,
	}
	if err := PG.Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func DeleteUserCalendarAccount(systemUserID, id int64) (bool, error) {
	tx := PG.Where("id = ? AND system_user_id = ?", id, systemUserID).Delete(&UserCalendarAccount{})
	if tx.Error != nil {
		return false, tx.Error
	}
	return tx.RowsAffected > 0, nil
}
