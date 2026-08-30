package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"zenmind/internal/config"
	"zenmind/internal/db"
	"zenmind/internal/etl"
	"zenmind/internal/models"
	"zenmind/internal/zentao"
	"zenmind/internal/zentaoauth"
)

type CreateTaskTool struct{}

func (t CreateTaskTool) Definition() ToolDef {
	return ToolDef{
		Name:        "createTask",
		Description: "创建任务（必填：所属执行 execution_id、任务类型 type、任务标题 name、截止日期 deadline；其余字段可选）。",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"execution_id": {Type: "number", Description: "所属执行（迭代）ID，必填"},
				"project_id":   {Type: "number", Description: "项目 ID，可选"},
				"name":         {Type: "string", Description: "任务标题"},
				"type":         {Type: "string", Description: "任务类型，必填"},
				"assigned_to":  {Type: "string", Description: "指派给账号，可选；未填则默认当前用户绑定账号"},
				"pri":          {Type: "number", Description: "优先级 0-4，可选"},
				"estimate":     {Type: "number", Description: "预估工时，可选"},
				"est_started":  {Type: "string", Description: "预估开始日期 YYYY-MM-DD，可选，默认当天"},
				"deadline":     {Type: "string", Description: "截止日期 YYYY-MM-DD，必填（禅道实例通常要求）"},
				"story_id":     {Type: "number", Description: "关联需求 ID，可选"},
				"module_id":    {Type: "number", Description: "模块 ID，可选"},
				"desc":         {Type: "string", Description: "任务描述，可选"},
			},
			Required: []string{"execution_id", "type", "name", "deadline"},
		},
	}
}

func (t CreateTaskTool) Execute(ctx context.Context, caller CallerInfo, args map[string]interface{}) ToolCallResult {
	name := strings.TrimSpace(stringArg(args, "name"))
	executionID := int64(floatArg(args, "execution_id"))
	taskType := strings.TrimSpace(stringArg(args, "type"))
	deadline := strings.TrimSpace(stringArg(args, "deadline"))

	missing := make([]string, 0, 4)
	if name == "" {
		missing = append(missing, "name")
	}
	if executionID <= 0 {
		missing = append(missing, "execution_id")
	}
	if taskType == "" {
		missing = append(missing, "type")
	}
	if deadline == "" {
		missing = append(missing, "deadline")
	}
	if len(missing) > 0 {
		return ErrorResult("缺少必填字段: " + strings.Join(missing, ", "))
	}

	if _, err := time.Parse("2006-01-02", deadline); err != nil {
		return ErrorResult("deadline format must be YYYY-MM-DD")
	}

	projectID := int64(floatArg(args, "project_id"))

	payload := map[string]any{
		"name": name,
	}
	payload["execution"] = executionID
	if projectID > 0 {
		payload["project"] = projectID
	}

	payload["type"] = taskType

	assignedTo := strings.TrimSpace(stringArg(args, "assigned_to"))
	if assignedTo == "" {
		var err error
		assignedTo, err = resolveZentaoAccount(caller.Username)
		if err != nil {
			return ErrorResult("assigned_to 未指定，且无法解析当前用户绑定禅道账号: " + err.Error())
		}
	}
	payload["assignedTo"] = assignedTo
	if _, ok := args["pri"]; ok {
		pri := int(floatArg(args, "pri"))
		if pri < 0 || pri > 4 {
			return ErrorResult("pri must be between 0 and 4")
		}
		payload["pri"] = pri
	}
	if _, ok := args["estimate"]; ok {
		estimate := floatArg(args, "estimate")
		if estimate < 0 {
			return ErrorResult("estimate must be >= 0")
		}
		payload["estimate"] = estimate
	}
	estStarted := strings.TrimSpace(stringArg(args, "est_started"))
	if estStarted == "" {
		estStarted = strings.TrimSpace(stringArg(args, "estStarted"))
	}
	if estStarted != "" {
		if _, err := time.Parse("2006-01-02", estStarted); err != nil {
			return ErrorResult("est_started format must be YYYY-MM-DD")
		}
		payload["estStarted"] = estStarted
	} else {
		payload["estStarted"] = time.Now().Format("2006-01-02")
	}
	payload["deadline"] = deadline
	if storyID := int64(floatArg(args, "story_id")); storyID > 0 {
		payload["story"] = storyID
	}
	if moduleID := int64(floatArg(args, "module_id")); moduleID > 0 {
		payload["module"] = moduleID
	}
	if s := strings.TrimSpace(stringArg(args, "desc")); s != "" {
		payload["desc"] = s
	}

	resp, err := withZentaoClient(ctx, caller.Username, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APICreateTask(ctx, token, payload)
	})
	if err != nil {
		return ErrorResult(mcpWriteErr(err))
	}

	taskID := extractCreatedTaskID(resp)
	if taskID <= 0 {
		return ErrorResult("禅道返回成功但未给出 task_id，无法确认任务已创建。请到禅道任务列表核对后再重试。")
	}
	go etl.SyncTasks()

	targetID := ""
	if taskID > 0 {
		targetID = fmt.Sprintf("%d", taskID)
	}
	_ = db.WriteAudit(db.AuditInput{
		ActorUserID:   &caller.UserID,
		ActorUsername: caller.Username,
		Action:        "mcp_create_task",
		TargetType:    "task",
		TargetID:      targetID,
		Metadata: models.JSONB{
			"task_id": taskID,
			"payload": payload,
			"result":  resp,
		},
	})

	out, _ := json.Marshal(map[string]any{
		"ok":      true,
		"task_id": taskID,
		"result":  resp,
	})
	return TextResult(string(out))
}

func extractCreatedTaskID(resp map[string]any) int64 {
	if id, ok := int64FromAny(resp["id"]); ok && id > 0 {
		return id
	}
	if item, ok := resp["task"].(map[string]any); ok {
		if id, ok := int64FromAny(item["id"]); ok && id > 0 {
			return id
		}
	}
	if item, ok := resp["data"].(map[string]any); ok {
		if id, ok := int64FromAny(item["id"]); ok && id > 0 {
			return id
		}
	}
	return 0
}

func int64FromAny(v any) (int64, bool) {
	switch x := v.(type) {
	case int:
		return int64(x), true
	case int64:
		return x, true
	case float64:
		return int64(x), true
	case json.Number:
		n, err := x.Int64()
		return n, err == nil
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
		return n, err == nil
	default:
		return 0, false
	}
}

type UpdateEffortTool struct{}

func (t UpdateEffortTool) Definition() ToolDef {
	return ToolDef{
		Name:        "updateEffort",
		Description: "编辑已有报工记录（支持日期、工时、工作说明）。",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"effort_id": {Type: "number", Description: "报工记录 ID"},
				"work_date": {Type: "string", Description: "工作日期 YYYY-MM-DD，可选"},
				"consumed":  {Type: "number", Description: "本次消耗工时，可选"},
				"left":      {Type: "number", Description: "剩余工时，可选"},
				"work":      {Type: "string", Description: "工作说明，可选"},
			},
			Required: []string{"effort_id"},
		},
	}
}

func (t UpdateEffortTool) Execute(ctx context.Context, caller CallerInfo, args map[string]interface{}) ToolCallResult {
	effortID := int64(floatArg(args, "effort_id"))
	if effortID <= 0 {
		return ErrorResult("effort_id must be a positive integer")
	}
	payload := map[string]any{}
	if s := strings.TrimSpace(stringArg(args, "work")); s != "" {
		payload["work"] = s
	}
	if s := strings.TrimSpace(stringArg(args, "work_date")); s != "" {
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return ErrorResult("work_date format must be YYYY-MM-DD")
		}
		payload["date"] = s
	}
	if v, ok := args["consumed"]; ok {
		f := floatArg(map[string]interface{}{"x": v}, "x")
		if f < 0 {
			return ErrorResult("consumed must be >= 0")
		}
		payload["consumed"] = f
	}
	if v, ok := args["left"]; ok {
		f := floatArg(map[string]interface{}{"x": v}, "x")
		if f < 0 {
			return ErrorResult("left must be >= 0")
		}
		payload["left"] = f
	}
	if len(payload) == 0 {
		return ErrorResult("no fields to update")
	}

	taskID := lookupMCPEffortTaskID(effortID)
	resp, mode, err := mcpUpdateEffort(ctx, caller.Username, taskID, effortID, payload)
	if err != nil {
		return ErrorResult(mcpWriteErr(err))
	}
	if verifyErr := verifyEffortUpdatedPersisted(ctx, caller.Username, taskID, effortID, payload, resp); verifyErr != nil {
		return ErrorResult(verifyErr.Error())
	}
	go func() {
		etl.SyncEfforts()
		etl.SyncTasks()
	}()
	_ = db.WriteAudit(db.AuditInput{
		ActorUserID:   &caller.UserID,
		ActorUsername: caller.Username,
		Action:        "mcp_update_effort",
		TargetType:    "effort",
		TargetID:      fmt.Sprintf("%d", effortID),
		Metadata: models.JSONB{
			"mode":    mode,
			"payload": payload,
			"result":  resp,
		},
	})
	out, _ := json.Marshal(map[string]any{"ok": true, "effort_id": effortID, "mode": mode, "result": resp})
	return TextResult(string(out))
}

