package mcp

import (
	"context"
	"encoding/json"
	"strings"

	"zenmind/internal/db"
	"zenmind/internal/models"
)

// ListTasksTool 查询全量任务，不按当前调用者的指派账号过滤。
type ListTasksTool struct{}

func (t ListTasksTool) Definition() ToolDef {
	return ToolDef{
		Name:        "listTasks",
		Description: "查询任务列表（来自本地同步数据，不限当前用户负责的任务），支持按人员、迭代、项目集、产品和产品计划筛选。",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"id":           {Type: "number", Description: "任务 ID，精确匹配，可选"},
				"status":       {Type: "string", Description: "任务状态过滤：wait|doing|done|closed|pause|cancel，可选"},
				"assigned_to":  {Type: "string", Description: "按指派禅道账号过滤，可选"},
				"execution_id": {Type: "number", Description: "按迭代 ID 过滤，可选"},
				"project_id":   {Type: "number", Description: "按项目 ID 过滤，可选"},
				"program_id":   {Type: "number", Description: "按项目集 ID 过滤，可选"},
				"product_id":   {Type: "number", Description: "按关联需求的产品 ID 过滤，可选"},
				"plan_id":      {Type: "number", Description: "按关联需求的产品计划 ID 过滤，可选"},
				"story_id":     {Type: "number", Description: "按关联需求 ID 过滤，可选"},
				"limit":        {Type: "number", Description: "最多返回条数，默认 50，最大 200"},
			},
		},
	}
}

