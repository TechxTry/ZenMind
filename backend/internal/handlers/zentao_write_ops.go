package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"zenmind/internal/config"
	"zenmind/internal/db"
	"zenmind/internal/etl"
	"zenmind/internal/models"
	"zenmind/internal/source"
	"zenmind/internal/zentao"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type updateZentaoEffortBody struct {
	WorkDate string        `json:"work_date"`
	Work     string        `json:"work"`
	Consumed hourFormValue `json:"consumed"`
	Left     hourFormValue `json:"left"`
}

type updateZentaoTaskBody struct {
	Name         string         `json:"name"`
	Type         string         `json:"type"`
	Desc         string         `json:"desc"`
	AssignedTo   string         `json:"assigned_to"`
	Pri          *int           `json:"pri"`
	Estimate     *float64       `json:"estimate"`
	EstStarted   string         `json:"est_started"`
	Deadline     string         `json:"deadline"`
	Status       string         `json:"status"`
	StoryID      *int64         `json:"story_id"`
	ModuleID     *int64         `json:"module_id"`
	FromBug      *int64         `json:"from_bug"`
	Mailto       string         `json:"mailto"`
	Color        string         `json:"color"`
	CustomFields map[string]any `json:"custom_fields"`
}

// UpdateZentaoEffort PATCH /api/zentao/efforts/:id
func UpdateZentaoEffort(c *gin.Context) {
	sub := currentSub(c)
	if sub == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing sub"})
		return
	}
	effortID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || effortID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid effort id"})
		return
	}

	var req updateZentaoEffortBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	payload := make(map[string]any)
	if work := strings.TrimSpace(req.Work); work != "" {
		payload["work"] = work
	}
	if date := strings.TrimSpace(req.WorkDate); date != "" {
		if _, err := time.Parse("2006-01-02", date); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "work_date format must be YYYY-MM-DD"})
			return
		}
		payload["date"] = date
	}
	if v := strings.TrimSpace(string(req.Consumed)); v != "" {
		f, err := req.Consumed.Float()
		if err != nil || f < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid consumed"})
			return
		}
		payload["consumed"] = f
	}
	if v := strings.TrimSpace(string(req.Left)); v != "" {
		f, err := req.Left.Float()
		if err != nil || f < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid left"})
			return
		}
		payload["left"] = f
	}
	if len(payload) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "empty update payload"})
		return
	}

	taskID := lookupEffortTaskID(effortID)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 25*time.Second)
	defer cancel()

	resp, mode, err := updateEffortToZentao(ctx, sub, taskID, effortID, payload, &req)
	if err != nil {
		writeEffortWriteErr(c, err)
		return
	}
	if verifyErr := verifyEffortUpdatedPersisted(ctx, sub, mode, taskID, effortID, payload, resp); verifyErr != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": verifyErr.Error()})
		return
	}
	go func() {
		etl.SyncEfforts()
		etl.SyncTasks()
	}()
	_ = db.WriteAudit(db.AuditInput{
		ActorUsername: sub,
		Action:        "zentao_update_effort",
		TargetType:    "effort",
		TargetID:      strconv.FormatInt(effortID, 10),
		Metadata: models.JSONB{
			"mode":    mode,
			"payload": payload,
			"result":  resp,
		},
		IP: c.ClientIP(),
		UA: c.GetHeader("User-Agent"),
	})
	c.JSON(http.StatusOK, gin.H{"ok": true, "mode": mode, "result": resp})
}

// DeleteZentaoEffort DELETE /api/zentao/efforts/:id
func DeleteZentaoEffort(c *gin.Context) {
	sub := currentSub(c)
	if sub == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing sub"})
		return
	}
	effortID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || effortID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid effort id"})
		return
	}
	taskID := lookupEffortTaskID(effortID)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 25*time.Second)
	defer cancel()

	resp, mode, err := deleteEffortFromZentao(ctx, sub, taskID, effortID)
	if err != nil {
		writeEffortWriteErr(c, err)
		return
	}
	if verifyErr := verifyEffortDeletedPersisted(ctx, sub, mode, taskID, effortID); verifyErr != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": verifyErr.Error()})
		return
	}
	go func() {
		etl.SyncEfforts()
		etl.SyncTasks()
	}()
	_ = db.WriteAudit(db.AuditInput{
		ActorUsername: sub,
		Action:        "zentao_delete_effort",
		TargetType:    "effort",
		TargetID:      strconv.FormatInt(effortID, 10),
		Metadata: models.JSONB{
			"mode":   mode,
			"result": resp,
		},
		IP: c.ClientIP(),
		UA: c.GetHeader("User-Agent"),
	})
	c.JSON(http.StatusOK, gin.H{"ok": true, "mode": mode, "result": resp})
}