type DeleteEffortTool struct{}

func (t DeleteEffortTool) Definition() ToolDef {
	return ToolDef{
		Name:        "deleteEffort",
		Description: "删除一条报工记录。",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"effort_id": {Type: "number", Description: "报工记录 ID"},
			},
			Required: []string{"effort_id"},
		},
	}
}

func (t DeleteEffortTool) Execute(ctx context.Context, caller CallerInfo, args map[string]interface{}) ToolCallResult {
	effortID := int64(floatArg(args, "effort_id"))
	if effortID <= 0 {
		return ErrorResult("effort_id must be a positive integer")
	}
	taskID := lookupMCPEffortTaskID(effortID)
	resp, mode, err := mcpDeleteEffort(ctx, caller.Username, taskID, effortID)
	if err != nil {
		return ErrorResult(mcpWriteErr(err))
	}
	if verifyErr := verifyEffortDeletedPersisted(ctx, caller.Username, taskID, effortID); verifyErr != nil {
		return ErrorResult(verifyErr.Error())
	}
	go func() {
		etl.SyncEfforts()
		etl.SyncTasks()
	}()
	_ = db.WriteAudit(db.AuditInput{
		ActorUserID:   &caller.UserID,
		ActorUsername: caller.Username,
		Action:        "mcp_delete_effort",
		TargetType:    "effort",
		TargetID:      fmt.Sprintf("%d", effortID),
		Metadata: models.JSONB{
			"mode":   mode,
			"result": resp,
		},
	})
	out, _ := json.Marshal(map[string]any{"ok": true, "effort_id": effortID, "mode": mode, "result": resp})
	return TextResult(string(out))
}

type UpdateTaskTool struct{}

func (t UpdateTaskTool) Definition() ToolDef {
	objTrue := true
	return ToolDef{
		Name:        "updateTask",
		Description: "编辑任务字段（标题、类型、描述、指派人、优先级、工时、日期、状态、模块、需求等标准字段；可通过 custom_fields 透传禅道自定义字段）。失败时返回禅道接口的具体错误信息。",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"task_id":     {Type: "number", Description: "任务 ID，必填"},
				"name":        {Type: "string", Description: "任务标题，可选"},
				"type":        {Type: "string", Description: "任务类型（design|devel|request|test|study|discuss|ui|affair|misc 等），可选"},
				"desc":        {Type: "string", Description: "任务描述，可选"},
				"assigned_to": {Type: "string", Description: "新指派账号，可选"},
				"pri":         {Type: "number", Description: "优先级 0-4，可选"},
				"estimate":    {Type: "number", Description: "预估工时，可选"},
				"est_started": {Type: "string", Description: "预计开始日期 YYYY-MM-DD，可选"},
				"deadline":    {Type: "string", Description: "截止日期 YYYY-MM-DD，可选"},
				"status":      {Type: "string", Description: "wait|doing|done|closed|pause|cancel，可选"},
				"story_id":    {Type: "number", Description: "关联需求 ID，可选"},
				"module_id":   {Type: "number", Description: "模块 ID，可选"},
				"from_bug":    {Type: "number", Description: "来源 Bug ID，可选"},
				"mailto":      {Type: "string", Description: "抄送账号，多个可用逗号分隔，可选"},
				"color":       {Type: "string", Description: "任务颜色标记，可选"},
				"custom_fields": {
					Type:                 "object",
					Description:          "自定义字段键值对，按禅道字段编码原样透传（如 {\"field_xxx\":\"值\"}）；会合并进 PUT body",
					AdditionalProperties: &objTrue,
				},
			},
			Required: []string{"task_id"},
		},
	}
}

func (t UpdateTaskTool) Execute(ctx context.Context, caller CallerInfo, args map[string]interface{}) ToolCallResult {
	taskID := int64(floatArg(args, "task_id"))
	if taskID <= 0 {
		return ErrorResult("修改任务失败：task_id 必须为正整数")
	}
	payload, err := buildTaskUpdatePayload(args)
	if err != nil {
		return ErrorResult("修改任务失败：" + err.Error())
	}
	if len(payload) == 0 {
		return ErrorResult("修改任务失败：未提供任何可更新字段（name/type/desc/assigned_to/pri/estimate/est_started/deadline/status/story_id/module_id/from_bug/mailto/color/custom_fields）")
	}
	resp, err := withZentaoClient(ctx, caller.Username, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APIUpdateTask(ctx, token, taskID, payload)
	})
	if err != nil {
		return ErrorResult(fmt.Sprintf("修改任务失败（task_id=%d）：%s", taskID, mcpWriteErr(err)))
	}
	if verifyErr := verifyTaskUpdatePersisted(ctx, caller.Username, taskID, payload); verifyErr != nil {
		return ErrorResult(fmt.Sprintf("修改任务失败（task_id=%d）：%s", taskID, verifyErr.Error()))
	}
	go etl.SyncTasks()
	_ = db.WriteAudit(db.AuditInput{
		ActorUserID:   &caller.UserID,
		ActorUsername: caller.Username,
		Action:        "mcp_update_task",
		TargetType:    "task",
		TargetID:      fmt.Sprintf("%d", taskID),
		Metadata: models.JSONB{
			"payload": payload,
			"result":  resp,
		},
	})
	out, _ := json.Marshal(map[string]any{"ok": true, "task_id": taskID, "updated_fields": payloadKeys(payload), "result": resp})
	return TextResult(string(out))
}

// buildTaskUpdatePayload 将 MCP 参数转换为禅道 PUT /tasks/:id body。
// custom_fields（或 fields）中的键会按禅道字段编码原样合并；标准字段优先，避免被误覆盖。
func buildTaskUpdatePayload(args map[string]interface{}) (map[string]any, error) {
	payload := map[string]any{}

	if s := strings.TrimSpace(stringArg(args, "name")); s != "" {
		payload["name"] = s
	}
	if s := strings.TrimSpace(stringArg(args, "type")); s != "" {
		payload["type"] = s
	}
	if s := strings.TrimSpace(stringArg(args, "desc")); s != "" {
		payload["desc"] = s
	}
	if s := strings.TrimSpace(stringArg(args, "assigned_to")); s != "" {
		payload["assignedTo"] = s
	}
	if _, ok := args["pri"]; ok {
		pri := int(floatArg(args, "pri"))
		if pri < 0 || pri > 4 {
			return nil, fmt.Errorf("pri must be between 0 and 4")
		}
		payload["pri"] = pri
	}
	if _, ok := args["estimate"]; ok {
		estimate := floatArg(args, "estimate")
		if estimate < 0 {
			return nil, fmt.Errorf("estimate must be >= 0")
		}
		payload["estimate"] = estimate
	}
	estStarted := strings.TrimSpace(stringArg(args, "est_started"))
	if estStarted == "" {
		estStarted = strings.TrimSpace(stringArg(args, "estStarted"))
	}
	if estStarted != "" {
		if _, err := time.Parse("2006-01-02", estStarted); err != nil {
			return nil, fmt.Errorf("est_started format must be YYYY-MM-DD")
		}
		payload["estStarted"] = estStarted
	}
	if s := strings.TrimSpace(stringArg(args, "deadline")); s != "" {
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return nil, fmt.Errorf("deadline format must be YYYY-MM-DD")
		}
		payload["deadline"] = s
	}
	if s := strings.TrimSpace(stringArg(args, "status")); s != "" {
		switch s {
		case "wait", "doing", "done", "closed", "pause", "cancel":
			payload["status"] = s
		default:
			return nil, fmt.Errorf("status must be one of wait|doing|done|closed|pause|cancel")
		}
	}
	if storyID := int64(floatArg(args, "story_id")); storyID > 0 {
		payload["story"] = storyID
	}
	if moduleID := int64(floatArg(args, "module_id")); moduleID > 0 {
		payload["module"] = moduleID
	}
	fromBug := int64(floatArg(args, "from_bug"))
	if fromBug <= 0 {
		fromBug = int64(floatArg(args, "fromBug"))
	}
	if fromBug > 0 {
		payload["fromBug"] = fromBug
	}
	if s := strings.TrimSpace(stringArg(args, "mailto")); s != "" {
		payload["mailto"] = s
	}
	if s := strings.TrimSpace(stringArg(args, "color")); s != "" {
		payload["color"] = s
	}

	custom := firstMapArg(args, "custom_fields", "fields")
	if custom != nil {
		reserved := map[string]struct{}{
			"task_id": {}, "taskId": {}, "id": {},
			"custom_fields": {}, "fields": {},
		}
		for k, v := range custom {
			key := strings.TrimSpace(k)
			if key == "" {
				continue
			}
			if _, blocked := reserved[key]; blocked {
				continue
			}
			if _, exists := payload[key]; exists {
				continue // 标准字段优先
			}
			payload[key] = v
		}
	}
	return payload, nil
}

