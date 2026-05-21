package db

import (
	"fmt"
	"io/fs"
	"log"
	"sort"
	"strings"

	"zenmind/migrations"
)

// RunPendingMigrations 在已连接 PG 的前提下，执行尚未记录在 schema_migrations 中的 .sql 文件（按文件名排序）。
// 迁移文件随后端二进制内嵌（见 migrations.Files），更新镜像后重启即可自动补齐新脚本。
func RunPendingMigrations() error {
	if PG == nil {
		return fmt.Errorf("RunPendingMigrations: PG is nil")
	}

	if err := PG.Exec(`
CREATE TABLE IF NOT EXISTS schema_migrations (
  version     TEXT PRIMARY KEY,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`).Error; err != nil {
		return fmt.Errorf("schema_migrations: %w", err)
	}

	names, err := listSQLFiles(migrations.Files)
	if err != nil {
		return err
	}

	for _, name := range names {
		var n int64
		if err := PG.Table("schema_migrations").Where("version = ?", name).Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			continue
		}

		body, err := fs.ReadFile(migrations.Files, name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		sql := strings.TrimSpace(string(body))
		if sql == "" {
			if err := PG.Exec("INSERT INTO schema_migrations (version) VALUES (?)", name).Error; err != nil {
				return fmt.Errorf("migration %s empty marker: %w", name, err)
			}
			log.Printf("[migrate] skipped empty %s (recorded)", name)
			continue
		}

		tx := PG.Begin()
		if err := tx.Exec(sql).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", name).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s record: %w", name, err)
		}
		if err := tx.Commit().Error; err != nil {
			return err
		}
		log.Printf("[migrate] applied %s", name)
	}
	return nil
}

func listSQLFiles(fsys fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if strings.HasSuffix(strings.ToLower(n), ".sql") {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names, nil
}