// UpdateZentaoTask PATCH /api/zentao/tasks/:id
func UpdateZentaoTask(c *gin.Context) {
	sub := currentSub(c)
	if sub == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing sub"})
		return
	}
	taskID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || taskID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task id"})
		return
	}
	var req updateZentaoTaskBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	payload := make(map[string]any)
	if v := strings.TrimSpace(req.Name); v != "" {
		payload["name"] = v
	}
	if v := strings.TrimSpace(req.Type); v != "" {
		payload["type"] = v
	}
	if v := strings.TrimSpace(req.Desc); v != "" {
		payload["desc"] = v
	}
	if v := strings.TrimSpace(req.AssignedTo); v != "" {
		payload["assignedTo"] = v
	}
	if req.Pri != nil {
		if *req.Pri < 0 || *req.Pri > 4 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "pri must be between 0 and 4"})
			return
		}
		payload["pri"] = *req.Pri
	}
	if req.Estimate != nil {
		if *req.Estimate < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "estimate must be >= 0"})
			return
		}
		payload["estimate"] = *req.Estimate
	}
	if v := strings.TrimSpace(req.EstStarted); v != "" {
		if _, err := time.Parse("2006-01-02", v); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "est_started format must be YYYY-MM-DD"})
			return
		}
		payload["estStarted"] = v
	}
	if v := strings.TrimSpace(req.Deadline); v != "" {
		if _, err := time.Parse("2006-01-02", v); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "deadline format must be YYYY-MM-DD"})
			return
		}
		payload["deadline"] = v
	}
	if v := strings.TrimSpace(req.Status); v != "" {
		switch v {
		case "wait", "doing", "done", "closed", "pause", "cancel":
			payload["status"] = v
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "status must be one of wait|doing|done|closed|pause|cancel"})
			return
		}
	}
	if req.StoryID != nil && *req.StoryID > 0 {
		payload["story"] = *req.StoryID
	}
	if req.ModuleID != nil && *req.ModuleID > 0 {
		payload["module"] = *req.ModuleID
	}
	if req.FromBug != nil && *req.FromBug > 0 {
		payload["fromBug"] = *req.FromBug
	}
	if v := strings.TrimSpace(req.Mailto); v != "" {
		payload["mailto"] = v
	}
	if v := strings.TrimSpace(req.Color); v != "" {
		payload["color"] = v
	}
	for k, v := range req.CustomFields {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		if _, exists := payload[key]; exists {
			continue
		}
		payload[key] = v
	}
	if len(payload) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "empty update payload"})
		return
	}

	resp, err := callZentaoWrite(c.Request.Context(), sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APIUpdateTask(ctx, token, taskID, payload)
	})
	if err != nil {
		writeErr(c, err)
		return
	}
	if verifyErr := verifyTaskUpdatePersisted(c.Request.Context(), sub, taskID, payload); verifyErr != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": verifyErr.Error()})
		return
	}
	go func() {
		etl.SyncTasks()
	}()
	_ = db.WriteAudit(db.AuditInput{
		ActorUsername: sub,
		Action:        "zentao_update_task",
		TargetType:    "task",
		TargetID:      strconv.FormatInt(taskID, 10),
		Metadata: models.JSONB{
			"payload": payload,
			"result":  resp,
		},
		IP: c.ClientIP(),
		UA: c.GetHeader("User-Agent"),
	})
	c.JSON(http.StatusOK, gin.H{"ok": true, "result": resp})
}