func firstMapArg(args map[string]interface{}, keys ...string) map[string]any {
	for _, key := range keys {
		v, ok := args[key]
		if !ok || v == nil {
			continue
		}
		if m, ok := v.(map[string]any); ok {
			return m
		}
		// 尝试再 marshal/unmarshal，兼容少见类型
		b, err := json.Marshal(v)
		if err != nil {
			continue
		}
		out := map[string]any{}
		if err := json.Unmarshal(b, &out); err == nil {
			return out
		}
	}
	return nil
}

func payloadKeys(payload map[string]any) []string {
	keys := make([]string, 0, len(payload))
	for k := range payload {
		keys = append(keys, k)
	}
	return keys
}

type CreateStoryTool struct{}

func (t CreateStoryTool) Definition() ToolDef {
	return ToolDef{
		Name:        "createStory",
		Description: "创建需求（必填：title、product_id、category、pri；其余字段可选）。",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"title":       {Type: "string", Description: "需求标题，必填"},
				"product_id":  {Type: "number", Description: "产品 ID，必填"},
				"category":    {Type: "number", Description: "类别（category）ID，必填"},
				"assigned_to": {Type: "string", Description: "指派给账号，可选"},
				"estimate":    {Type: "number", Description: "预估工时，可选"},
				"status":      {Type: "string", Description: "draft|reviewing|active|changing|changed|closed，可选"},
				"spec":        {Type: "string", Description: "需求描述，可选"},
				"pri":         {Type: "number", Description: "优先级 0-4，必填"},
				"plan_id":     {Type: "number", Description: "产品计划 ID，可选（可用 listPlans 查询）"},
			},
			Required: []string{"title", "product_id", "category", "pri"},
		},
	}
}

func (t CreateStoryTool) Execute(ctx context.Context, caller CallerInfo, args map[string]interface{}) ToolCallResult {
	title := strings.TrimSpace(stringArg(args, "title"))
	productID := int64(floatArg(args, "product_id"))
	categoryID := int64(floatArg(args, "category"))
	if categoryID <= 0 {
		categoryID = int64(floatArg(args, "category_id"))
	}
	_, hasPri := args["pri"]

	missing := make([]string, 0, 4)
	if title == "" {
		missing = append(missing, "title")
	}
	if productID <= 0 {
		missing = append(missing, "product_id")
	}
	if categoryID <= 0 {
		missing = append(missing, "category")
	}
	if !hasPri {
		missing = append(missing, "pri")
	}
	if len(missing) > 0 {
		return ErrorResult("缺少必填字段: " + strings.Join(missing, ", "))
	}

	payload := map[string]any{
		"title":    title,
		"product":  productID,
		"category": categoryID,
		"module":   categoryID, // 兼容部分实例把“类别”落在 module 字段
	}
	if s := strings.TrimSpace(stringArg(args, "assigned_to")); s != "" {
		payload["assignedTo"] = s
	}
	if _, ok := args["estimate"]; ok {
		estimate := floatArg(args, "estimate")
		if estimate < 0 {
			return ErrorResult("estimate must be >= 0")
		}
		payload["estimate"] = estimate
	}
	if s := strings.ToLower(strings.TrimSpace(stringArg(args, "status"))); s != "" {
		if !isValidStoryStatus(s) {
			return ErrorResult("status must be one of draft|reviewing|active|changing|changed|closed")
		}
		payload["status"] = s
	}
	if s := strings.TrimSpace(stringArg(args, "spec")); s != "" {
		payload["spec"] = s
	}
	pri := int(floatArg(args, "pri"))
	if pri < 0 || pri > 4 {
		return ErrorResult("pri must be between 0 and 4")
	}
	payload["pri"] = pri
	if planID := planIDArg(args); planID > 0 {
		payload["plan"] = planID
	}

	resp, err := withZentaoClient(ctx, caller.Username, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APICreateStory(ctx, token, payload)
	})
	if err != nil {
		return ErrorResult(mcpWriteErr(err))
	}

	storyID := extractCreatedStoryID(resp)
	if storyID <= 0 {
		return ErrorResult("禅道返回成功但未给出 story_id，无法确认需求已创建。请到禅道需求列表核对后再重试。")
	}
	if verifyErr := verifyStoryCreatedPersisted(ctx, caller.Username, storyID); verifyErr != nil {
		return ErrorResult(verifyErr.Error())
	}
	go etl.SyncStories()
	_ = db.WriteAudit(db.AuditInput{
		ActorUserID:   &caller.UserID,
		ActorUsername: caller.Username,
		Action:        "mcp_create_story",
		TargetType:    "story",
		TargetID:      fmt.Sprintf("%d", storyID),
		Metadata: models.JSONB{
			"story_id": storyID,
			"payload":  payload,
			"result":   resp,
		},
	})
	out, _ := json.Marshal(map[string]any{
		"ok":       true,
		"story_id": storyID,
		"result":   resp,
	})
	return TextResult(string(out))
}

type UpdateStoryTool struct{}

func (t UpdateStoryTool) Definition() ToolDef {
	return ToolDef{
		Name:        "updateStory",
		Description: "编辑需求字段（标题、状态、预估、指派人、产品等）。",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"story_id":    {Type: "number", Description: "需求 ID"},
				"title":       {Type: "string", Description: "需求标题，可选"},
				"product_id":  {Type: "number", Description: "产品 ID，可选"},
				"category":    {Type: "number", Description: "类别（category）ID，可选"},
				"assigned_to": {Type: "string", Description: "指派给账号，可选"},
				"estimate":    {Type: "number", Description: "预估工时，可选"},
				"status":      {Type: "string", Description: "draft|reviewing|active|changing|changed|closed，可选"},
				"spec":        {Type: "string", Description: "需求描述，可选"},
				"pri":         {Type: "number", Description: "优先级 0-4，可选"},
				"plan_id":     {Type: "number", Description: "产品计划 ID，可选（可用 listPlans 查询）"},
			},
			Required: []string{"story_id"},
		},
	}
}

