package db

import (
	"strings"
	"zenmind/internal/config"
)

// AppEncryptionSecret is used to encrypt secrets at rest (datasource password, user zentao credentials, etc.).
func AppEncryptionSecret() string {
	if strings.TrimSpace(config.Global.ZentaoCredSecret) != "" {
		return config.Global.ZentaoCredSecret
	}
	return config.Global.JWTSecret
}
