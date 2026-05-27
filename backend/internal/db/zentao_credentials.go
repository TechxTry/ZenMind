package db

import (
	"errors"
	"zenmind/internal/crypto"

	"gorm.io/gorm"
)

type ZentaoCredential struct {
	Username      string `gorm:"column:username;primaryKey"`
	ZentaoAccount string `gorm:"column:zentao_account"`
	PasswordEnc   string `gorm:"column:password_enc"`
}

func (ZentaoCredential) TableName() string { return "user_zentao_credentials" }

type ZentaoCredentialRef struct {
	Username      string `gorm:"column:username"`
	ZentaoAccount string `gorm:"column:zentao_account"`
}

func UpsertZentaoCredential(username, ztAccount, plainPassword string) error {
	enc, err := crypto.EncryptString(plainPassword, AppEncryptionSecret())
	if err != nil {
		return err
	}
	row := ZentaoCredential{
		Username:      username,
		ZentaoAccount: ztAccount,
		PasswordEnc:   enc,
	}
	return PG.Exec(`
INSERT INTO user_zentao_credentials (username, zentao_account, password_enc, updated_at)
VALUES (?, ?, ?, NOW())
ON CONFLICT (username) DO UPDATE
SET zentao_account = EXCLUDED.zentao_account, password_enc = EXCLUDED.password_enc, updated_at = NOW()
`, row.Username, row.ZentaoAccount, row.PasswordEnc).Error
}

// GetZentaoCredentialAccount 仅读取已保存的禅道登录名（不解密密码），用于状态展示。
func GetZentaoCredentialAccount(username string) (zentaoAccount string, ok bool, err error) {
	var row struct {
		ZentaoAccount string `gorm:"column:zentao_account"`
	}
	e := PG.Table((ZentaoCredential{}).TableName()).
		Select("zentao_account").
		Where("username = ?", username).
		Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if e != nil {
		return "", false, e
	}
	return row.ZentaoAccount, true, nil
}

func GetZentaoCredential(username string) (account string, plainPassword string, ok bool, err error) {
	var row ZentaoCredential
	e := PG.Where("username = ?", username).First(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return "", "", false, nil
	}
	if e != nil {
		return "", "", false, e
	}
	pt, err := crypto.DecryptString(row.PasswordEnc, AppEncryptionSecret())
	if err != nil {
		return "", "", false, err
	}
	return row.ZentaoAccount, pt, true, nil
}

func ListZentaoCredentialRefs() ([]ZentaoCredentialRef, error) {
	var rows []ZentaoCredentialRef
	err := PG.Table((ZentaoCredential{}).TableName()).
		Select("username, zentao_account").
		Order("username ASC").
		Find(&rows).Error
	return rows, err
}

func DeleteZentaoCredential(username string) error {
	return PG.Exec(`DELETE FROM user_zentao_credentials WHERE username = ?`, username).Error
}