func (t UpdateStoryTool) Execute(ctx context.Context, caller CallerInfo, args map[string]interface{}) ToolCallResult {
	storyID := int64(floatArg(args, "story_id"))
	if storyID <= 0 {
		return ErrorResult("story_id must be a positive integer")
	}
	payload := map[string]any{}
	if s := strings.TrimSpace(stringArg(args, "title")); s != "" {
		payload["title"] = s
	}
	if productID := int64(floatArg(args, "product_id")); productID > 0 {
		payload["product"] = productID
	}
	categoryID := int64(floatArg(args, "category"))
	if categoryID <= 0 {
		categoryID = int64(floatArg(args, "category_id"))
	}
	if categoryID > 0 {
		payload["category"] = categoryID
		payload["module"] = categoryID
	}
	if s := strings.TrimSpace(stringArg(args, "assigned_to")); s != "" {
		payload["assignedTo"] = s
	}
	if _, ok := args["estimate"]; ok {
		estimate := floatArg(args, "estimate")
		if estimate < 0 {
			return ErrorResult("estimate must be >= 0")
		}
		payload["estimate"] = estimate
	}
	if s := strings.ToLower(strings.TrimSpace(stringArg(args, "status"))); s != "" {
		if !isValidStoryStatus(s) {
			return ErrorResult("status must be one of draft|reviewing|active|changing|changed|closed")
		}
		payload["status"] = s
	}
	if s := strings.TrimSpace(stringArg(args, "spec")); s != "" {
		payload["spec"] = s
	}
	if _, ok := args["pri"]; ok {
		pri := int(floatArg(args, "pri"))
		if pri < 0 || pri > 4 {
			return ErrorResult("pri must be between 0 and 4")
		}
		payload["pri"] = pri
	}
	if planID := planIDArg(args); planID > 0 {
		payload["plan"] = planID
	}
	if len(payload) == 0 {
		return ErrorResult("no fields to update")
	}

	resp, err := withZentaoClient(ctx, caller.Username, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APIUpdateStory(ctx, token, storyID, payload)
	})
	if err != nil {
		return ErrorResult(mcpWriteErr(err))
	}
	if verifyErr := verifyStoryUpdatedPersisted(ctx, caller.Username, storyID, payload); verifyErr != nil {
		return ErrorResult(verifyErr.Error())
	}
	go etl.SyncStories()
	_ = db.WriteAudit(db.AuditInput{
		ActorUserID:   &caller.UserID,
		ActorUsername: caller.Username,
		Action:        "mcp_update_story",
		TargetType:    "story",
		TargetID:      fmt.Sprintf("%d", storyID),
		Metadata: models.JSONB{
			"payload": payload,
			"result":  resp,
		},
	})
	out, _ := json.Marshal(map[string]any{"ok": true, "story_id": storyID, "result": resp})
	return TextResult(string(out))
}

type DeleteStoryTool struct{}

func (t DeleteStoryTool) Definition() ToolDef {
	return ToolDef{
		Name:        "deleteStory",
		Description: "删除需求（若禅道实例无删除路由，则回退为关闭需求）。",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"story_id": {Type: "number", Description: "需求 ID"},
			},
			Required: []string{"story_id"},
		},
	}
}

func (t DeleteStoryTool) Execute(ctx context.Context, caller CallerInfo, args map[string]interface{}) ToolCallResult {
	storyID := int64(floatArg(args, "story_id"))
	if storyID <= 0 {
		return ErrorResult("story_id must be a positive integer")
	}
	resp, mode, err := mcpDeleteStory(ctx, caller.Username, storyID)
	if err != nil {
		return ErrorResult(mcpWriteErr(err))
	}
	if verifyErr := verifyStoryDeletedPersisted(ctx, caller.Username, storyID, mode); verifyErr != nil {
		return ErrorResult(verifyErr.Error())
	}
	go etl.SyncStories()
	_ = db.WriteAudit(db.AuditInput{
		ActorUserID:   &caller.UserID,
		ActorUsername: caller.Username,
		Action:        "mcp_delete_story",
		TargetType:    "story",
		TargetID:      fmt.Sprintf("%d", storyID),
		Metadata: models.JSONB{
			"mode":   mode,
			"result": resp,
		},
	})
	out, _ := json.Marshal(map[string]any{"ok": true, "story_id": storyID, "mode": mode, "result": resp})
	return TextResult(string(out))
}

type CreateBugTool struct{}

func (t CreateBugTool) Definition() ToolDef {
	return ToolDef{
		Name:        "createBug",
		Description: "创建缺陷（必填：title；其余字段可选）。",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"title":        {Type: "string", Description: "缺陷标题，必填"},
				"steps":        {Type: "string", Description: "复现步骤/描述，可选"},
				"execution_id": {Type: "number", Description: "所属迭代 ID，可选"},
				"project_id":   {Type: "number", Description: "项目 ID，可选"},
				"product_id":   {Type: "number", Description: "产品 ID，可选"},
				"assigned_to":  {Type: "string", Description: "指派给账号，可选"},
				"severity":     {Type: "number", Description: "严重级别 1-4，可选"},
				"story_id":     {Type: "number", Description: "关联需求 ID，可选"},
				"task_id":      {Type: "number", Description: "关联任务 ID，可选"},
				"type":         {Type: "string", Description: "缺陷类型，可选"},
				"plan_id":      {Type: "number", Description: "产品计划 ID，可选（可用 listPlans 查询）"},
			},
			Required: []string{"title"},
		},
	}
}

func (t CreateBugTool) Execute(ctx context.Context, caller CallerInfo, args map[string]interface{}) ToolCallResult {
	title := strings.TrimSpace(stringArg(args, "title"))
	if title == "" {
		return ErrorResult("title is required")
	}
	payload := map[string]any{"title": title}
	if s := strings.TrimSpace(stringArg(args, "steps")); s != "" {
		payload["steps"] = s
	}
	if executionID := int64(floatArg(args, "execution_id")); executionID > 0 {
		payload["execution"] = executionID
	}
	if projectID := int64(floatArg(args, "project_id")); projectID > 0 {
		payload["project"] = projectID
	}
	if productID := int64(floatArg(args, "product_id")); productID > 0 {
		payload["product"] = productID
	}
	if s := strings.TrimSpace(stringArg(args, "assigned_to")); s != "" {
		payload["assignedTo"] = s
	}
	if _, ok := args["severity"]; ok {
		severity := int(floatArg(args, "severity"))
		if severity < 1 || severity > 4 {
			return ErrorResult("severity must be between 1 and 4")
		}
		payload["severity"] = severity
	}
	if storyID := int64(floatArg(args, "story_id")); storyID > 0 {
		payload["story"] = storyID
	}
	if taskID := int64(floatArg(args, "task_id")); taskID > 0 {
		payload["task"] = taskID
	}
	if s := strings.TrimSpace(stringArg(args, "type")); s != "" {
		payload["type"] = s
	}
	if planID := planIDArg(args); planID > 0 {
		payload["plan"] = planID
	}

	resp, err := withZentaoClient(ctx, caller.Username, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APICreateBug(ctx, token, payload)
	})
	if err != nil {
		return ErrorResult(mcpWriteErr(err))
	}

	bugID := extractCreatedBugID(resp)
	if bugID <= 0 {
		return ErrorResult("禅道返回成功但未给出 bug_id，无法确认缺陷已创建。请到禅道缺陷列表核对后再重试。")
	}
	if verifyErr := verifyBugCreatedPersisted(ctx, caller.Username, bugID); verifyErr != nil {
		return ErrorResult(verifyErr.Error())
	}
	go etl.SyncBugs()
	_ = db.WriteAudit(db.AuditInput{
		ActorUserID:   &caller.UserID,
		ActorUsername: caller.Username,
		Action:        "mcp_create_bug",
		TargetType:    "bug",
		TargetID:      fmt.Sprintf("%d", bugID),
		Metadata: models.JSONB{
			"bug_id":  bugID,
			"payload": payload,
			"result":  resp,
		},
	})
	out, _ := json.Marshal(map[string]any{
		"ok":     true,
		"bug_id": bugID,
		"result": resp,
	})
	return TextResult(string(out))
}

type UpdateBugTool struct{}

func (t UpdateBugTool) Definition() ToolDef {
	return ToolDef{
		Name:        "updateBug",
		Description: "编辑缺陷字段（标题、状态、严重级别、指派人、关联项等）。",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"bug_id":       {Type: "number", Description: "缺陷 ID"},
				"title":        {Type: "string", Description: "缺陷标题，可选"},
				"steps":        {Type: "string", Description: "复现步骤/描述，可选"},
				"execution_id": {Type: "number", Description: "迭代 ID，可选"},
				"assigned_to":  {Type: "string", Description: "指派给账号，可选"},
				"severity":     {Type: "number", Description: "严重级别 1-4，可选"},
				"status":       {Type: "string", Description: "active|resolved|closed|wait|activating，可选"},
				"resolution":   {Type: "string", Description: "解决方案编码，可选"},
				"story_id":     {Type: "number", Description: "关联需求 ID，可选"},
				"task_id":      {Type: "number", Description: "关联任务 ID，可选"},
				"plan_id":      {Type: "number", Description: "产品计划 ID，可选（可用 listPlans 查询）"},
			},
			Required: []string{"bug_id"},
		},
	}
}

