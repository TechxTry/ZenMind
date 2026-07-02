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

// ---- listMyBugs ----

type ListMyBugsTool struct{}

func (t ListMyBugsTool) Definition() ToolDef {
	return ToolDef{
		Name:        "listMyBugs",
		Description: "查询当前归属给我的缺陷列表（来自本地同步数据），支持状态、严重级别和结构维度筛选。",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"id":           {Type: "number", Description: "缺陷 ID，精确匹配，可选"},
				"status":       {Type: "string", Description: "缺陷状态过滤：active | resolved | closed | wait | activating，可选"},
				"severity":     {Type: "number", Description: "严重级别（整数），可选"},
				"execution_id": {Type: "number", Description: "按迭代 ID 过滤，可选"},
				"project_id":   {Type: "number", Description: "按项目 ID 过滤（通过迭代所属项目），可选"},
				"program_id":   {Type: "number", Description: "按项目集 ID 过滤（项目集→项目→迭代），可选"},
				"product_id":   {Type: "number", Description: "按产品 ID 过滤（通过关联需求），可选"},
				"limit":        {Type: "number", Description: "最多返回条数，默认 50，最大 200"},
			},
		},
	}
}

func (t ListMyBugsTool) Execute(_ context.Context, caller CallerInfo, args map[string]interface{}) ToolCallResult {
	limit := int(floatArg(args, "limit"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	recordID := int64(floatArg(args, "id"))
	status := strings.TrimSpace(stringArg(args, "status"))
	severity := int(floatArg(args, "severity"))
	execID := int64(floatArg(args, "execution_id"))
	projectID := int64(floatArg(args, "project_id"))
	programID := int64(floatArg(args, "program_id"))
	productID := int64(floatArg(args, "product_id"))

	pg := db.PG
	if pg == nil {
		return ErrorResult("db not initialized")
	}

	zentaoAccount, err := resolveZentaoAccount(caller.Username)
	if err != nil {
		return ErrorResult("无法确认禅道账号：" + err.Error())
	}

	query := pg.Model(&models.LocalBug{}).
		Where("deleted = false").
		Where("assigned_to = ?", zentaoAccount)

	if recordID > 0 {
		query = query.Where("id = ?", recordID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if severity > 0 {
		query = query.Where("severity = ?", severity)
	}
	if execID > 0 {
		query = query.Where("execution_id = ?", execID)
	}
	if projectID > 0 {
		query = query.Where("execution_id IN (SELECT id FROM local_executions WHERE deleted = false AND parent_id = ?)", projectID)
	}
	if programID > 0 {
		query = query.Where(`
			execution_id IN (
				SELECT e.id FROM local_executions e
				JOIN local_projects p ON p.id = e.parent_id
				WHERE e.deleted = false AND p.deleted = false AND p.parent_id = ?
			)`, programID)
	}
	if productID > 0 {
		query = query.Where("story_id IN (SELECT id FROM local_stories WHERE deleted = false AND product_id = ?)", productID)
	}

	var rows []models.LocalBug
	if err := query.Order("last_edited_date DESC, id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return ErrorResult("query failed: " + err.Error())
	}

	type row struct {
		ID             int64  `json:"id"`
		Title          string `json:"title"`
		Severity       int    `json:"severity"`
		Status         string `json:"status"`
		AssignedTo     string `json:"assigned_to"`
		ResolvedBy     string `json:"resolved_by"`
		Resolution     string `json:"resolution"`
		ExecutionID    int64  `json:"execution_id"`
		StoryID        int64  `json:"story_id"`
		TaskID         int64  `json:"task_id"`
		OpenedDate     string `json:"opened_date"`
		ResolvedDate   string `json:"resolved_date"`
		ClosedDate     string `json:"closed_date"`
		LastEditedDate string `json:"last_edited_date"`
	}

	result := make([]row, 0, len(rows))
	for _, b := range rows {
		item := row{
			ID:          b.ID,
			Title:       b.Title,
			Severity:    b.Severity,
			Status:      b.Status,
			AssignedTo:  b.AssignedTo,
			ResolvedBy:  b.ResolvedBy,
			Resolution:  b.Resolution,
			ExecutionID: b.ExecutionID,
			StoryID:     b.StoryID,
			TaskID:      b.TaskID,
		}
		if b.OpenedDate != nil {
			item.OpenedDate = b.OpenedDate.Format("2006-01-02")
		}
		if b.ResolvedDate != nil {
			item.ResolvedDate = b.ResolvedDate.Format("2006-01-02")
		}
		if b.ClosedDate != nil {
			item.ClosedDate = b.ClosedDate.Format("2006-01-02")
		}
		if b.LastEditedDate != nil {
			item.LastEditedDate = b.LastEditedDate.Format("2006-01-02 15:04:05")
		}
		result = append(result, item)
	}

	out, _ := json.Marshal(map[string]any{
		"bugs":  result,
		"count": len(result),
		"filters": map[string]any{
			"id":           recordID,
			"status":       status,
			"severity":     severity,
			"execution_id": execID,
			"project_id":   projectID,
			"program_id":   programID,
			"product_id":   productID,
			"limit":        limit,
		},
	})
	return TextResult(string(out))
}

// ---- listMyStories ----

type ListMyStoriesTool struct{}

func (t ListMyStoriesTool) Definition() ToolDef {
	return ToolDef{
		Name:        "listMyStories",
		Description: "查询当前归属给我的需求列表（来自本地同步数据），支持状态与结构维度筛选。",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"id":           {Type: "number", Description: "需求 ID，精确匹配，可选"},
				"status":       {Type: "string", Description: "需求状态过滤：draft|reviewing|active|changing|changed|closed，可选"},
				"execution_id": {Type: "number", Description: "按迭代 ID 过滤（通过任务/缺陷关联需求），可选"},
				"product_id":   {Type: "number", Description: "按产品 ID 过滤，可选"},
				"limit":        {Type: "number", Description: "最多返回条数，默认 50，最大 200"},
			},
		},
	}
}

func (t ListMyStoriesTool) Execute(_ context.Context, caller CallerInfo, args map[string]interface{}) ToolCallResult {
	limit := int(floatArg(args, "limit"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	recordID := int64(floatArg(args, "id"))
	status := strings.TrimSpace(stringArg(args, "status"))
	execID := int64(floatArg(args, "execution_id"))
	productID := int64(floatArg(args, "product_id"))

	pg := db.PG
	if pg == nil {
		return ErrorResult("db not initialized")
	}

	zentaoAccount, err := resolveZentaoAccount(caller.Username)
	if err != nil {
		return ErrorResult("无法确认禅道账号：" + err.Error())
	}

	query := pg.Model(&models.LocalStory{}).
		Where("deleted = false").
		Where("assigned_to = ?", zentaoAccount)

	if recordID > 0 {
		query = query.Where("id = ?", recordID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if execID > 0 {
		query = query.Where(`
			id IN (
				SELECT DISTINCT story_id FROM local_tasks
				WHERE deleted = false AND execution_id = ? AND story_id > 0
				UNION
				SELECT DISTINCT story_id FROM local_bugs
				WHERE deleted = false AND execution_id = ? AND story_id > 0
			)`, execID, execID)
	}
	if productID > 0 {
		query = query.Where("product_id = ?", productID)
	}

	var rows []models.LocalStory
	if err := query.Order("last_edited_date DESC, id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return ErrorResult("query failed: " + err.Error())
	}

	type row struct {
		ID             int64   `json:"id"`
		Title          string  `json:"title"`
		Status         string  `json:"status"`
		AssignedTo     string  `json:"assigned_to"`
		Estimate       float64 `json:"estimate"`
		ProductID      int64   `json:"product_id"`
		OpenedDate     string  `json:"opened_date"`
		ClosedDate     string  `json:"closed_date"`
		LastEditedDate string  `json:"last_edited_date"`
	}

	result := make([]row, 0, len(rows))
	for _, s := range rows {
		item := row{
			ID:         s.ID,
			Title:      s.Title,
			Status:     s.Status,
			AssignedTo: s.AssignedTo,
			Estimate:   s.Estimate,
			ProductID:  s.ProductID,
		}
		if s.OpenedDate != nil {
			item.OpenedDate = s.OpenedDate.Format("2006-01-02")
		}
		if s.ClosedDate != nil {
			item.ClosedDate = s.ClosedDate.Format("2006-01-02")
		}
		if s.LastEditedDate != nil {
			item.LastEditedDate = s.LastEditedDate.Format("2006-01-02 15:04:05")
		}
		result = append(result, item)
	}

	out, _ := json.Marshal(map[string]any{
		"stories": result,
		"count":   len(result),
		"filters": map[string]any{
			"id":           recordID,
			"status":       status,
			"execution_id": execID,
			"product_id":   productID,
			"limit":        limit,
		},
	})
	return TextResult(string(out))
}

// ---- listMyExecutions ----

type ListMyExecutionsTool struct{}

func (t ListMyExecutionsTool) Definition() ToolDef {
	return ToolDef{
		Name:        "listMyExecutions",
		Description: "查询我可见的执行（迭代）列表，可用于创建任务时选择 execution_id。",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"status":     {Type: "string", Description: "执行状态过滤（如 wait | doing | suspended | closed）"},
				"project_id": {Type: "number", Description: "按所属项目 ID 过滤，可选"},
				"name":       {Type: "string", Description: "执行名称模糊匹配，可选"},
				"limit":      {Type: "number", Description: "最多返回条数，默认 30，最大 100"},
			},
		},
	}
}

func (t ListMyExecutionsTool) Execute(_ context.Context, caller CallerInfo, args map[string]interface{}) ToolCallResult {
	status := strings.TrimSpace(stringArg(args, "status"))
	name := strings.TrimSpace(stringArg(args, "name"))
	projectID := int64(floatArg(args, "project_id"))
	limit := int(floatArg(args, "limit"))
	if limit <= 0 {
		limit = 30
	}
	if limit > 100 {
		limit = 100
	}

	pg := db.PG
	if pg == nil {
		return ErrorResult("db not initialized")
	}

	zentaoAccount, err := resolveZentaoAccount(caller.Username)
	if err != nil {
		return ErrorResult("无法确认禅道账号：" + err.Error())
	}

	query := pg.Model(&models.LocalExecution{}).
		Where("deleted = false").
		Where(`
			id IN (
				SELECT DISTINCT execution_id FROM local_tasks
				WHERE deleted = false AND execution_id > 0 AND assigned_to = ?
				UNION
				SELECT DISTINCT execution_id FROM local_bugs
				WHERE deleted = false AND execution_id > 0 AND assigned_to = ?
			)`, zentaoAccount, zentaoAccount)

	if status != "" {
		query = query.Where("status = ?", status)
	}
	if projectID > 0 {
		query = query.Where("parent_id = ?", projectID)
	}
	if name != "" {
		like := "%" + strings.ToLower(name) + "%"
		query = query.Where("LOWER(name) LIKE ?", like)
	}

	var rows []models.LocalExecution
	if err := query.Order("begin_date DESC, id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return ErrorResult("query failed: " + err.Error())
	}

	type row struct {
		ID        int64  `json:"id"`
		Name      string `json:"name"`
		Status    string `json:"status"`
		ProjectID int64  `json:"project_id"`
		BeginDate string `json:"begin_date"`
		EndDate   string `json:"end_date"`
	}

	result := make([]row, 0, len(rows))
	for _, e := range rows {
		var pid int64
		if e.ParentID != nil {
			pid = *e.ParentID
		}
		beginDate := ""
		if e.BeginDate != nil {
			beginDate = e.BeginDate.Format("2006-01-02")
		}
		endDate := ""
		if e.EndDate != nil {
			endDate = e.EndDate.Format("2006-01-02")
		}

		result = append(result, row{
			ID:        e.ID,
			Name:      e.Name,
			Status:    e.Status,
			ProjectID: pid,
			BeginDate: beginDate,
			EndDate:   endDate,
		})
	}

	b, _ := json.Marshal(map[string]interface{}{
		"executions": result,
		"count":      len(result),
	})
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