func verifyTaskUpdatePersisted(ctx context.Context, sub string, taskID int64, payload map[string]any) error {
	taskResp, err := callZentaoWrite(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APIGetTask(ctx, token, taskID)
	})
	if err != nil {
		return fmt.Errorf("任务更新后回读失败：%w", err)
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
			if got == "" {
				got = "(empty)"
			}
			diffs = append(diffs, fmt.Sprintf("name=%q (期望 %q)", got, want))
		}
	}
	if v, ok := payload["type"]; ok {
		want := strings.TrimSpace(fmt.Sprintf("%v", v))
		got := strings.TrimSpace(zentao.ExtractTaskString(taskResp, "type"))
		if !strings.EqualFold(want, got) {
			if got == "" {
				got = "(empty)"
			}
			diffs = append(diffs, fmt.Sprintf("type=%q (期望 %q)", got, want))
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
			if got == "" {
				got = "(empty)"
			}
			diffs = append(diffs, fmt.Sprintf("estStarted=%q (期望 %q)", got, want))
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
			if got == "" {
				got = "(empty)"
			}
			diffs = append(diffs, fmt.Sprintf("%s=%q (期望 %q)", key, got, want))
		}
	}
	return diffs
}

func verifyEffortUpdatedPersisted(ctx context.Context, sub, mode string, taskID, effortID int64, payload, updateResp map[string]any) error {
	switch mode {
	case "api_v1":
		verifyTaskID := taskID
		if verifyTaskID <= 0 {
			if id, ok := zentao.ExtractEffortTaskID(updateResp); ok && id > 0 {
				verifyTaskID = id
			}
		}
		if verifyTaskID > 0 {
			efforts, err := loadTaskEffortsForVerify(ctx, sub, verifyTaskID)
			if err != nil {
				return fmt.Errorf("报工更新后回读失败：%w", err)
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

		effResp, err := callZentaoWrite(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
			return cli.APIGetEffort(ctx, token, effortID)
		})
		if err != nil {
			return fmt.Errorf("报工更新后回读失败：%w", err)
		}
		if diffs := diffEffortPayloadByMap(payload, effResp); len(diffs) > 0 {
			return fmt.Errorf("报工已请求更新，但禅道未生效：%s", strings.Join(diffs, "；"))
		}
		return nil

	case "webform":
		return verifyEffortUpdatedViaMySQL(taskID, effortID, payload)
	default:
		return fmt.Errorf("unknown effort update mode: %s", mode)
	}
}

func verifyEffortDeletedPersisted(ctx context.Context, sub, mode string, taskID, effortID int64) error {
	switch mode {
	case "api_v1":
		if taskID > 0 {
			efforts, err := loadTaskEffortsForVerify(ctx, sub, taskID)
			if err != nil {
				return fmt.Errorf("报工删除后回读失败：%w", err)
			}
			for _, e := range efforts {
				if e.ID == effortID {
					return fmt.Errorf("报工删除未生效：effort_id=%d 仍存在", effortID)
				}
			}
			return nil
		}

		_, err := callZentaoWrite(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
			return cli.APIGetEffort(ctx, token, effortID)
		})
		if err == nil {
			return fmt.Errorf("报工删除未生效：effort_id=%d 仍可查询", effortID)
		}
		if he, ok := zentao.IsAPIHTTPError(err); ok && he.Status == http.StatusNotFound {
			return nil
		}
		return fmt.Errorf("报工删除后回读失败：%w", err)

	case "webform":
		return verifyEffortDeletedViaMySQL(effortID)
	default:
		return fmt.Errorf("unknown effort delete mode: %s", mode)
	}
}

