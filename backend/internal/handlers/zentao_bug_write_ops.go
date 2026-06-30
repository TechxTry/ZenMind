package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"zenmind/internal/db"
	"zenmind/internal/etl"
	"zenmind/internal/models"
	"zenmind/internal/zentao"

	"github.com/gin-gonic/gin"
)

type createZentaoBugBody struct {
	Title      string `json:"title"`
	Steps      string `json:"steps"`
	Execution  int64  `json:"execution_id"`
	ProjectID  int64  `json:"project_id"`
	ProductID  int64  `json:"product_id"`
	AssignedTo string `json:"assigned_to"`
	Severity   *int   `json:"severity"`
	StoryID    int64  `json:"story_id"`
	TaskID     int64  `json:"task_id"`
	Type       string `json:"type"`
}

type updateZentaoBugBody struct {
	Title      string `json:"title"`
	Steps      string `json:"steps"`
	Execution  int64  `json:"execution_id"`
	AssignedTo string `json:"assigned_to"`
	Severity   *int   `json:"severity"`
	Status     string `json:"status"`
	Resolution string `json:"resolution"`
	StoryID    int64  `json:"story_id"`
	TaskID     int64  `json:"task_id"`
}

// CreateZentaoBug POST /api/zentao/bugs
func CreateZentaoBug(c *gin.Context) {
	sub := currentSub(c)
	if sub == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing sub"})
		return
	}

	var req createZentaoBugBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}

	payload := map[string]any{
		"title": title,
	}
	if steps := strings.TrimSpace(req.Steps); steps != "" {
		payload["steps"] = steps
	}
	if req.Execution > 0 {
		payload["execution"] = req.Execution
	}
	if req.ProjectID > 0 {
		payload["project"] = req.ProjectID
	}
	if req.ProductID > 0 {
		payload["product"] = req.ProductID
	}
	if assigned := strings.TrimSpace(req.AssignedTo); assigned != "" {
		payload["assignedTo"] = assigned
	}
	if req.Severity != nil {
		if *req.Severity < 1 || *req.Severity > 4 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "severity must be between 1 and 4"})
			return
		}
		payload["severity"] = *req.Severity
	}
	if req.StoryID > 0 {
		payload["story"] = req.StoryID
	}
	if req.TaskID > 0 {
		payload["task"] = req.TaskID
	}
	if bugType := strings.TrimSpace(req.Type); bugType != "" {
		payload["type"] = bugType
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 25*time.Second)
	defer cancel()

	resp, err := callZentaoWrite(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APICreateBug(ctx, token, payload)
	})
	if err != nil {
		writeErr(c, err)
		return
	}

	bugID := extractCreatedBugID(resp)
	if bugID <= 0 {
		c.JSON(http.StatusBadGateway, gin.H{
			"ok":    false,
			"error": "禅道返回成功但未给出 bug_id，无法确认缺陷已创建。请到禅道缺陷列表核对后再重试。",
		})
		return
	}
	if verifyErr := verifyBugCreatedPersisted(ctx, sub, bugID); verifyErr != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": verifyErr.Error()})
		return
	}

	go etl.SyncBugs()
	_ = db.WriteAudit(db.AuditInput{
		ActorUsername: sub,
		Action:        "zentao_create_bug",
		TargetType:    "bug",
		TargetID:      strconv.FormatInt(bugID, 10),
		Metadata: models.JSONB{
			"payload": payload,
			"result":  resp,
		},
		IP: c.ClientIP(),
		UA: c.GetHeader("User-Agent"),
	})
	c.JSON(http.StatusOK, gin.H{"ok": true, "bug_id": bugID, "result": resp})
}

