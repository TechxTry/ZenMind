// Package db manages dual database connections:
//   - PG: local PostgreSQL (read-write target)
//   - MySQL: Zentao source (read-only)
package db

import (
	"encoding/json"
	"log"
	"sync"
	"time"
	"zenmind/internal/config"
	"zenmind/internal/models"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	PG    *gorm.DB
	Zentao *gorm.DB
	ztMu  sync.RWMutex
)

// InitPG opens the PostgreSQL connection pool.
func InitPG() {
	var err error
	PG, err = gorm.Open(postgres.Open(config.Global.PGDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		log.Fatalf("[db] failed to connect PG: %v", err)
	}
	sql, _ := PG.DB()
	sql.SetMaxOpenConns(20)
	sql.SetMaxIdleConns(5)
	sql.SetConnMaxLifetime(30 * time.Minute)
	log.Println("[db] PostgreSQL connected")
}

// ConnectZentao opens (or refreshes) the Zentao MySQL connection.
func ConnectZentao(cfg config.Config) error {
	ztMu.Lock()
	defer ztMu.Unlock()

	dsn := cfg.ZentaoDSN()
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return err
	}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(5)
	sql.SetMaxIdleConns(2)
	sql.SetConnMaxLifetime(10 * time.Minute)
	Zentao = db
	log.Println("[db] Zentao MySQL connected")
	return nil
}

// GetZentao returns the current Zentao DB handle (thread-safe).
func GetZentao() *gorm.DB {
	ztMu.RLock()
	defer ztMu.RUnlock()
	return Zentao
}

// RowToJSONB converts an arbitrary struct to models.JSONB via JSON marshal.
func RowToJSONB(v interface{}) models.JSONB {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var m models.JSONB
	_ = json.Unmarshal(b, &m)
	return m
}

// SafeTime returns nil if t is zero (protects PG from 0000-00-00 Zentao times).
func SafeTime(t *time.Time) *time.Time {
	if t == nil || t.IsZero() || t.Year() <= 1 {
		return nil
	}
	return t
}
