package db

import (
	"fmt"
	"time"
)

// MCPEffortLog mirrors the mcp_effort_logs table.
type MCPEffortLog struct {
	ID              int64     `gorm:"primaryKey;column:id"`
	ClientRequestID string    `gorm:"column:client_request_id"`
	ActorUsername   string    `gorm:"column:actor_username"`
	ObjectType      string    `gorm:"column:object_type"`
	ObjectID        int64     `gorm:"column:object_id"`
	WorkDate        string    `gorm:"column:work_date"`
	Consumed        float64   `gorm:"column:consumed"`
	Work            string    `gorm:"column:work"`
	Status          string    `gorm:"column:status"`
	ZentaoEffortID  int64     `gorm:"column:zentao_effort_id"`
	ZentaoMode      string    `gorm:"column:zentao_mode"`
	ErrorMessage    string    `gorm:"column:error_message"`
	RetryCount      int       `gorm:"column:retry_count"`
	CreatedAt       time.Time `gorm:"column:created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at"`
}

func (MCPEffortLog) TableName() string { return "mcp_effort_logs" }

// MCPEffortLogInput holds the fields needed to create a new log entry.
type MCPEffortLogInput struct {
	ClientRequestID string
	ActorUsername   string
	ObjectType      string
	ObjectID        int64
	WorkDate        string
	Consumed        float64
	Work            string
}

// GetMCPEffortLog looks up an existing log by client_request_id for idempotency.
func GetMCPEffortLog(clientReqID string) (*MCPEffortLog, bool, error) {
	if PG == nil {
		return nil, false, fmt.Errorf("db not initialized")
	}
	var row MCPEffortLog
	res := PG.Where("client_request_id = ?", clientReqID).First(&row)
	if res.Error != nil {
		if isNotFound(res.Error) {
			return nil, false, nil
		}
		return nil, false, res.Error
	}
	return &row, true, nil
}

// CreateMCPEffortLog inserts a new log row in "pending" status and returns its ID.
func CreateMCPEffortLog(in MCPEffortLogInput) (int64, error) {
	if PG == nil {
		return 0, fmt.Errorf("db not initialized")
	}
	row := MCPEffortLog{
		ClientRequestID: in.ClientRequestID,
		ActorUsername:   in.ActorUsername,
		ObjectType:      in.ObjectType,
		ObjectID:        in.ObjectID,
		WorkDate:        in.WorkDate,
		Consumed:        in.Consumed,
		Work:            in.Work,
		Status:          "pending",
	}
	if err := PG.Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

// SucceedMCPEffortLog marks the log row as successful.
func SucceedMCPEffortLog(id, zentaoEffortID int64, mode string) error {
	if PG == nil {
		return fmt.Errorf("db not initialized")
	}
	return PG.Model(&MCPEffortLog{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":           "success",
		"zentao_effort_id": zentaoEffortID,
		"zentao_mode":      mode,
		"error_message":    "",
		"updated_at":       time.Now(),
	}).Error
}

// FailMCPEffortLog marks the log row as failed and stores the error reason.
func FailMCPEffortLog(id int64, errMsg string) error {
	if PG == nil {
		return fmt.Errorf("db not initialized")
	}
	return PG.Model(&MCPEffortLog{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":        "failed",
		"error_message": errMsg,
		"retry_count":   PG.Raw("retry_count + 1"),
		"updated_at":    time.Now(),
	}).Error
}

// isNotFound checks if a GORM error is a "record not found" error.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	return err.Error() == "record not found"
}
