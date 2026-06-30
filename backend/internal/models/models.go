package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// JSONB is a helper type for PostgreSQL JSONB columns.
type JSONB map[string]interface{}

func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	b, err := json.Marshal(j)
	return string(b), err
}

func (j *JSONB) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("cannot scan type %T into JSONB", value)
	}
	return json.Unmarshal(bytes, j)
}

// ---- Local Tables ----

type LocalUser struct {
	ID       int64     `json:"id" gorm:"primaryKey;column:id"`
	Account  string    `json:"account" gorm:"uniqueIndex;column:account"`
	Realname string    `json:"realname" gorm:"column:realname"`
	Role     string    `json:"role" gorm:"column:role"`
	Deleted  bool      `json:"deleted" gorm:"column:deleted;default:false"`
	RawData  JSONB     `json:"raw_data" gorm:"column:raw_data;type:jsonb"`
	SyncedAt time.Time `json:"synced_at" gorm:"column:synced_at"`
}

func (LocalUser) TableName() string { return "local_users" }

type ProjectGroup struct {
	ID          int       `json:"id" gorm:"primaryKey;autoIncrement;column:id"`
	Name        string    `json:"name" gorm:"uniqueIndex;column:name"`
	Description string    `json:"description" gorm:"column:description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (ProjectGroup) TableName() string { return "project_groups" }

type GroupMember struct {
	GroupID int    `gorm:"primaryKey;column:group_id"`
	Account string `gorm:"primaryKey;column:account"`
}

func (GroupMember) TableName() string { return "group_members" }

type LocalExecution struct {
	ID        int64      `json:"id" gorm:"primaryKey;column:id"`
	Name      string     `json:"name" gorm:"column:name"`
	Status    string     `json:"status" gorm:"column:status"`
	BeginDate *time.Time `json:"begin_date" gorm:"column:begin_date"`
	EndDate   *time.Time `json:"end_date" gorm:"column:end_date"`
	ParentID  *int64     `json:"parent_id" gorm:"column:parent_id"`
	Type      string     `json:"type" gorm:"column:type"`
	Deleted   bool       `json:"deleted" gorm:"column:deleted;default:false"`
	RawData   JSONB      `json:"raw_data" gorm:"column:raw_data;type:jsonb"`
	SyncedAt  time.Time  `json:"synced_at" gorm:"column:synced_at"`
}

func (LocalExecution) TableName() string { return "local_executions" }

type LocalProgram struct {
	ID        int64      `json:"id" gorm:"primaryKey;column:id"`
	Name      string     `json:"name" gorm:"column:name"`
	Status    string     `json:"status" gorm:"column:status"`
	ParentID  *int64     `json:"parent_id" gorm:"column:parent_id"`
	Path      string     `json:"path" gorm:"column:path"`
	Grade     *int       `json:"grade" gorm:"column:grade"`
	BeginDate *time.Time `json:"begin_date" gorm:"column:begin_date"`
	EndDate   *time.Time `json:"end_date" gorm:"column:end_date"`
	Deleted   bool       `json:"deleted" gorm:"column:deleted;default:false"`
	RawData   JSONB      `json:"raw_data" gorm:"column:raw_data;type:jsonb"`
	SyncedAt  time.Time  `json:"synced_at" gorm:"column:synced_at"`
}

func (LocalProgram) TableName() string { return "local_programs" }

type LocalProject struct {
	ID        int64      `json:"id" gorm:"primaryKey;column:id"`
	Name      string     `json:"name" gorm:"column:name"`
	Status    string     `json:"status" gorm:"column:status"`
	ParentID  *int64     `json:"parent_id" gorm:"column:parent_id"`
	Path      string     `json:"path" gorm:"column:path"`
	Grade     *int       `json:"grade" gorm:"column:grade"`
	BeginDate *time.Time `json:"begin_date" gorm:"column:begin_date"`
	EndDate   *time.Time `json:"end_date" gorm:"column:end_date"`
	Deleted   bool       `json:"deleted" gorm:"column:deleted;default:false"`
	RawData   JSONB      `json:"raw_data" gorm:"column:raw_data;type:jsonb"`
	SyncedAt  time.Time  `json:"synced_at" gorm:"column:synced_at"`
}

func (LocalProject) TableName() string { return "local_projects" }

type LocalProductLine struct {
	ID       int64     `json:"id" gorm:"primaryKey;column:id"`
	Name     string    `json:"name" gorm:"column:name"`
	ParentID *int64    `json:"parent_id" gorm:"column:parent_id"`
	Path     string    `json:"path" gorm:"column:path"`
	Grade    *int      `json:"grade" gorm:"column:grade"`
	Deleted  bool      `json:"deleted" gorm:"column:deleted;default:false"`
	RawData  JSONB     `json:"raw_data" gorm:"column:raw_data;type:jsonb"`
	SyncedAt time.Time `json:"synced_at" gorm:"column:synced_at"`
}

func (LocalProductLine) TableName() string { return "local_product_lines" }

type LocalProduct struct {
	ID       int64     `json:"id" gorm:"primaryKey;column:id"`
	Name     string    `json:"name" gorm:"column:name"`
	Code     string    `json:"code" gorm:"column:code"`
	Status   string    `json:"status" gorm:"column:status"`
	LineID   *int64    `json:"line_id" gorm:"column:line_id"`
	Deleted  bool      `json:"deleted" gorm:"column:deleted;default:false"`
	RawData  JSONB     `json:"raw_data" gorm:"column:raw_data;type:jsonb"`
	SyncedAt time.Time `json:"synced_at" gorm:"column:synced_at"`
}

func (LocalProduct) TableName() string { return "local_products" }

type LocalTask struct {
	ID             int64      `json:"id" gorm:"primaryKey;column:id"`
	Name           string     `json:"name" gorm:"column:name"`
	Type           string     `json:"type" gorm:"column:type"`
	Status         string     `json:"status" gorm:"column:status"`
	AssignedTo     string     `json:"assigned_to" gorm:"column:assigned_to"`
	FinishedBy     string     `json:"finished_by" gorm:"column:finished_by"`
	Estimate       float64    `json:"estimate" gorm:"column:estimate"`
	Consumed       float64    `json:"consumed" gorm:"column:consumed"`
	ExecutionID    int64      `json:"execution_id" gorm:"column:execution_id"`
	StoryID        int64      `json:"story_id" gorm:"column:story_id"`
	OpenedDate     *time.Time `json:"opened_date" gorm:"column:opened_date"`
	StartedDate    *time.Time `json:"started_date" gorm:"column:started_date"`
	AssignedDate   *time.Time `json:"assigned_date" gorm:"column:assigned_date"`
	DeadlineDate   *time.Time `json:"deadline_date" gorm:"column:deadline_date"`
	FinishedDate   *time.Time `json:"finished_date" gorm:"column:finished_date"`
	ClosedDate     *time.Time `json:"closed_date" gorm:"column:closed_date"`
	LastEditedDate *time.Time `json:"last_edited_date" gorm:"column:last_edited_date"`
	Deleted        bool       `json:"deleted" gorm:"column:deleted;default:false"`
	RawData        JSONB      `json:"raw_data" gorm:"column:raw_data;type:jsonb"`
	SyncedAt       time.Time  `json:"synced_at" gorm:"column:synced_at"`
}

func (LocalTask) TableName() string { return "local_tasks" }

type LocalStory struct {
	ID             int64      `json:"id" gorm:"primaryKey;column:id"`
	Title          string     `json:"title" gorm:"column:title"`
	Status         string     `json:"status" gorm:"column:status"`
	AssignedTo     string     `json:"assigned_to" gorm:"column:assigned_to"`
	Estimate       float64    `json:"estimate" gorm:"column:estimate"`
	ProductID      int64      `json:"product_id" gorm:"column:product_id"`
	OpenedDate     *time.Time `json:"opened_date" gorm:"column:opened_date"`
	ClosedDate     *time.Time `json:"closed_date" gorm:"column:closed_date"`
	LastEditedDate *time.Time `json:"last_edited_date" gorm:"column:last_edited_date"`
	Deleted        bool       `json:"deleted" gorm:"column:deleted;default:false"`
	RawData        JSONB      `json:"raw_data" gorm:"column:raw_data;type:jsonb"`
	SyncedAt       time.Time  `json:"synced_at" gorm:"column:synced_at"`
}

func (LocalStory) TableName() string { return "local_stories" }

type LocalBug struct {
	ID             int64      `json:"id" gorm:"primaryKey;column:id"`
	Title          string     `json:"title" gorm:"column:title"`
	Severity       int        `json:"severity" gorm:"column:severity"`
	Status         string     `json:"status" gorm:"column:status"`
	AssignedTo     string     `json:"assigned_to" gorm:"column:assigned_to"`
	ResolvedBy     string     `json:"resolved_by" gorm:"column:resolved_by"`
	Resolution     string     `json:"resolution" gorm:"column:resolution"`
	ExecutionID    int64      `json:"execution_id" gorm:"column:execution_id"`
	StoryID        int64      `json:"story_id" gorm:"column:story_id"`
	TaskID         int64      `json:"task_id" gorm:"column:task_id"`
	OpenedDate     *time.Time `json:"opened_date" gorm:"column:opened_date"`
	ResolvedDate   *time.Time `json:"resolved_date" gorm:"column:resolved_date"`
	ClosedDate     *time.Time `json:"closed_date" gorm:"column:closed_date"`
	LastEditedDate *time.Time `json:"last_edited_date" gorm:"column:last_edited_date"`
	Deleted        bool       `json:"deleted" gorm:"column:deleted;default:false"`
	RawData        JSONB      `json:"raw_data" gorm:"column:raw_data;type:jsonb"`
	SyncedAt       time.Time  `json:"synced_at" gorm:"column:synced_at"`
}

func (LocalBug) TableName() string { return "local_bugs" }

type LocalAction struct {
	ID         int64      `json:"id" gorm:"primaryKey;column:id"`
	ObjectType string     `json:"object_type" gorm:"column:object_type"`
	ObjectID   int64      `json:"object_id" gorm:"column:object_id"`
	Actor      string     `json:"actor" gorm:"column:actor"`
	Action     string     `json:"action" gorm:"column:action"`
	ActionDate *time.Time `json:"action_date" gorm:"column:action_date"`
	Comment    string     `json:"comment" gorm:"column:comment"`
	Extra      string     `json:"extra" gorm:"column:extra"`
	Deleted    bool       `json:"deleted" gorm:"column:deleted;default:false"`
	RawData    JSONB      `json:"raw_data" gorm:"column:raw_data;type:jsonb"`
	SyncedAt   time.Time  `json:"synced_at" gorm:"column:synced_at"`
}

func (LocalAction) TableName() string { return "local_actions" }

type LocalHistory struct {
	ID       int64     `json:"id" gorm:"primaryKey;column:id"`
	ActionID int64     `json:"action_id" gorm:"column:action_id"`
	Field    string    `json:"field" gorm:"column:field"`
	Old      string    `json:"old" gorm:"column:old"`
	New      string    `json:"new" gorm:"column:new"`
	Diff     string    `json:"diff" gorm:"column:diff"`
	RawData  JSONB     `json:"raw_data" gorm:"column:raw_data;type:jsonb"`
	SyncedAt time.Time `json:"synced_at" gorm:"column:synced_at"`
}

func (LocalHistory) TableName() string { return "local_histories" }

type LocalEffort struct {
	ID         int64      `json:"id" gorm:"primaryKey;column:id"`
	Account    string     `json:"account" gorm:"column:account"`
	WorkDate   *time.Time `json:"work_date" gorm:"column:work_date"`
	Consumed   float64    `json:"consumed" gorm:"column:consumed"`
	Work       string     `json:"work" gorm:"column:work"`
	ObjectType string     `json:"object_type" gorm:"column:object_type"`
	ObjectID   int64      `json:"object_id" gorm:"column:object_id"`
	Deleted    bool       `json:"deleted" gorm:"column:deleted;default:false"`
	RawData    JSONB      `json:"raw_data" gorm:"column:raw_data;type:jsonb"`
	SyncedAt   time.Time  `json:"synced_at" gorm:"column:synced_at"`
}

func (LocalEffort) TableName() string { return "local_efforts" }

type SyncWatermark struct {
	Table     string    `json:"table" gorm:"primaryKey;column:table_name"`
	Watermark time.Time `json:"watermark" gorm:"column:watermark"`
	LastCount int64     `json:"last_count" gorm:"column:last_count"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (SyncWatermark) TableName() string { return "sync_watermarks" }

// ---- System Tables (Auth/RBAC) ----

type SystemUser struct {
	ID             int64     `json:"id" gorm:"primaryKey;column:id"`
	Username       string    `json:"username" gorm:"uniqueIndex;column:username"`
	DisplayName    string    `json:"display_name" gorm:"column:display_name"`
	PasswordHash   string    `json:"-" gorm:"column:password_hash"`
	Role           string    `json:"role" gorm:"column:role"`
	DataScope      string    `json:"data_scope" gorm:"column:data_scope"`
	DefaultGroupID *int      `json:"default_group_id" gorm:"column:default_group_id"`
	Disabled       bool      `json:"disabled" gorm:"column:disabled;default:false"`
	CreatedAt      time.Time `json:"created_at" gorm:"column:created_at"`
	UpdatedAt      time.Time `json:"updated_at" gorm:"column:updated_at"`
}

func (SystemUser) TableName() string { return "system_users" }

type ZentaoBinding struct {
	ID            int64     `json:"id" gorm:"primaryKey;column:id"`
	SystemUserID  int64     `json:"system_user_id" gorm:"uniqueIndex;column:system_user_id"`
	ZentaoAccount string    `json:"zentao_account" gorm:"uniqueIndex;column:zentao_account"`
	CreatedAt     time.Time `json:"created_at" gorm:"column:created_at"`
	UpdatedAt     time.Time `json:"updated_at" gorm:"column:updated_at"`
}

func (ZentaoBinding) TableName() string { return "zentao_bindings" }

type AuditLog struct {
	ID            int64     `json:"id" gorm:"primaryKey;column:id"`
	ActorUserID   *int64    `json:"actor_user_id" gorm:"column:actor_user_id"`
	ActorUsername string    `json:"actor_username" gorm:"column:actor_username"`
	Action        string    `json:"action" gorm:"column:action"`
	TargetType    string    `json:"target_type" gorm:"column:target_type"`
	TargetID      string    `json:"target_id" gorm:"column:target_id"`
	Metadata      JSONB     `json:"metadata" gorm:"column:metadata;type:jsonb"`
	IP            string    `json:"ip" gorm:"column:ip"`
	UA            string    `json:"ua" gorm:"column:ua"`
	CreatedAt     time.Time `json:"created_at" gorm:"column:created_at"`
}

func (AuditLog) TableName() string { return "audit_logs" }

// SyncRunLog records each ETL or effort-reconcile execution.
type SyncRunLog struct {
	ID            int64      `json:"id" gorm:"primaryKey;column:id"`
	JobType       string     `json:"job_type" gorm:"column:job_type"`
	TriggerSource string     `json:"trigger_source" gorm:"column:trigger_source"`
	Status        string     `json:"status" gorm:"column:status"`
	Message       string     `json:"message,omitempty" gorm:"column:message"`
	ActorUsername string     `json:"actor_username,omitempty" gorm:"column:actor_username"`
	Metadata      JSONB      `json:"metadata,omitempty" gorm:"column:metadata;type:jsonb"`
	StartedAt     time.Time  `json:"started_at" gorm:"column:started_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty" gorm:"column:finished_at"`
	DurationMs    *int64     `json:"duration_ms,omitempty" gorm:"column:duration_ms"`
	DisplayName   string     `json:"display_name" gorm:"-"`
	StatusLabel   string     `json:"status_label" gorm:"-"`
}

func (SyncRunLog) TableName() string { return "sync_run_logs" }