func (t UpdateBugTool) Execute(ctx context.Context, caller CallerInfo, args map[string]interface{}) ToolCallResult {
	bugID := int64(floatArg(args, "bug_id"))
	if bugID <= 0 {
		return ErrorResult("bug_id must be a positive integer")
	}
	payload := map[string]any{}
	if s := strings.TrimSpace(stringArg(args, "title")); s != "" {
		payload["title"] = s
	}
	if s := strings.TrimSpace(stringArg(args, "steps")); s != "" {
		payload["steps"] = s
	}
	if executionID := int64(floatArg(args, "execution_id")); executionID > 0 {
		payload["execution"] = executionID
	}
	if s := strings.TrimSpace(stringArg(args, "assigned_to")); s != "" {
		payload["assignedTo"] = s
	}
	if _, ok := args["severity"]; ok {
		severity := int(floatArg(args, "severity"))
		if severity < 1 || severity > 4 {
			return ErrorResult("severity must be between 1 and 4")
		}
		payload["severity"] = severity
	}
	if s := strings.ToLower(strings.TrimSpace(stringArg(args, "status"))); s != "" {
		switch s {
		case "active", "resolved", "closed", "wait", "activating":
			payload["status"] = s
		default:
			return ErrorResult("status must be one of active|resolved|closed|wait|activating")
		}
	}
	if s := strings.ToLower(strings.TrimSpace(stringArg(args, "resolution"))); s != "" {
		payload["resolution"] = s
	}
	if storyID := int64(floatArg(args, "story_id")); storyID > 0 {
		payload["story"] = storyID
	}
	if taskID := int64(floatArg(args, "task_id")); taskID > 0 {
		payload["task"] = taskID
	}
	if planID := planIDArg(args); planID > 0 {
		payload["plan"] = planID
	}
	if len(payload) == 0 {
		return ErrorResult("no fields to update")
	}

	resp, err := withZentaoClient(ctx, caller.Username, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APIUpdateBug(ctx, token, bugID, payload)
	})
	if err != nil {
		return ErrorResult(mcpWriteErr(err))
	}
	if verifyErr := verifyBugUpdatedPersisted(ctx, caller.Username, bugID, payload); verifyErr != nil {
		return ErrorResult(verifyErr.Error())
	}
	go etl.SyncBugs()
	_ = db.WriteAudit(db.AuditInput{
		ActorUserID:   &caller.UserID,
		ActorUsername: caller.Username,
		Action:        "mcp_update_bug",
		TargetType:    "bug",
		TargetID:      fmt.Sprintf("%d", bugID),
		Metadata: models.JSONB{
			"payload": payload,
			"result":  resp,
		},
	})
	out, _ := json.Marshal(map[string]any{"ok": true, "bug_id": bugID, "result": resp})
	return TextResult(string(out))
}

type DeleteBugTool struct{}

func (t DeleteBugTool) Definition() ToolDef {
	return ToolDef{
		Name:        "deleteBug",
		Description: "删除缺陷（若禅道实例无删除路由，则回退为关闭缺陷）。",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"bug_id":     {Type: "number", Description: "缺陷 ID"},
				"resolution": {Type: "string", Description: "回退关闭时使用的解决方案编码，默认 bydesign"},
			},
			Required: []string{"bug_id"},
		},
	}
}

func (t DeleteBugTool) Execute(ctx context.Context, caller CallerInfo, args map[string]interface{}) ToolCallResult {
	bugID := int64(floatArg(args, "bug_id"))
	if bugID <= 0 {
		return ErrorResult("bug_id must be a positive integer")
	}
	resolution := strings.ToLower(strings.TrimSpace(stringArg(args, "resolution")))
	if resolution == "" {
		resolution = "bydesign"
	}
	resp, mode, err := mcpDeleteBug(ctx, caller.Username, bugID, resolution)
	if err != nil {
		return ErrorResult(mcpWriteErr(err))
	}
	if verifyErr := verifyBugDeletedPersisted(ctx, caller.Username, bugID, mode); verifyErr != nil {
		return ErrorResult(verifyErr.Error())
	}
	go etl.SyncBugs()
	_ = db.WriteAudit(db.AuditInput{
		ActorUserID:   &caller.UserID,
		ActorUsername: caller.Username,
		Action:        "mcp_delete_bug",
		TargetType:    "bug",
		TargetID:      fmt.Sprintf("%d", bugID),
		Metadata: models.JSONB{
			"mode":   mode,
			"result": resp,
		},
	})
	out, _ := json.Marshal(map[string]any{"ok": true, "bug_id": bugID, "mode": mode, "result": resp})
	return TextResult(string(out))
}

func isValidStoryStatus(status string) bool {
	switch status {
	case "draft", "reviewing", "active", "changing", "changed", "closed":
		return true
	default:
		return false
	}
}

func extractCreatedStoryID(resp map[string]any) int64 {
	if id, ok := int64FromAny(resp["id"]); ok && id > 0 {
		return id
	}
	if item, ok := resp["story"].(map[string]any); ok {
		if id, ok := int64FromAny(item["id"]); ok && id > 0 {
			return id
		}
	}
	if item, ok := resp["data"].(map[string]any); ok {
		if id, ok := int64FromAny(item["id"]); ok && id > 0 {
			return id
		}
	}
	return 0
}

func verifyStoryCreatedPersisted(ctx context.Context, sub string, storyID int64) error {
	_, err := withZentaoClient(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APIGetStory(ctx, token, storyID)
	})
	if err != nil {
		return fmt.Errorf("需求创建后回读失败：%s", mcpWriteErr(err))
	}
	return nil
}

func verifyStoryUpdatedPersisted(ctx context.Context, sub string, storyID int64, payload map[string]any) error {
	storyResp, err := withZentaoClient(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APIGetStory(ctx, token, storyID)
	})
	if err != nil {
		return fmt.Errorf("需求更新后回读失败：%s", mcpWriteErr(err))
	}
	if diffs := diffStoryPayload(payload, storyResp); len(diffs) > 0 {
		return fmt.Errorf("需求已请求更新，但禅道未生效：%s", strings.Join(diffs, "；"))
	}
	return nil
}

func diffStoryPayload(payload map[string]any, storyResp map[string]any) []string {
	diffs := make([]string, 0, 6)
	if v, ok := payload["title"]; ok {
		want := strings.TrimSpace(fmt.Sprintf("%v", v))
		got := strings.TrimSpace(zentao.ExtractStoryTitle(storyResp))
		if want != got {
			diffs = append(diffs, fmt.Sprintf("title=%q (期望 %q)", got, want))
		}
	}
	if v, ok := payload["assignedTo"]; ok {
		want := strings.TrimSpace(fmt.Sprintf("%v", v))
		got := strings.TrimSpace(zentao.ExtractStoryAssignedTo(storyResp))
		if !strings.EqualFold(want, got) {
			if got == "" {
				got = "(empty)"
			}
			diffs = append(diffs, fmt.Sprintf("assignedTo=%q (期望 %q)", got, want))
		}
	}
	if v, ok := payload["status"]; ok {
		want := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", v)))
		got := zentao.ExtractStoryStatus(storyResp)
		if want != got {
			if got == "" {
				got = "(empty)"
			}
			diffs = append(diffs, fmt.Sprintf("status=%q (期望 %q)", got, want))
		}
	}
	if v, ok := payload["product"]; ok {
		want, okWant := int64FromAny(v)
		got, okGot := zentao.ExtractStoryProductID(storyResp)
		if !okWant || !okGot || got != want {
			diffs = append(diffs, fmt.Sprintf("product=%v (期望 %v)", got, want))
		}
	}
	if v, ok := payload["estimate"]; ok {
		want, okWant := float64FromAny(v)
		got, okGot := zentao.ExtractStoryEstimate(storyResp)
		if !okWant || !okGot || !floatAlmostEqual(got, want) {
			diffs = append(diffs, fmt.Sprintf("estimate=%g (期望 %g)", got, want))
		}
	}
	if v, ok := payload["plan"]; ok {
		want, okWant := int64FromAny(v)
		got := zentao.ExtractStoryPlanIDs(storyResp)
		if !okWant || !zentao.ContainsPlanID(got, want) {
			diffs = append(diffs, fmt.Sprintf("plan=%v (期望 %v)", got, want))
		}
	}
	return diffs
}

