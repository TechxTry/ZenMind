package db

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

const MCPTokenPrefix = "zmcp_"

type MCPAccessToken struct {
	ID          int64      `json:"id" gorm:"primaryKey;column:id"`
	UserID      int64      `json:"user_id" gorm:"column:user_id"`
	TokenName   string     `json:"token_name" gorm:"column:token_name"`
	TokenHash   string     `json:"-" gorm:"column:token_hash"`
	TokenPrefix string     `json:"token_prefix" gorm:"column:token_prefix"`
	ExpiresAt   *time.Time `json:"expires_at" gorm:"column:expires_at"`
	LastUsedAt  *time.Time `json:"last_used_at" gorm:"column:last_used_at"`
	RevokedAt   *time.Time `json:"revoked_at" gorm:"column:revoked_at"`
	CreatedAt   time.Time  `json:"created_at" gorm:"column:created_at"`
	UpdatedAt   time.Time  `json:"updated_at" gorm:"column:updated_at"`
}

func (MCPAccessToken) TableName() string { return "mcp_access_tokens" }

func GenerateMCPRawToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return MCPTokenPrefix + hex.EncodeToString(b), nil
}

func HashMCPToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func CreateMCPAccessToken(userID int64, tokenName string, expiresAt *time.Time) (MCPAccessToken, string, error) {
	raw, err := GenerateMCPRawToken()
	if err != nil {
		return MCPAccessToken{}, "", err
	}
	row := MCPAccessToken{
		UserID:      userID,
		TokenName:   tokenName,
		TokenHash:   HashMCPToken(raw),
		TokenPrefix: raw[:12],
		ExpiresAt:   expiresAt,
	}
	if err := PG.Create(&row).Error; err != nil {
		return MCPAccessToken{}, "", err
	}
	return row, raw, nil
}

func ListMCPAccessTokensByUser(userID int64) ([]MCPAccessToken, error) {
	var rows []MCPAccessToken
	err := PG.Where("user_id = ?", userID).
		Where("revoked_at IS NULL").
		Order("id DESC").
		Find(&rows).Error
	return rows, err
}

func RevokeMCPAccessToken(userID, tokenID int64) error {
	now := time.Now()
	res := PG.Model(&MCPAccessToken{}).
		Where("id = ? AND user_id = ? AND revoked_at IS NULL", tokenID, userID).
		Updates(map[string]interface{}{"revoked_at": now, "updated_at": now})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("token not found")
	}
	return nil
}

func FindUserByMCPRawToken(raw string) (MCPAccessToken, bool, error) {
	hash := HashMCPToken(raw)
	var tok MCPAccessToken
	err := PG.Where("token_hash = ?", hash).
		Where("revoked_at IS NULL").
		Where("(expires_at IS NULL OR expires_at > NOW())").
		First(&tok).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return MCPAccessToken{}, false, nil
		}
		return MCPAccessToken{}, false, err
	}
	return tok, true, nil
}

func TouchMCPAccessTokenLastUsed(id int64) {
	now := time.Now()
	_ = PG.Model(&MCPAccessToken{}).Where("id = ?", id).
		Updates(map[string]interface{}{"last_used_at": now, "updated_at": now}).Error
}