// UpdateZentaoBug PATCH /api/zentao/bugs/:id
func UpdateZentaoBug(c *gin.Context) {
	sub := currentSub(c)
	if sub == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing sub"})
		return
	}
	bugID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || bugID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bug id"})
		return
	}

	var req updateZentaoBugBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	payload := make(map[string]any)
	if title := strings.TrimSpace(req.Title); title != "" {
		payload["title"] = title
	}
	if steps := strings.TrimSpace(req.Steps); steps != "" {
		payload["steps"] = steps
	}
	if req.Execution > 0 {
		payload["execution"] = req.Execution
	}
	if assigned := strings.TrimSpace(req.AssignedTo); assigned != "" {
		payload["assignedTo"] = assigned
	}
	if req.Severity != nil {
		if *req.Severity < 1 || *req.Severity > 4 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "severity must be between 1 and 4"})
			return
		}
		payload["severity"] = *req.Severity
	}
	if status := strings.ToLower(strings.TrimSpace(req.Status)); status != "" {
		switch status {
		case "active", "resolved", "closed", "wait", "activating":
			payload["status"] = status
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "status must be one of active|resolved|closed|wait|activating"})
			return
		}
	}
	if resolution := strings.ToLower(strings.TrimSpace(req.Resolution)); resolution != "" {
		payload["resolution"] = resolution
	}
	if req.StoryID > 0 {
		payload["story"] = req.StoryID
	}
	if req.TaskID > 0 {
		payload["task"] = req.TaskID
	}
	if len(payload) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "empty update payload"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 25*time.Second)
	defer cancel()

	resp, err := callZentaoWrite(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APIUpdateBug(ctx, token, bugID, payload)
	})
	if err != nil {
		writeErr(c, err)
		return
	}
	if verifyErr := verifyBugUpdatedPersisted(ctx, sub, bugID, payload); verifyErr != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": verifyErr.Error()})
		return
	}

	go etl.SyncBugs()
	_ = db.WriteAudit(db.AuditInput{
		ActorUsername: sub,
		Action:        "zentao_update_bug",
		TargetType:    "bug",
		TargetID:      strconv.FormatInt(bugID, 10),
		Metadata: models.JSONB{
			"payload": payload,
			"result":  resp,
		},
		IP: c.ClientIP(),
		UA: c.GetHeader("User-Agent"),
	})
	c.JSON(http.StatusOK, gin.H{"ok": true, "result": resp})
}

// DeleteZentaoBug DELETE /api/zentao/bugs/:id
func DeleteZentaoBug(c *gin.Context) {
	sub := currentSub(c)
	if sub == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing sub"})
		return
	}
	bugID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || bugID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bug id"})
		return
	}
	resolution := strings.ToLower(strings.TrimSpace(c.Query("resolution")))
	if resolution == "" {
		resolution = "bydesign"
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 25*time.Second)
	defer cancel()

	resp, mode, err := deleteBugFromZentao(ctx, sub, bugID, resolution)
	if err != nil {
		writeErr(c, err)
		return
	}
	if verifyErr := verifyBugDeletedPersisted(ctx, sub, bugID, mode); verifyErr != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": verifyErr.Error()})
		return
	}

	go etl.SyncBugs()
	_ = db.WriteAudit(db.AuditInput{
		ActorUsername: sub,
		Action:        "zentao_delete_bug",
		TargetType:    "bug",
		TargetID:      strconv.FormatInt(bugID, 10),
		Metadata: models.JSONB{
			"mode":   mode,
			"result": resp,
		},
		IP: c.ClientIP(),
		UA: c.GetHeader("User-Agent"),
	})
	c.JSON(http.StatusOK, gin.H{"ok": true, "mode": mode, "result": resp})
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
	_, err := callZentaoWrite(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APIGetBug(ctx, token, bugID)
	})
	if err != nil {
		return fmt.Errorf("缺陷创建后回读失败：%w", err)
	}
	return nil
}

func verifyBugUpdatedPersisted(ctx context.Context, sub string, bugID int64, payload map[string]any) error {
	bugResp, err := callZentaoWrite(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APIGetBug(ctx, token, bugID)
	})
	if err != nil {
		return fmt.Errorf("缺陷更新后回读失败：%w", err)
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
	return diffs
}

func deleteBugFromZentao(ctx context.Context, sub string, bugID int64, resolution string) (map[string]any, string, error) {
	_, err := callZentaoWrite(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return map[string]any{"ok": true}, cli.APIDeleteBug(ctx, token, bugID)
	})
	if err == nil {
		return map[string]any{"ok": true}, "api_v1_delete", nil
	}
	if !zentao.IsBugAPIMissing(err) {
		return nil, "", err
	}

	payload := map[string]any{
		"status": "closed",
	}
	if strings.TrimSpace(resolution) != "" {
		payload["resolution"] = resolution
	}
	resp, closeErr := callZentaoWrite(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APIUpdateBug(ctx, token, bugID, payload)
	})
	if closeErr != nil {
		return nil, "", fmt.Errorf("zentao api has no bug delete route and close fallback failed: %w", closeErr)
	}
	return map[string]any{"ok": true, "close_result": resp}, "api_v1_close", nil
}

func verifyBugDeletedPersisted(ctx context.Context, sub string, bugID int64, mode string) error {
	bugResp, err := callZentaoWrite(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APIGetBug(ctx, token, bugID)
	})
	if err != nil {
		if he, ok := zentao.IsAPIHTTPError(err); ok && he.Status == http.StatusNotFound {
			return nil
		}
		return fmt.Errorf("缺陷删除后回读失败：%w", err)
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