func extractCreatedBugID(resp map[string]any) int64 {
	if id, ok := int64FromAny(resp["id"]); ok && id > 0 {
		return id
	}
	if item, ok := resp["bug"].(map[string]any); ok {
		if id, ok := int64FromAny(item["id"]); ok && id > 0 {
			return id
		}
	}
	if item, ok := resp["data"].(map[string]any); ok {
		if id, ok := int64FromAny(item["id"]); ok && id > 0 {
			return id
		}
	}
	return 0
}

func verifyBugCreatedPersisted(ctx context.Context, sub string, bugID int64) error {
	_, err := withZentaoClient(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APIGetBug(ctx, token, bugID)
	})
	if err != nil {
		return fmt.Errorf("缺陷创建后回读失败：%s", mcpWriteErr(err))
	}
	return nil
}

func verifyBugUpdatedPersisted(ctx context.Context, sub string, bugID int64, payload map[string]any) error {
	bugResp, err := withZentaoClient(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APIGetBug(ctx, token, bugID)
	})
	if err != nil {
		return fmt.Errorf("缺陷更新后回读失败：%s", mcpWriteErr(err))
	}
	if diffs := diffBugPayload(payload, bugResp); len(diffs) > 0 {
		return fmt.Errorf("缺陷已请求更新，但禅道未生效：%s", strings.Join(diffs, "；"))
	}
	return nil
}

func diffBugPayload(payload map[string]any, bugResp map[string]any) []string {
	diffs := make([]string, 0, 8)
	if v, ok := payload["title"]; ok {
		want := strings.TrimSpace(fmt.Sprintf("%v", v))
		got := strings.TrimSpace(zentao.ExtractBugTitle(bugResp))
		if want != got {
			diffs = append(diffs, fmt.Sprintf("title=%q (期望 %q)", got, want))
		}
	}
	if v, ok := payload["assignedTo"]; ok {
		want := strings.TrimSpace(fmt.Sprintf("%v", v))
		got := strings.TrimSpace(zentao.ExtractBugAssignedTo(bugResp))
		if !strings.EqualFold(want, got) {
			if got == "" {
				got = "(empty)"
			}
			diffs = append(diffs, fmt.Sprintf("assignedTo=%q (期望 %q)", got, want))
		}
	}
	if v, ok := payload["severity"]; ok {
		want, okWant := int64FromAny(v)
		got, okGot := zentao.ExtractBugSeverity(bugResp)
		if !okWant || !okGot || int64(got) != want {
			diffs = append(diffs, fmt.Sprintf("severity=%v (期望 %v)", got, want))
		}
	}
	if v, ok := payload["status"]; ok {
		want := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", v)))
		got := zentao.ExtractBugStatus(bugResp)
		if want != got {
			if got == "" {
				got = "(empty)"
			}
			diffs = append(diffs, fmt.Sprintf("status=%q (期望 %q)", got, want))
		}
	}
	if v, ok := payload["resolution"]; ok {
		want := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", v)))
		got := zentao.ExtractBugResolution(bugResp)
		if want != got {
			if got == "" {
				got = "(empty)"
			}
			diffs = append(diffs, fmt.Sprintf("resolution=%q (期望 %q)", got, want))
		}
	}
	if v, ok := payload["execution"]; ok {
		want, okWant := int64FromAny(v)
		got, okGot := zentao.ExtractBugExecutionID(bugResp)
		if !okWant || !okGot || got != want {
			diffs = append(diffs, fmt.Sprintf("execution=%v (期望 %v)", got, want))
		}
	}
	if v, ok := payload["story"]; ok {
		want, okWant := int64FromAny(v)
		got, okGot := zentao.ExtractBugStoryID(bugResp)
		if !okWant || !okGot || got != want {
			diffs = append(diffs, fmt.Sprintf("story=%v (期望 %v)", got, want))
		}
	}
	if v, ok := payload["task"]; ok {
		want, okWant := int64FromAny(v)
		got, okGot := zentao.ExtractBugTaskID(bugResp)
		if !okWant || !okGot || got != want {
			diffs = append(diffs, fmt.Sprintf("task=%v (期望 %v)", got, want))
		}
	}
	if v, ok := payload["plan"]; ok {
		want, okWant := int64FromAny(v)
		got := zentao.ExtractBugPlanIDs(bugResp)
		if !okWant || !zentao.ContainsPlanID(got, want) {
			diffs = append(diffs, fmt.Sprintf("plan=%v (期望 %v)", got, want))
		}
	}
	return diffs
}

func verifyTaskUpdatePersisted(ctx context.Context, sub string, taskID int64, payload map[string]any) error {
	taskResp, err := withZentaoClient(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APIGetTask(ctx, token, taskID)
	})
	if err != nil {
		return fmt.Errorf("任务更新后回读失败：%s", mcpWriteErr(err))
	}
	if diffs := diffTaskPayload(payload, taskResp); len(diffs) > 0 {
		return fmt.Errorf("任务已请求更新，但禅道未生效：%s。请确认字段值、任务状态流转限制、自定义字段编码与当前账号权限", strings.Join(diffs, "；"))
	}
	return nil
}

