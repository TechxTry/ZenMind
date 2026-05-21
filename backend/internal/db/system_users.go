package db

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"zenmind/internal/config"
	"zenmind/internal/models"

	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	RoleSuperAdmin = "super_admin"
	RoleAdmin      = "admin"
	RoleUser       = "user"

	ScopeSelf  = "SELF"
	ScopeGroup = "GROUP"
	ScopeAll   = "ALL"
)

func normalizeRole(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case RoleSuperAdmin, RoleAdmin, RoleUser:
		return s
	default:
		return RoleUser
	}
}

func normalizeScope(s string) string {
	s = strings.TrimSpace(strings.ToUpper(s))
	switch s {
	case ScopeSelf, ScopeGroup, ScopeAll:
		return s
	default:
		return ScopeSelf
	}
}

func HashPassword(plain string) (string, error) {
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return "", fmt.Errorf("empty password")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func CheckPassword(hash, plain string) bool {
	if strings.TrimSpace(hash) == "" || strings.TrimSpace(plain) == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

func GetSystemUserByUsername(username string) (models.SystemUser, bool, error) {
	var u models.SystemUser
	e := PG.Where("username = ?", username).First(&u).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return models.SystemUser{}, false, nil
	}
	if e != nil {
		return models.SystemUser{}, false, e
	}
	u.Role = normalizeRole(u.Role)
	u.DataScope = normalizeScope(u.DataScope)
	return u, true, nil
}

func GetSystemUserByID(id int64) (models.SystemUser, bool, error) {
	var u models.SystemUser
	e := PG.Where("id = ?", id).First(&u).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return models.SystemUser{}, false, nil
	}
	if e != nil {
		return models.SystemUser{}, false, e
	}
	u.Role = normalizeRole(u.Role)
	u.DataScope = normalizeScope(u.DataScope)
	return u, true, nil
}

func GetZentaoBindingBySystemUserID(uid int64) (models.ZentaoBinding, bool, error) {
	var b models.ZentaoBinding
	e := PG.Where("system_user_id = ?", uid).First(&b).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return models.ZentaoBinding{}, false, nil
	}
	if e != nil {
		return models.ZentaoBinding{}, false, e
	}
	return b, true, nil
}

// EnsureBootstrapAdmin ensures at least one super admin exists.
// If system_users is empty, it will create a super_admin using ADMIN_USER/ADMIN_PASS.
func EnsureBootstrapAdmin() error {
	var n int64
	if err := PG.Model(&models.SystemUser{}).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return nil
	}

	username := strings.TrimSpace(config.Global.AdminUser)
	pass := strings.TrimSpace(config.Global.AdminPass)
	if username == "" || pass == "" {
		return fmt.Errorf("bootstrap admin requires ADMIN_USER and ADMIN_PASS")
	}
	hash, err := HashPassword(pass)
	if err != nil {
		return err
	}
	now := time.Now()
	u := models.SystemUser{
		Username:     username,
		DisplayName:  "Administrator",
		PasswordHash: hash,
		Role:         RoleSuperAdmin,
		DataScope:    ScopeAll,
		Disabled:     false,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	return PG.Create(&u).Error
}

type CreateSystemUserInput struct {
	Username       string
	DisplayName    string
	Password       string
	Role           string
	DataScope      string
	DefaultGroupID *int
}

