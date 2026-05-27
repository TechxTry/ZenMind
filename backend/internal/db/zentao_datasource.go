package db

import (
	"encoding/json"
	"errors"
	"log"
	"strings"
	"zenmind/internal/config"
	"zenmind/internal/crypto"

	"gorm.io/gorm"
)

const settingKeyZentaoDatasource = "zentao_datasource"

type zentaoDatasourceStored struct {
	Host        string `json:"host"`
	Port        string `json:"port"`
	User        string `json:"user"`
	DBName      string `json:"db_name"`
	PasswordEnc string `json:"password_enc"`
}

func loadZentaoDatasourceStored() (zentaoDatasourceStored, bool, error) {
	var row struct {
		Value string `gorm:"column:value"`
	}
	err := PG.Table("app_settings").Select("value").Where("setting_key = ?", settingKeyZentaoDatasource).Scan(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return zentaoDatasourceStored{}, false, nil
	}
	if err != nil {
		return zentaoDatasourceStored{}, false, err
	}
	raw := strings.TrimSpace(row.Value)
	if raw == "" {
		return zentaoDatasourceStored{}, false, nil
	}
	var stored zentaoDatasourceStored
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return zentaoDatasourceStored{}, false, err
	}
	return stored, true, nil
}

// LoadZentaoDatasourceIntoConfig applies persisted datasource settings to config.Global.
// Env defaults remain when nothing is stored in PG.
func LoadZentaoDatasourceIntoConfig() error {
	stored, ok, err := loadZentaoDatasourceStored()
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if h := strings.TrimSpace(stored.Host); h != "" {
		config.Global.ZentaoHost = h
	}
	if p := strings.TrimSpace(stored.Port); p != "" {
		config.Global.ZentaoPort = p
	}
	if u := strings.TrimSpace(stored.User); u != "" {
		config.Global.ZentaoUser = u
	}
	if d := strings.TrimSpace(stored.DBName); d != "" {
		config.Global.ZentaoDBName = d
	}
	enc := strings.TrimSpace(stored.PasswordEnc)
	if enc == "" {
		return nil
	}
	plain, err := crypto.DecryptString(enc, AppEncryptionSecret())
	if err != nil {
		log.Printf("[db] zentao datasource password decrypt failed (check ZT_CRED_SECRET / JWT_SECRET): %v", err)
		return nil
	}
	config.Global.ZentaoPass = plain
	return nil
}

// SaveZentaoDatasource persists MySQL datasource settings (password encrypted).
func SaveZentaoDatasource(host, port, user, dbName, plainPassword string) error {
	enc, err := crypto.EncryptString(plainPassword, AppEncryptionSecret())
	if err != nil {
		return err
	}
	stored := zentaoDatasourceStored{
		Host:        strings.TrimSpace(host),
		Port:        strings.TrimSpace(port),
		User:        strings.TrimSpace(user),
		DBName:      strings.TrimSpace(dbName),
		PasswordEnc: enc,
	}
	b, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	return PG.Exec(`
INSERT INTO app_settings (setting_key, value, updated_at) VALUES (?, ?, NOW())
ON CONFLICT (setting_key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		settingKeyZentaoDatasource, string(b)).Error
}

// GetPersistedZentaoDatasourcePassword returns the decrypted password from PG when configured.
func GetPersistedZentaoDatasourcePassword() (string, bool, error) {
	stored, ok, err := loadZentaoDatasourceStored()
	if err != nil || !ok {
		return "", ok, err
	}
	enc := strings.TrimSpace(stored.PasswordEnc)
	if enc == "" {
		return "", false, nil
	}
	plain, err := crypto.DecryptString(enc, AppEncryptionSecret())
	if err != nil {
		return "", true, err
	}
	return plain, true, nil
}