func diffTaskPayload(payload map[string]any, taskResp map[string]any) []string {
	diffs := make([]string, 0, 8)
	knownKeys := map[string]struct{}{
		"assignedTo": {}, "pri": {}, "deadline": {}, "status": {},
		"name": {}, "type": {}, "desc": {}, "estimate": {}, "estStarted": {},
		"story": {}, "module": {}, "fromBug": {}, "mailto": {}, "color": {},
	}

	if v, ok := payload["assignedTo"]; ok {
		want := strings.TrimSpace(fmt.Sprintf("%v", v))
		got := strings.TrimSpace(zentao.ExtractTaskAssignedTo(taskResp))
		if !strings.EqualFold(want, got) {
			if got == "" {
				got = "(empty)"
			}
			diffs = append(diffs, fmt.Sprintf("assignedTo=%q (期望 %q)", got, want))
		}
	}
	if v, ok := payload["pri"]; ok {
		want, okWant := int64FromAny(v)
		got, okGot := zentao.ExtractTaskPri(taskResp)
		if !okWant || !okGot || int64(got) != want {
			diffs = append(diffs, fmt.Sprintf("pri=%v (期望 %v)", got, want))
		}
	}
	if v, ok := payload["deadline"]; ok {
		want := zentao.NormalizeDateYMD(fmt.Sprintf("%v", v))
		got := zentao.ExtractTaskDeadline(taskResp)
		if want != got {
			if got == "" {
				got = "(empty)"
			}
			diffs = append(diffs, fmt.Sprintf("deadline=%q (期望 %q)", got, want))
		}
	}
	if v, ok := payload["status"]; ok {
		want := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", v)))
		got := zentao.ExtractTaskStatus(taskResp)
		if want != got {
			if got == "" {
				got = "(empty)"
			}
			diffs = append(diffs, fmt.Sprintf("status=%q (期望 %q)", got, want))
		}
	}
	if v, ok := payload["name"]; ok {
		want := strings.TrimSpace(fmt.Sprintf("%v", v))
		got := strings.TrimSpace(zentao.ExtractTaskString(taskResp, "name", "title"))
		if want != got {
			diffs = append(diffs, fmt.Sprintf("name=%q (期望 %q)", emptyAsPlaceholder(got), want))
		}
	}
	if v, ok := payload["type"]; ok {
		want := strings.TrimSpace(fmt.Sprintf("%v", v))
		got := strings.TrimSpace(zentao.ExtractTaskString(taskResp, "type"))
		if !strings.EqualFold(want, got) {
			diffs = append(diffs, fmt.Sprintf("type=%q (期望 %q)", emptyAsPlaceholder(got), want))
		}
	}
	if v, ok := payload["desc"]; ok {
		want := strings.TrimSpace(stripHTMLRough(fmt.Sprintf("%v", v)))
		got := strings.TrimSpace(stripHTMLRough(zentao.ExtractTaskString(taskResp, "desc", "description")))
		if want != "" && want != got {
			diffs = append(diffs, fmt.Sprintf("desc 未按期望更新（回读长度=%d，期望长度=%d）", len(got), len(want)))
		}
	}
	if v, ok := payload["estimate"]; ok {
		want, okWant := floatFromAny(v)
		got, okGot := zentao.ExtractTaskFloat(taskResp, "estimate")
		if !okWant || !okGot || !floatAlmostEqual(want, got) {
			diffs = append(diffs, fmt.Sprintf("estimate=%v (期望 %v)", got, want))
		}
	}
	if v, ok := payload["estStarted"]; ok {
		want := zentao.NormalizeDateYMD(fmt.Sprintf("%v", v))
		got := zentao.NormalizeDateYMD(zentao.ExtractTaskString(taskResp, "estStarted", "est_started"))
		if want != got {
			diffs = append(diffs, fmt.Sprintf("estStarted=%q (期望 %q)", emptyAsPlaceholder(got), want))
		}
	}
	if v, ok := payload["story"]; ok {
		want, okWant := int64FromAny(v)
		got, okGot := zentao.ExtractTaskInt64(taskResp, "story", "storyID", "storyId")
		if !okWant || !okGot || want != got {
			diffs = append(diffs, fmt.Sprintf("story=%v (期望 %v)", got, want))
		}
	}
	if v, ok := payload["module"]; ok {
		want, okWant := int64FromAny(v)
		got, okGot := zentao.ExtractTaskInt64(taskResp, "module", "moduleID", "moduleId")
		if !okWant || !okGot || want != got {
			diffs = append(diffs, fmt.Sprintf("module=%v (期望 %v)", got, want))
		}
	}
	if v, ok := payload["fromBug"]; ok {
		want, okWant := int64FromAny(v)
		got, okGot := zentao.ExtractTaskInt64(taskResp, "fromBug", "from_bug")
		if !okWant || !okGot || want != got {
			diffs = append(diffs, fmt.Sprintf("fromBug=%v (期望 %v)", got, want))
		}
	}
	if v, ok := payload["mailto"]; ok {
		want := strings.TrimSpace(fmt.Sprintf("%v", v))
		got := strings.TrimSpace(zentao.ExtractTaskString(taskResp, "mailto"))
		if !strings.EqualFold(want, got) {
			diffs = append(diffs, fmt.Sprintf("mailto=%q (期望 %q)", emptyAsPlaceholder(got), want))
		}
	}
	if v, ok := payload["color"]; ok {
		want := strings.TrimSpace(fmt.Sprintf("%v", v))
		got := strings.TrimSpace(zentao.ExtractTaskString(taskResp, "color"))
		if want != got {
			diffs = append(diffs, fmt.Sprintf("color=%q (期望 %q)", emptyAsPlaceholder(got), want))
		}
	}

	// 自定义字段：尽力按同名键回读比对；禅道 GET 未回传该键时跳过（避免误报）
	for key, wantVal := range payload {
		if _, isKnown := knownKeys[key]; isKnown {
			continue
		}
		gotRaw, ok := zentao.ExtractTaskRaw(taskResp, key)
		if !ok {
			continue
		}
		want := strings.TrimSpace(fmt.Sprintf("%v", wantVal))
		got := strings.TrimSpace(fmt.Sprintf("%v", gotRaw))
		if want != got && !strings.EqualFold(want, got) {
			diffs = append(diffs, fmt.Sprintf("%s=%q (期望 %q)", key, emptyAsPlaceholder(got), want))
		}
	}
	return diffs
}

func emptyAsPlaceholder(s string) string {
	if s == "" {
		return "(empty)"
	}
	return s
}

func stripHTMLRough(s string) string {
	out := strings.Builder{}
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			out.WriteRune(r)
		}
	}
	return strings.TrimSpace(out.String())
}

func floatFromAny(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func verifyEffortUpdatedPersisted(ctx context.Context, sub string, taskID, effortID int64, payload, updateResp map[string]any) error {
	verifyTaskID := taskID
	if verifyTaskID <= 0 {
		if id, ok := zentao.ExtractEffortTaskID(updateResp); ok && id > 0 {
			verifyTaskID = id
		}
	}
	if verifyTaskID > 0 {
		efforts, err := loadTaskEffortsForVerify(ctx, sub, verifyTaskID)
		if err != nil {
			return fmt.Errorf("报工更新后回读失败：%s", mcpWriteErr(err))
		}
		for _, e := range efforts {
			if e.ID != effortID {
				continue
			}
			if diffs := diffEffortPayload(payload, e); len(diffs) > 0 {
				return fmt.Errorf("报工已请求更新，但禅道未生效：%s", strings.Join(diffs, "；"))
			}
			return nil
		}
		return fmt.Errorf("报工更新后回读未找到 effort_id=%d", effortID)
	}

	effResp, err := withZentaoClient(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APIGetEffort(ctx, token, effortID)
	})
	if err != nil {
		return fmt.Errorf("报工更新后回读失败：%s", mcpWriteErr(err))
	}
	if diffs := diffEffortPayloadByMap(payload, effResp); len(diffs) > 0 {
		return fmt.Errorf("报工已请求更新，但禅道未生效：%s", strings.Join(diffs, "；"))
	}
	return nil
}

func verifyEffortDeletedPersisted(ctx context.Context, sub string, taskID, effortID int64) error {
	if taskID > 0 {
		efforts, err := loadTaskEffortsForVerify(ctx, sub, taskID)
		if err != nil {
			return fmt.Errorf("报工删除后回读失败：%s", mcpWriteErr(err))
		}
		for _, e := range efforts {
			if e.ID == effortID {
				return fmt.Errorf("报工删除未生效：effort_id=%d 仍存在", effortID)
			}
		}
		return nil
	}

	_, err := withZentaoClient(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APIGetEffort(ctx, token, effortID)
	})
	if err == nil {
		return fmt.Errorf("报工删除未生效：effort_id=%d 仍可查询", effortID)
	}
	if he, ok := zentao.IsAPIHTTPError(err); ok && he.Status == 404 {
		return nil
	}
	return fmt.Errorf("报工删除后回读失败：%s", mcpWriteErr(err))
}

func loadTaskEffortsForVerify(ctx context.Context, sub string, taskID int64) ([]zentao.APITaskEffort, error) {
	resp, err := withZentaoClient(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		items, err := cli.APIListTaskEfforts(ctx, token, taskID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"items": items}, nil
	})
	if err != nil {
		return nil, err
	}
	items, ok := resp["items"].([]zentao.APITaskEffort)
	if !ok {
		return nil, fmt.Errorf("unexpected effort list response type")
	}
	return items, nil
}

func diffEffortPayload(payload map[string]any, effort zentao.APITaskEffort) []string {
	diffs := make([]string, 0, 4)
	if v, ok := payload["work"]; ok {
		want := strings.TrimSpace(fmt.Sprintf("%v", v))
		got := strings.TrimSpace(effort.Work)
		if want != got {
			diffs = append(diffs, fmt.Sprintf("work=%q (期望 %q)", got, want))
		}
	}
	if v, ok := payload["date"]; ok {
		want := zentao.NormalizeDateYMD(fmt.Sprintf("%v", v))
		got := zentao.NormalizeDateYMD(effort.Date)
		if want != got {
			diffs = append(diffs, fmt.Sprintf("date=%q (期望 %q)", got, want))
		}
	}
	if v, ok := payload["consumed"]; ok {
		want, okWant := float64FromAny(v)
		if !okWant || !floatAlmostEqual(effort.Consumed, want) {
			diffs = append(diffs, fmt.Sprintf("consumed=%g (期望 %g)", effort.Consumed, want))
		}
	}
	if v, ok := payload["left"]; ok {
		want, okWant := float64FromAny(v)
		if !okWant || !floatAlmostEqual(effort.Left, want) {
			diffs = append(diffs, fmt.Sprintf("left=%g (期望 %g)", effort.Left, want))
		}
	}
	return diffs
}