func CreateSystemUser(in CreateSystemUserInput) (models.SystemUser, error) {
	in.Username = strings.TrimSpace(in.Username)
	if in.Username == "" {
		return models.SystemUser{}, fmt.Errorf("username required")
	}
	hash, err := HashPassword(in.Password)
	if err != nil {
		return models.SystemUser{}, err
	}
	now := time.Now()
	u := models.SystemUser{
		Username:       in.Username,
		DisplayName:    strings.TrimSpace(in.DisplayName),
		PasswordHash:   hash,
		Role:           normalizeRole(in.Role),
		DataScope:      normalizeScope(in.DataScope),
		DefaultGroupID: in.DefaultGroupID,
		Disabled:       false,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := PG.Create(&u).Error; err != nil {
		return models.SystemUser{}, err
	}
	return u, nil
}

// CreateSystemUserTx is a transactional variant used by batch operations.
func CreateSystemUserTx(tx *gorm.DB, in CreateSystemUserInput) (models.SystemUser, error) {
	in.Username = strings.TrimSpace(in.Username)
	if in.Username == "" {
		return models.SystemUser{}, fmt.Errorf("username required")
	}
	hash, err := HashPassword(in.Password)
	if err != nil {
		return models.SystemUser{}, err
	}
	now := time.Now()
	u := models.SystemUser{
		Username:       in.Username,
		DisplayName:    strings.TrimSpace(in.DisplayName),
		PasswordHash:   hash,
		Role:           normalizeRole(in.Role),
		DataScope:      normalizeScope(in.DataScope),
		DefaultGroupID: in.DefaultGroupID,
		Disabled:       false,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := tx.Create(&u).Error; err != nil {
		return models.SystemUser{}, err
	}
	return u, nil
}

type UpdateSystemUserInput struct {
	DisplayName    *string
	Role           *string
	DataScope      *string
	DefaultGroupID **int
	Disabled       *bool
}

func UpdateSystemUser(id int64, in UpdateSystemUserInput) (models.SystemUser, bool, error) {
	u, ok, err := GetSystemUserByID(id)
	if err != nil || !ok {
		return models.SystemUser{}, ok, err
	}
	updates := map[string]interface{}{}
	if in.DisplayName != nil {
		updates["display_name"] = strings.TrimSpace(*in.DisplayName)
	}
	if in.Role != nil {
		updates["role"] = normalizeRole(*in.Role)
	}
	if in.DataScope != nil {
		updates["data_scope"] = normalizeScope(*in.DataScope)
	}
	if in.DefaultGroupID != nil {
		updates["default_group_id"] = *in.DefaultGroupID
	}
	if in.Disabled != nil {
		updates["disabled"] = *in.Disabled
	}
	if len(updates) == 0 {
		return u, true, nil
	}
	updates["updated_at"] = time.Now()
	if err := PG.Model(&models.SystemUser{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return models.SystemUser{}, true, err
	}
	return GetSystemUserByID(id)
}

func ResetSystemUserPassword(id int64, newPassword string) error {
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	return PG.Model(&models.SystemUser{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"password_hash": hash,
			"updated_at":    time.Now(),
		}).Error
}

func ListSystemUsers(q string, page, pageSize int) (rows []models.SystemUser, total int64, err error) {
	query := PG.Model(&models.SystemUser{})
	if strings.TrimSpace(q) != "" {
		like := "%" + strings.TrimSpace(q) + "%"
		query = query.Where("username ILIKE ? OR display_name ILIKE ?", like, like)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	for i := range rows {
		rows[i].Role = normalizeRole(rows[i].Role)
		rows[i].DataScope = normalizeScope(rows[i].DataScope)
		rows[i].PasswordHash = ""
	}
	return rows, total, nil
}

func classifyBindingError(err error, zentaoAccount string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			if pgErr.ConstraintName == "zentao_bindings_zentao_account_key" || pgErr.ConstraintName == "zentao_bindings_account" {
				return fmt.Errorf("禅道账号 %q 已被其他系统用户绑定", zentaoAccount)
			}
		case "23503": // foreign_key_violation
			return fmt.Errorf("禅道账号 %q 不存在于本地同步人员表（local_users），请先同步禅道数据", zentaoAccount)
		}
	}
	return fmt.Errorf("绑定失败: %w", err)
}

func SetZentaoBinding(systemUserID int64, zentaoAccount string) (models.ZentaoBinding, error) {
	zentaoAccount = strings.TrimSpace(zentaoAccount)
	if zentaoAccount == "" {
		return models.ZentaoBinding{}, fmt.Errorf("zentao_account required")
	}
	row := models.ZentaoBinding{
		SystemUserID:  systemUserID,
		ZentaoAccount: zentaoAccount,
	}
	// Upsert by system_user_id; unique constraint on zentao_account will protect reuse.
	if err := PG.Exec(`
INSERT INTO zentao_bindings (system_user_id, zentao_account, created_at, updated_at)
VALUES (?, ?, NOW(), NOW())
ON CONFLICT (system_user_id) DO UPDATE
SET zentao_account = EXCLUDED.zentao_account, updated_at = NOW()
`, row.SystemUserID, row.ZentaoAccount).Error; err != nil {
		return models.ZentaoBinding{}, classifyBindingError(err, zentaoAccount)
	}
	b, _, err := GetZentaoBindingBySystemUserID(systemUserID)
	return b, err
}

// SetZentaoBindingTx is a transactional variant used by batch operations.
func SetZentaoBindingTx(tx *gorm.DB, systemUserID int64, zentaoAccount string) (models.ZentaoBinding, error) {
	zentaoAccount = strings.TrimSpace(zentaoAccount)
	if zentaoAccount == "" {
		return models.ZentaoBinding{}, fmt.Errorf("zentao_account required")
	}
	if err := tx.Exec(`
INSERT INTO zentao_bindings (system_user_id, zentao_account, created_at, updated_at)
VALUES (?, ?, NOW(), NOW())
ON CONFLICT (system_user_id) DO UPDATE
SET zentao_account = EXCLUDED.zentao_account, updated_at = NOW()
`, systemUserID, zentaoAccount).Error; err != nil {
		return models.ZentaoBinding{}, classifyBindingError(err, zentaoAccount)
	}

	var b models.ZentaoBinding
	if err := tx.Where("system_user_id = ?", systemUserID).First(&b).Error; err != nil {
		// Shouldn't happen in a fresh transaction, but keep error for completeness.
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.ZentaoBinding{}, nil
		}
		return models.ZentaoBinding{}, err
	}
	return b, nil
}

func DeleteZentaoBinding(systemUserID int64) error {
	return PG.Exec(`DELETE FROM zentao_bindings WHERE system_user_id = ?`, systemUserID).Error
}