func loadTaskEffortsForVerify(ctx context.Context, sub string, taskID int64) ([]zentao.APITaskEffort, error) {
	resp, err := callZentaoWrite(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
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

func verifyEffortUpdatedViaMySQL(taskID, effortID int64, payload map[string]any) error {
	ztDB := db.GetZentao()
	if ztDB == nil {
		return fmt.Errorf("webform 编辑已提交，但当前未连接禅道 MySQL，无法确认是否生效")
	}

	var row source.ZtEffort
	err := ztDB.Table(source.ZtEffort{}.TableName()).
		Where("id = ?", effortID).
		Where("(deleted = '0' OR deleted = 0 OR deleted IS NULL)").
		First(&row).Error
	if err != nil {
		if errorsIsRecordNotFound(err) {
			return fmt.Errorf("webform 编辑后未查到 effort_id=%d，可能未生效", effortID)
		}
		return err
	}

	if diffs := diffEffortPayloadWithMySQLRow(payload, row); len(diffs) > 0 {
		return fmt.Errorf("webform 报工编辑未生效：%s", strings.Join(diffs, "；"))
	}

	if v, ok := payload["left"]; ok && taskID > 0 {
		wantLeft, okWant := float64FromAny(v)
		if okWant {
			var taskRow struct {
				Left float64 `gorm:"column:left"`
			}
			if err := ztDB.Table(source.ZtTask{}.TableName()).Select("`left`").Where("id = ?", taskID).Take(&taskRow).Error; err == nil {
				if !floatAlmostEqual(taskRow.Left, wantLeft) {
					return fmt.Errorf("webform 报工编辑未生效：task.left=%g (期望 %g)", taskRow.Left, wantLeft)
				}
			}
		}
	}

	return nil
}

func verifyEffortDeletedViaMySQL(effortID int64) error {
	ztDB := db.GetZentao()
	if ztDB == nil {
		return fmt.Errorf("webform 删除已提交，但当前未连接禅道 MySQL，无法确认是否生效")
	}
	// Webform 删除在少数环境下会有轻微延迟，短轮询可避免把“稍后生效”误判为失败。
	const maxAttempts = 5
	for attempt := 0; attempt < maxAttempts; attempt++ {
		var row source.ZtEffort
		err := ztDB.Table(source.ZtEffort{}.TableName()).
			Where("id = ?", effortID).
			Where("(deleted = '0' OR deleted = 0 OR deleted IS NULL)").
			First(&row).Error
		if err != nil {
			if errorsIsRecordNotFound(err) {
				return nil
			}
			return err
		}
		if attempt < maxAttempts-1 {
			time.Sleep(250 * time.Millisecond)
			continue
		}
		return fmt.Errorf("webform 报工删除未生效：effort_id=%d 仍存在", effortID)
	}
	return fmt.Errorf("webform 报工删除未生效：effort_id=%d 仍存在", effortID)
}

func diffEffortPayloadWithMySQLRow(payload map[string]any, row source.ZtEffort) []string {
	diffs := make([]string, 0, 3)
	if v, ok := payload["work"]; ok {
		want := strings.TrimSpace(fmt.Sprintf("%v", v))
		got := strings.TrimSpace(row.Work)
		if want != got {
			diffs = append(diffs, fmt.Sprintf("work=%q (期望 %q)", got, want))
		}
	}
	if v, ok := payload["date"]; ok {
		want := zentao.NormalizeDateYMD(fmt.Sprintf("%v", v))
		got := ""
		if row.Date != nil {
			got = row.Date.Format("2006-01-02")
		}
		if want != got {
			diffs = append(diffs, fmt.Sprintf("date=%q (期望 %q)", got, want))
		}
	}
	if v, ok := payload["consumed"]; ok {
		want, okWant := float64FromAny(v)
		if !okWant || !floatAlmostEqual(row.Consumed, want) {
			diffs = append(diffs, fmt.Sprintf("consumed=%g (期望 %g)", row.Consumed, want))
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

func floatFromAny(v any) (float64, bool) {
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

func errorsIsRecordNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

func callZentaoWrite(ctx context.Context, sub string, call func(context.Context, *zentao.APIClient, string) (map[string]any, error)) (map[string]any, error) {
	baseURL := strings.TrimSpace(config.Global.ZentaoBaseURL)
	if baseURL == "" {
		return nil, &zentao.APIHTTPError{Status: http.StatusBadRequest, URL: "config", Body: "zentao base_url not configured"}
	}
	token, err := ensureAPIToken(ctx, sub)
	if err != nil {
		return nil, err
	}
	cli := zentao.NewAPIClient(baseURL)
	resp, err := call(ctx, cli, token)
	if err == nil {
		return resp, nil
	}
	if zentao.IsAPIUnauthorizedError(err) {
		deleteAPIToken(ctx, sub)
		newToken, reloginErr := loginAndCacheAPIToken(ctx, sub)
		if reloginErr != nil {
			return nil, reloginErr
		}
		return call(ctx, cli, newToken)
	}
	return nil, err
}

func lookupEffortTaskID(effortID int64) int64 {
	var row models.LocalEffort
	if err := db.PG.First(&row, effortID).Error; err != nil {
		return 0
	}
	if row.ObjectType != "" && row.ObjectType != "task" {
		return 0
	}
	return row.ObjectID
}

func updateEffortToZentao(ctx context.Context, sub string, taskID, effortID int64, payload map[string]any, req *updateZentaoEffortBody) (map[string]any, string, error) {
	resp, err := callZentaoWrite(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APIUpdateEffort(ctx, token, taskID, effortID, payload)
	})
	if err == nil {
		return resp, "api_v1", nil
	}
	if !zentao.IsEffortAPIMissing(err) {
		return nil, "", err
	}
	web, webErr := submitEffortEditViaWebform(ctx, sub, effortID, req)
	if webErr != nil {
		return nil, "", fmt.Errorf("zentao api has no effort update route and webform failed: %w", webErr)
	}
	return map[string]any{"ok": true, "webform": web}, "webform", nil
}

func deleteEffortFromZentao(ctx context.Context, sub string, taskID, effortID int64) (map[string]any, string, error) {
	_, err := callZentaoWrite(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return map[string]any{"ok": true}, cli.APIDeleteEffort(ctx, token, taskID, effortID)
	})
	if err == nil {
		return map[string]any{"ok": true}, "api_v1", nil
	}
	if !zentao.IsEffortAPIMissing(err) {
		return nil, "", err
	}
	web, webErr := submitEffortDeleteViaWebform(ctx, sub, effortID)
	if webErr != nil {
		return nil, "", fmt.Errorf("zentao api has no effort delete route and webform failed: %w", webErr)
	}
	return map[string]any{"ok": true, "webform": web}, "webform", nil
}

func submitEffortEditViaWebform(ctx context.Context, sub string, effortID int64, req *updateZentaoEffortBody) (*zentao.CreateEffortResult, error) {
	baseURL := strings.TrimSpace(config.Global.ZentaoBaseURL)
	cookies, err := loadZentaoSessionCookies(ctx, sub)
	if err != nil {
		return nil, err
	}
	in := zentao.EditTaskEffortInput{EffortID: effortID}
	if req != nil {
		in.WorkDate = strings.TrimSpace(req.WorkDate)
		in.Work = strings.TrimSpace(req.Work)
		if v := strings.TrimSpace(string(req.Consumed)); v != "" {
			in.Consumed = v
		}
		if v := strings.TrimSpace(string(req.Left)); v != "" {
			in.Left = v
		}
	}
	if in.Consumed == "" {
		return nil, fmt.Errorf("consumed is required for webform edit")
	}
	return zentao.UpdateTaskEffortByWebForm(ctx, baseURL, cookies, in)
}

func submitEffortDeleteViaWebform(ctx context.Context, sub string, effortID int64) (*zentao.CreateEffortResult, error) {
	baseURL := strings.TrimSpace(config.Global.ZentaoBaseURL)
	cookies, err := loadZentaoSessionCookies(ctx, sub)
	if err != nil {
		return nil, err
	}
	return zentao.DeleteTaskEffortByWebForm(ctx, baseURL, cookies, effortID)
}

func writeEffortWriteErr(c *gin.Context, err error) {
	if zentao.IsAuthExpiredError(err) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"ok":    false,
			"error": "禅道网页会话已过期，请在「禅道授权」重新绑定后再编辑报工",
		})
		return
	}
	writeErr(c, err)
}

func writeErr(c *gin.Context, err error) {
	if he, ok := zentao.IsAPIHTTPError(err); ok {
		detail := strings.TrimSpace(he.Body)
		if detail == "" {
			detail = "(无响应正文)"
		}
		c.JSON(http.StatusBadGateway, gin.H{
			"ok":         false,
			"error":      fmt.Sprintf("禅道接口错误 HTTP %d：%s", he.Status, detail),
			"api_status": he.Status,
			"api_body":   he.Body,
			"api_url":    he.URL,
		})
		return
	}
	if strings.Contains(strings.ToLower(err.Error()), "login") || strings.Contains(strings.ToLower(err.Error()), "token") {
		c.JSON(http.StatusUnauthorized, gin.H{
			"ok":    false,
			"error": "禅道 API 登录失败，请在「禅道授权」页面确认账号密码：" + err.Error(),
		})
		return
	}
	c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": err.Error()})
}