func diffEffortPayloadByMap(payload map[string]any, effortResp map[string]any) []string {
	diffs := make([]string, 0, 4)
	if v, ok := payload["work"]; ok {
		want := strings.TrimSpace(fmt.Sprintf("%v", v))
		got := strings.TrimSpace(zentao.ExtractEffortWork(effortResp))
		if want != got {
			diffs = append(diffs, fmt.Sprintf("work=%q (期望 %q)", got, want))
		}
	}
	if v, ok := payload["date"]; ok {
		want := zentao.NormalizeDateYMD(fmt.Sprintf("%v", v))
		got := zentao.ExtractEffortDate(effortResp)
		if want != got {
			diffs = append(diffs, fmt.Sprintf("date=%q (期望 %q)", got, want))
		}
	}
	if v, ok := payload["consumed"]; ok {
		want, okWant := float64FromAny(v)
		got, okGot := zentao.ExtractEffortConsumed(effortResp)
		if !okWant || !okGot || !floatAlmostEqual(got, want) {
			diffs = append(diffs, fmt.Sprintf("consumed=%g (期望 %g)", got, want))
		}
	}
	if v, ok := payload["left"]; ok {
		want, okWant := float64FromAny(v)
		got, okGot := zentao.ExtractEffortLeft(effortResp)
		if !okWant || !okGot || !floatAlmostEqual(got, want) {
			diffs = append(diffs, fmt.Sprintf("left=%g (期望 %g)", got, want))
		}
	}
	return diffs
}

func float64FromAny(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func floatAlmostEqual(a, b float64) bool {
	const eps = 1e-3
	if a > b {
		return a-b <= eps
	}
	return b-a <= eps
}

func withZentaoClient(ctx context.Context, sub string, call func(context.Context, *zentao.APIClient, string) (map[string]any, error)) (map[string]any, error) {
	baseURL := strings.TrimSpace(config.Global.ZentaoBaseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("zentao base_url not configured")
	}
	token, err := zentaoauth.EnsureAPIToken(ctx, sub)
	if err != nil {
		return nil, err
	}
	cli := zentao.NewAPIClient(baseURL)
	resp, err := call(ctx, cli, token)
	if err == nil {
		return resp, nil
	}
	if zentao.IsAPIUnauthorizedError(err) {
		zentaoauth.DeleteAPIToken(ctx, sub)
		newToken, reloginErr := zentaoauth.LoginAndCacheAPIToken(ctx, sub)
		if reloginErr != nil {
			return nil, reloginErr
		}
		return call(ctx, cli, newToken)
	}
	return nil, err
}

func lookupMCPEffortTaskID(effortID int64) int64 {
	var row models.LocalEffort
	if err := db.PG.First(&row, effortID).Error; err != nil {
		return 0
	}
	if row.ObjectType != "" && row.ObjectType != "task" {
		return 0
	}
	return row.ObjectID
}

func mcpUpdateEffort(ctx context.Context, username string, taskID, effortID int64, payload map[string]any) (map[string]any, string, error) {
	resp, err := withZentaoClient(ctx, username, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APIUpdateEffort(ctx, token, taskID, effortID, payload)
	})
	if err == nil {
		return resp, "api_v1", nil
	}
	if !zentao.IsEffortAPIMissing(err) {
		return nil, "", err
	}
	return nil, "", fmt.Errorf("zentao api has no effort update route; bind zentao web session or use UI: %w", err)
}

func mcpDeleteEffort(ctx context.Context, username string, taskID, effortID int64) (map[string]any, string, error) {
	_, err := withZentaoClient(ctx, username, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return map[string]any{"ok": true}, cli.APIDeleteEffort(ctx, token, taskID, effortID)
	})
	if err == nil {
		return map[string]any{"ok": true}, "api_v1", nil
	}
	if !zentao.IsEffortAPIMissing(err) {
		return nil, "", err
	}
	return nil, "", fmt.Errorf("zentao api has no effort delete route; bind zentao web session or use UI: %w", err)
}

func mcpDeleteStory(ctx context.Context, username string, storyID int64) (map[string]any, string, error) {
	_, err := withZentaoClient(ctx, username, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return map[string]any{"ok": true}, cli.APIDeleteStory(ctx, token, storyID)
	})
	if err == nil {
		return map[string]any{"ok": true}, "api_v1_delete", nil
	}
	if !zentao.IsStoryAPIMissing(err) {
		return nil, "", err
	}
	resp, closeErr := withZentaoClient(ctx, username, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APIUpdateStory(ctx, token, storyID, map[string]any{"status": "closed"})
	})
	if closeErr != nil {
		return nil, "", fmt.Errorf("zentao api has no story delete route and close fallback failed: %w", closeErr)
	}
	return map[string]any{"ok": true, "close_result": resp}, "api_v1_close", nil
}

func mcpDeleteBug(ctx context.Context, username string, bugID int64, resolution string) (map[string]any, string, error) {
	_, err := withZentaoClient(ctx, username, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return map[string]any{"ok": true}, cli.APIDeleteBug(ctx, token, bugID)
	})
	if err == nil {
		return map[string]any{"ok": true}, "api_v1_delete", nil
	}
	if !zentao.IsBugAPIMissing(err) {
		return nil, "", err
	}
	if strings.TrimSpace(resolution) == "" {
		resolution = "bydesign"
	}
	payload := map[string]any{
		"status":     "closed",
		"resolution": resolution,
	}
	resp, closeErr := withZentaoClient(ctx, username, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APIUpdateBug(ctx, token, bugID, payload)
	})
	if closeErr != nil {
		return nil, "", fmt.Errorf("zentao api has no bug delete route and close fallback failed: %w", closeErr)
	}
	return map[string]any{"ok": true, "close_result": resp}, "api_v1_close", nil
}

func verifyStoryDeletedPersisted(ctx context.Context, sub string, storyID int64, mode string) error {
	storyResp, err := withZentaoClient(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APIGetStory(ctx, token, storyID)
	})
	if err != nil {
		if he, ok := zentao.IsAPIHTTPError(err); ok && he.Status == 404 {
			return nil
		}
		return fmt.Errorf("需求删除后回读失败：%s", mcpWriteErr(err))
	}
	if zentao.ExtractStoryDeleted(storyResp) {
		return nil
	}
	status := zentao.ExtractStoryStatus(storyResp)
	if mode == "api_v1_close" && status == "closed" {
		return nil
	}
	return fmt.Errorf("需求删除未生效：story_id=%d 仍存在（status=%s）", storyID, status)
}

func verifyBugDeletedPersisted(ctx context.Context, sub string, bugID int64, mode string) error {
	bugResp, err := withZentaoClient(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APIGetBug(ctx, token, bugID)
	})
	if err != nil {
		if he, ok := zentao.IsAPIHTTPError(err); ok && he.Status == 404 {
			return nil
		}
		return fmt.Errorf("缺陷删除后回读失败：%s", mcpWriteErr(err))
	}
	if zentao.ExtractBugDeleted(bugResp) {
		return nil
	}
	status := zentao.ExtractBugStatus(bugResp)
	if mode == "api_v1_close" && status == "closed" {
		return nil
	}
	return fmt.Errorf("缺陷删除未生效：bug_id=%d 仍存在（status=%s）", bugID, status)
}

func mcpWriteErr(err error) string {
	if err == nil {
		return ""
	}
	if he, ok := zentao.IsAPIHTTPError(err); ok {
		detail := formatZentaoAPIErrorBody(he.Body)
		if detail == "" {
			detail = "(无响应正文)"
		}
		return fmt.Sprintf("禅道接口错误 HTTP %d：%s", he.Status, detail)
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "login") || strings.Contains(lower, "token") || strings.Contains(lower, "unauthorized") {
		return "禅道 API 登录失败，请先在「禅道授权」页重新绑定：" + msg
	}
	return msg
}

func formatZentaoAPIErrorBody(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err == nil {
		for _, key := range []string{"error", "message", "msg"} {
			if s, ok := m[key].(string); ok {
				if t := strings.TrimSpace(s); t != "" {
					return t
				}
			}
		}
		if errs, ok := m["errors"].(map[string]any); ok && len(errs) > 0 {
			parts := make([]string, 0, len(errs))
			for k, v := range errs {
				parts = append(parts, fmt.Sprintf("%s=%v", k, v))
			}
			return "字段错误: " + strings.Join(parts, "; ")
		}
		if errs, ok := m["errors"].([]any); ok && len(errs) > 0 {
			parts := make([]string, 0, len(errs))
			for _, v := range errs {
				parts = append(parts, fmt.Sprintf("%v", v))
			}
			return "字段错误: " + strings.Join(parts, "; ")
		}
	}
	return body
}