func (t ListTasksTool) Execute(_ context.Context, _ CallerInfo, args map[string]interface{}) ToolCallResult {
	limit := queryLimit(args, 50, 200)
	recordID := int64(floatArg(args, "id"))
	status := strings.TrimSpace(stringArg(args, "status"))
	assignedTo := strings.TrimSpace(stringArg(args, "assigned_to"))
	executionID := int64(floatArg(args, "execution_id"))
	projectID := int64(floatArg(args, "project_id"))
	programID := int64(floatArg(args, "program_id"))
	productID := int64(floatArg(args, "product_id"))
	planID := planIDArg(args)
	storyID := int64(floatArg(args, "story_id"))

	pg := db.PG
	if pg == nil {
		return ErrorResult("db not initialized")
	}

	query := pg.Model(&models.LocalTask{}).Where("deleted = false")
	if recordID > 0 {
		query = query.Where("id = ?", recordID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if assignedTo != "" {
		query = query.Where("assigned_to = ?", assignedTo)
	}
	if executionID > 0 {
		query = query.Where("execution_id = ?", executionID)
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
	if planID > 0 {
		storyIDs, err := queryZentaoIDsByPlan("zt_story", planID, true)
		if err != nil {
			return ErrorResult(err.Error())
		}
		if len(storyIDs) == 0 {
			query = query.Where("1 = 0")
		} else {
			query = query.Where("story_id IN ?", storyIDs)
		}
	}
	if storyID > 0 {
		query = query.Where("story_id = ?", storyID)
	}

	var tasks []models.LocalTask
	if err := query.Order("last_edited_date DESC, id DESC").Limit(limit).Find(&tasks).Error; err != nil {
		return ErrorResult("query failed: " + err.Error())
	}

	type row struct {
		ID          int64   `json:"id"`
		Name        string  `json:"name"`
		Type        string  `json:"type"`
		Status      string  `json:"status"`
		AssignedTo  string  `json:"assigned_to"`
		Estimate    float64 `json:"estimate"`
		Consumed    float64 `json:"consumed"`
		ExecutionID int64   `json:"execution_id"`
		StoryID     int64   `json:"story_id"`
	}
	result := make([]row, 0, len(tasks))
	for _, task := range tasks {
		result = append(result, row{
			ID:          task.ID,
			Name:        task.Name,
			Type:        task.Type,
			Status:      task.Status,
			AssignedTo:  task.AssignedTo,
			Estimate:    task.Estimate,
			Consumed:    task.Consumed,
			ExecutionID: task.ExecutionID,
			StoryID:     task.StoryID,
		})
	}

	out, _ := json.Marshal(map[string]any{
		"tasks": result,
		"count": len(result),
		"filters": map[string]any{
			"id":           recordID,
			"status":       status,
			"assigned_to":  assignedTo,
			"execution_id": executionID,
			"project_id":   projectID,
			"program_id":   programID,
			"product_id":   productID,
			"plan_id":      planID,
			"story_id":     storyID,
			"limit":        limit,
		},
	})
	return TextResult(string(out))
}

// ListBugsTool 查询全量缺陷，不按当前调用者的指派账号过滤。
type ListBugsTool struct{}

func (t ListBugsTool) Definition() ToolDef {
	return ToolDef{
		Name:        "listBugs",
		Description: "查询缺陷列表（来自本地同步数据，不限当前用户负责的缺陷），支持按人员和结构维度筛选。",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"id":           {Type: "number", Description: "缺陷 ID，精确匹配，可选"},
				"status":       {Type: "string", Description: "缺陷状态过滤：active|resolved|closed|wait|activating，可选"},
				"severity":     {Type: "number", Description: "严重级别（整数），可选"},
				"assigned_to":  {Type: "string", Description: "按指派禅道账号过滤，可选"},
				"execution_id": {Type: "number", Description: "按迭代 ID 过滤，可选"},
				"project_id":   {Type: "number", Description: "按项目 ID 过滤，可选"},
				"program_id":   {Type: "number", Description: "按项目集 ID 过滤，可选"},
				"product_id":   {Type: "number", Description: "按产品 ID 过滤（通过关联需求），可选"},
				"plan_id":      {Type: "number", Description: "按产品计划 ID 过滤，可选"},
				"story_id":     {Type: "number", Description: "按关联需求 ID 过滤，可选"},
				"limit":        {Type: "number", Description: "最多返回条数，默认 50，最大 200"},
			},
		},
	}
}

func (t ListBugsTool) Execute(_ context.Context, _ CallerInfo, args map[string]interface{}) ToolCallResult {
	limit := queryLimit(args, 50, 200)
	recordID := int64(floatArg(args, "id"))
	status := strings.TrimSpace(stringArg(args, "status"))
	severity := int(floatArg(args, "severity"))
	assignedTo := strings.TrimSpace(stringArg(args, "assigned_to"))
	executionID := int64(floatArg(args, "execution_id"))
	projectID := int64(floatArg(args, "project_id"))
	programID := int64(floatArg(args, "program_id"))
	productID := int64(floatArg(args, "product_id"))
	planID := planIDArg(args)
	storyID := int64(floatArg(args, "story_id"))

	pg := db.PG
	if pg == nil {
		return ErrorResult("db not initialized")
	}

	query := pg.Model(&models.LocalBug{}).Where("deleted = false")
	if recordID > 0 {
		query = query.Where("id = ?", recordID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if severity > 0 {
		query = query.Where("severity = ?", severity)
	}
	if assignedTo != "" {
		query = query.Where("assigned_to = ?", assignedTo)
	}
	if executionID > 0 {
		query = query.Where("execution_id = ?", executionID)
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
	if planID > 0 {
		ids, err := queryZentaoIDsByPlan("zt_bug", planID, false)
		if err != nil {
			return ErrorResult(err.Error())
		}
		if len(ids) == 0 {
			query = query.Where("1 = 0")
		} else {
			query = query.Where("id IN ?", ids)
		}
	}
	if storyID > 0 {
		query = query.Where("story_id = ?", storyID)
	}

	var bugs []models.LocalBug
	if err := query.Order("last_edited_date DESC, id DESC").Limit(limit).Find(&bugs).Error; err != nil {
		return ErrorResult("query failed: " + err.Error())
	}

	type row struct {
		ID          int64  `json:"id"`
		Title       string `json:"title"`
		Severity    int    `json:"severity"`
		Status      string `json:"status"`
		AssignedTo  string `json:"assigned_to"`
		ResolvedBy  string `json:"resolved_by"`
		Resolution  string `json:"resolution"`
		ExecutionID int64  `json:"execution_id"`
		StoryID     int64  `json:"story_id"`
		TaskID      int64  `json:"task_id"`
		PlanID      int64  `json:"plan_id"`
	}
	bugIDs := make([]int64, 0, len(bugs))
	for _, bug := range bugs {
		bugIDs = append(bugIDs, bug.ID)
	}
	planByID := lookupZentaoPlanIDByIDs("zt_bug", bugIDs)
	result := make([]row, 0, len(bugs))
	for _, bug := range bugs {
		result = append(result, row{
			ID:          bug.ID,
			Title:       bug.Title,
			Severity:    bug.Severity,
			Status:      bug.Status,
			AssignedTo:  bug.AssignedTo,
			ResolvedBy:  bug.ResolvedBy,
			Resolution:  bug.Resolution,
			ExecutionID: bug.ExecutionID,
			StoryID:     bug.StoryID,
			TaskID:      bug.TaskID,
			PlanID:      planByID[bug.ID],
		})
	}

	out, _ := json.Marshal(map[string]any{
		"bugs":  result,
		"count": len(result),
		"filters": map[string]any{
			"id":           recordID,
			"status":       status,
			"severity":     severity,
			"assigned_to":  assignedTo,
			"execution_id": executionID,
			"project_id":   projectID,
			"program_id":   programID,
			"product_id":   productID,
			"plan_id":      planID,
			"story_id":     storyID,
			"limit":        limit,
		},
	})
	return TextResult(string(out))
}

// ListStoriesTool 查询全量需求，不按当前调用者的指派账号过滤。
type ListStoriesTool struct{}

func (t ListStoriesTool) Definition() ToolDef {
	return ToolDef{
		Name:        "listStories",
		Description: "查询需求列表（来自本地同步数据，不限当前用户负责的需求），支持按人员、产品、产品计划和项目结构筛选。",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"id":           {Type: "number", Description: "需求 ID，精确匹配，可选"},
				"status":       {Type: "string", Description: "需求状态过滤：draft|reviewing|active|changing|changed|closed，可选"},
				"assigned_to":  {Type: "string", Description: "按指派禅道账号过滤，可选"},
				"execution_id": {Type: "number", Description: "按关联迭代 ID 过滤，可选"},
				"project_id":   {Type: "number", Description: "按关联项目 ID 过滤，可选"},
				"program_id":   {Type: "number", Description: "按关联项目集 ID 过滤，可选"},
				"product_id":   {Type: "number", Description: "按产品 ID 过滤，可选"},
				"plan_id":      {Type: "number", Description: "按产品计划 ID 过滤，可选"},
				"limit":        {Type: "number", Description: "最多返回条数，默认 50，最大 200"},
			},
		},
	}
}

func (t ListStoriesTool) Execute(_ context.Context, _ CallerInfo, args map[string]interface{}) ToolCallResult {
	limit := queryLimit(args, 50, 200)
	recordID := int64(floatArg(args, "id"))
	status := strings.TrimSpace(stringArg(args, "status"))
	assignedTo := strings.TrimSpace(stringArg(args, "assigned_to"))
	executionID := int64(floatArg(args, "execution_id"))
	projectID := int64(floatArg(args, "project_id"))
	programID := int64(floatArg(args, "program_id"))
	productID := int64(floatArg(args, "product_id"))
	planID := planIDArg(args)

	pg := db.PG
	if pg == nil {
		return ErrorResult("db not initialized")
	}

	query := pg.Model(&models.LocalStory{}).Where("deleted = false")
	if recordID > 0 {
		query = query.Where("id = ?", recordID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if assignedTo != "" {
		query = query.Where("assigned_to = ?", assignedTo)
	}
	if executionID > 0 {
		query = query.Where(`
			id IN (
				SELECT DISTINCT story_id FROM local_tasks
				WHERE deleted = false AND execution_id = ? AND story_id > 0
				UNION
				SELECT DISTINCT story_id FROM local_bugs
				WHERE deleted = false AND execution_id = ? AND story_id > 0
			)`, executionID, executionID)
	}
	if projectID > 0 {
		query = query.Where(`
			id IN (
				SELECT DISTINCT t.story_id FROM local_tasks t
				JOIN local_executions e ON e.id = t.execution_id
				WHERE t.deleted = false AND e.deleted = false AND e.parent_id = ? AND t.story_id > 0
				UNION
				SELECT DISTINCT b.story_id FROM local_bugs b
				JOIN local_executions e ON e.id = b.execution_id
				WHERE b.deleted = false AND e.deleted = false AND e.parent_id = ? AND b.story_id > 0
			)`, projectID, projectID)
	}
	if programID > 0 {
		query = query.Where(`
			id IN (
				SELECT DISTINCT t.story_id FROM local_tasks t
				JOIN local_executions e ON e.id = t.execution_id
				JOIN local_projects p ON p.id = e.parent_id
				WHERE t.deleted = false AND e.deleted = false AND p.deleted = false AND p.parent_id = ? AND t.story_id > 0
				UNION
				SELECT DISTINCT b.story_id FROM local_bugs b
				JOIN local_executions e ON e.id = b.execution_id
				JOIN local_projects p ON p.id = e.parent_id
				WHERE b.deleted = false AND e.deleted = false AND p.deleted = false AND p.parent_id = ? AND b.story_id > 0
			)`, programID, programID)
	}
	if productID > 0 {
		query = query.Where("product_id = ?", productID)
	}
	if planID > 0 {
		ids, err := queryZentaoIDsByPlan("zt_story", planID, true)
		if err != nil {
			return ErrorResult(err.Error())
		}
		if len(ids) == 0 {
			query = query.Where("1 = 0")
		} else {
			query = query.Where("id IN ?", ids)
		}
	}

	var stories []models.LocalStory
	if err := query.Order("last_edited_date DESC, id DESC").Limit(limit).Find(&stories).Error; err != nil {
		return ErrorResult("query failed: " + err.Error())
	}

	type row struct {
		ID         int64   `json:"id"`
		Title      string  `json:"title"`
		Status     string  `json:"status"`
		AssignedTo string  `json:"assigned_to"`
		Estimate   float64 `json:"estimate"`
		ProductID  int64   `json:"product_id"`
		PlanID     int64   `json:"plan_id"`
	}
	storyIDs := make([]int64, 0, len(stories))
	for _, story := range stories {
		storyIDs = append(storyIDs, story.ID)
	}
	planByID := lookupZentaoPlanIDByIDs("zt_story", storyIDs)
	result := make([]row, 0, len(stories))
	for _, story := range stories {
		result = append(result, row{
			ID:         story.ID,
			Title:      story.Title,
			Status:     story.Status,
			AssignedTo: story.AssignedTo,
			Estimate:   story.Estimate,
			ProductID:  story.ProductID,
			PlanID:     planByID[story.ID],
		})
	}

	out, _ := json.Marshal(map[string]any{
		"stories": result,
		"count":   len(result),
		"filters": map[string]any{
			"id":           recordID,
			"status":       status,
			"assigned_to":  assignedTo,
			"execution_id": executionID,
			"project_id":   projectID,
			"program_id":   programID,
			"product_id":   productID,
			"plan_id":      planID,
			"limit":        limit,
		},
	})
	return TextResult(string(out))
}

func queryLimit(args map[string]interface{}, defaultLimit, maxLimit int) int {
	limit := int(floatArg(args, "limit"))
	if limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}
