package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"zenmind/internal/db"
	"zenmind/internal/models"

	"gorm.io/gorm"
)

// ---- listMyTasks ----

type ListMyTasksTool struct{}

func (t ListMyTasksTool) Definition() ToolDef {
	return ToolDef{
		Name:        "listMyTasks",
		Description: "查询我当前负责的任务列表（来自本地同步数据），可按状态筛选。",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"status": {Type: "string", Description: "任务状态过滤：wait | doing | done | closed，留空返回 wait+doing"},
				"limit":  {Type: "number", Description: "最多返回条数，默认 20，最大 50"},
			},
		},
	}
}

func (t ListMyTasksTool) Execute(_ context.Context, caller CallerInfo, args map[string]interface{}) ToolCallResult {
	status := strings.TrimSpace(stringArg(args, "status"))
	limit := int(floatArg(args, "limit"))
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	pg := db.PG
	if pg == nil {
		return ErrorResult("db not initialized")
	}

	// resolve zentao account for this system user
	zentaoAccount, err := resolveZentaoAccount(caller.Username)
	if err != nil {
		return ErrorResult("无法确认禅道账号：" + err.Error())
	}

	q := pg.Model(&models.LocalTask{}).
		Where("deleted = false").
		Where("assigned_to = ?", zentaoAccount)

	if status != "" {
		q = q.Where("status = ?", status)
	} else {
		q = q.Where("status IN ?", []string{"wait", "doing"})
	}

	var tasks []models.LocalTask
	if err := q.Order("id DESC").Limit(limit).Find(&tasks).Error; err != nil {
		return ErrorResult("query failed: " + err.Error())
	}

	type row struct {
		ID          int64   `json:"id"`
		Name        string  `json:"name"`
		Status      string  `json:"status"`
		Estimate    float64 `json:"estimate"`
		Consumed    float64 `json:"consumed"`
		ExecutionID int64   `json:"execution_id"`
	}
	result := make([]row, 0, len(tasks))
	for _, tk := range tasks {
		result = append(result, row{
			ID:          tk.ID,
			Name:        tk.Name,
			Status:      tk.Status,
			Estimate:    tk.Estimate,
			Consumed:    tk.Consumed,
			ExecutionID: tk.ExecutionID,
		})
	}
	b, _ := json.Marshal(map[string]interface{}{"tasks": result, "count": len(result)})
	return TextResult(string(b))
}

// ---- listMyEfforts ----

type ListMyEffortsTool struct{}

func (t ListMyEffortsTool) Definition() ToolDef {
	return ToolDef{
		Name:        "listMyEfforts",
		Description: "查询我的历史报工记录（来自本地同步数据），可按日期范围筛选。",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"start_date": {Type: "string", Description: "起始日期 YYYY-MM-DD，默认本月第一天"},
				"end_date":   {Type: "string", Description: "截止日期 YYYY-MM-DD，默认今天"},
				"limit":      {Type: "number", Description: "最多返回条数，默认 50，最大 200"},
			},
		},
	}
}

func (t ListMyEffortsTool) Execute(_ context.Context, caller CallerInfo, args map[string]interface{}) ToolCallResult {
	now := time.Now()
	startDate := strings.TrimSpace(stringArg(args, "start_date"))
	if startDate == "" {
		startDate = fmt.Sprintf("%d-%02d-01", now.Year(), now.Month())
	}
	endDate := strings.TrimSpace(stringArg(args, "end_date"))
	if endDate == "" {
		endDate = now.Format("2006-01-02")
	}
	limit := int(floatArg(args, "limit"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	pg := db.PG
	if pg == nil {
		return ErrorResult("db not initialized")
	}

	zentaoAccount, err := resolveZentaoAccount(caller.Username)
	if err != nil {
		return ErrorResult("无法确认禅道账号：" + err.Error())
	}

	var efforts []models.LocalEffort
	q := pg.Model(&models.LocalEffort{}).
		Where("deleted = false").
		Where("account = ?", zentaoAccount).
		Where("work_date >= ?", startDate).
		Where("work_date <= ?", endDate)

	if err := q.Order("work_date DESC, id DESC").Limit(limit).Find(&efforts).Error; err != nil {
		return ErrorResult("query failed: " + err.Error())
	}

	type row struct {
		ID         int64   `json:"id"`
		WorkDate   string  `json:"work_date"`
		Consumed   float64 `json:"consumed"`
		Work       string  `json:"work"`
		ObjectType string  `json:"object_type"`
		ObjectID   int64   `json:"object_id"`
	}
	result := make([]row, 0, len(efforts))
	for _, e := range efforts {
		wd := ""
		if e.WorkDate != nil {
			wd = e.WorkDate.Format("2006-01-02")
		}
		result = append(result, row{
			ID:         e.ID,
			WorkDate:   wd,
			Consumed:   e.Consumed,
			Work:       e.Work,
			ObjectType: e.ObjectType,
			ObjectID:   e.ObjectID,
		})
	}

	var total float64
	for _, r := range result {
		total += r.Consumed
	}

	b, _ := json.Marshal(map[string]interface{}{
		"efforts":          result,
		"count":            len(result),
		"total_consumed_h": total,
		"start_date":       startDate,
		"end_date":         endDate,
	})
	return TextResult(string(b))
}

// ---- shared helper ----

func resolveZentaoAccount(username string) (string, error) {
	pg := db.PG
	if pg == nil {
		return "", fmt.Errorf("db not initialized")
	}

	var su models.SystemUser
	if err := pg.Where("username = ?", username).First(&su).Error; err != nil {
		if isGormNotFound(err) {
			return "", fmt.Errorf("用户不存在")
		}
		return "", err
	}

	var binding models.ZentaoBinding
	if err := pg.Where("system_user_id = ?", su.ID).First(&binding).Error; err != nil {
		if isGormNotFound(err) {
			return "", fmt.Errorf("未绑定禅道账号，请先在「禅道授权」页面绑定")
		}
		return "", err
	}

	acct := strings.TrimSpace(binding.ZentaoAccount)
	if acct == "" {
		return "", fmt.Errorf("禅道账号绑定为空")
	}
	return acct, nil
}

func isGormNotFound(err error) bool {
	return err != nil && err == gorm.ErrRecordNotFound
}
