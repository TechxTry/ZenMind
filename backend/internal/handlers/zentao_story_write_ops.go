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

type createZentaoStoryBody struct {
	Title      string   `json:"title"`
	ProductID  int64    `json:"product_id"`
	AssignedTo string   `json:"assigned_to"`
	Estimate   *float64 `json:"estimate"`
	Status     string   `json:"status"`
	Spec       string   `json:"spec"`
	Pri        *int     `json:"pri"`
}

type updateZentaoStoryBody struct {
	Title      string   `json:"title"`
	ProductID  int64    `json:"product_id"`
	AssignedTo string   `json:"assigned_to"`
	Estimate   *float64 `json:"estimate"`
	Status     string   `json:"status"`
	Spec       string   `json:"spec"`
	Pri        *int     `json:"pri"`
}

// CreateZentaoStory POST /api/zentao/stories
func CreateZentaoStory(c *gin.Context) {
	sub := currentSub(c)
	if sub == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing sub"})
		return
	}

	var req createZentaoStoryBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}
	if req.ProductID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "product_id is required"})
		return
	}

	payload := map[string]any{
		"title":   title,
		"product": req.ProductID,
	}
	if assigned := strings.TrimSpace(req.AssignedTo); assigned != "" {
		payload["assignedTo"] = assigned
	}
	if req.Estimate != nil {
		if *req.Estimate < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "estimate must be >= 0"})
			return
		}
		payload["estimate"] = *req.Estimate
	}
	if status := strings.ToLower(strings.TrimSpace(req.Status)); status != "" {
		if !isValidStoryStatus(status) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "status must be one of draft|reviewing|active|changing|changed|closed"})
			return
		}
		payload["status"] = status
	}
	if spec := strings.TrimSpace(req.Spec); spec != "" {
		payload["spec"] = spec
	}
	if req.Pri != nil {
		if *req.Pri < 0 || *req.Pri > 4 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "pri must be between 0 and 4"})
			return
		}
		payload["pri"] = *req.Pri
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 25*time.Second)
	defer cancel()

	resp, err := callZentaoWrite(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APICreateStory(ctx, token, payload)
	})
	if err != nil {
		writeErr(c, err)
		return
	}

	storyID := extractCreatedStoryID(resp)
	if storyID <= 0 {
		c.JSON(http.StatusBadGateway, gin.H{
			"ok":    false,
			"error": "禅道返回成功但未给出 story_id，无法确认需求已创建。请到禅道需求列表核对后再重试。",
		})
		return
	}
	if verifyErr := verifyStoryCreatedPersisted(ctx, sub, storyID); verifyErr != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": verifyErr.Error()})
		return
	}

	go etl.SyncStories()
	_ = db.WriteAudit(db.AuditInput{
		ActorUsername: sub,
		Action:        "zentao_create_story",
		TargetType:    "story",
		TargetID:      strconv.FormatInt(storyID, 10),
		Metadata: models.JSONB{
			"payload": payload,
			"result":  resp,
		},
		IP: c.ClientIP(),
		UA: c.GetHeader("User-Agent"),
	})
	c.JSON(http.StatusOK, gin.H{"ok": true, "story_id": storyID, "result": resp})
}

// UpdateZentaoStory PATCH /api/zentao/stories/:id
func UpdateZentaoStory(c *gin.Context) {
	sub := currentSub(c)
	if sub == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing sub"})
		return
	}
	storyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || storyID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid story id"})
		return
	}

	var req updateZentaoStoryBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	payload := make(map[string]any)
	if title := strings.TrimSpace(req.Title); title != "" {
		payload["title"] = title
	}
	if req.ProductID > 0 {
		payload["product"] = req.ProductID
	}
	if assigned := strings.TrimSpace(req.AssignedTo); assigned != "" {
		payload["assignedTo"] = assigned
	}
	if req.Estimate != nil {
		if *req.Estimate < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "estimate must be >= 0"})
			return
		}
		payload["estimate"] = *req.Estimate
	}
	if status := strings.ToLower(strings.TrimSpace(req.Status)); status != "" {
		if !isValidStoryStatus(status) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "status must be one of draft|reviewing|active|changing|changed|closed"})
			return
		}
		payload["status"] = status
	}
	if spec := strings.TrimSpace(req.Spec); spec != "" {
		payload["spec"] = spec
	}
	if req.Pri != nil {
		if *req.Pri < 0 || *req.Pri > 4 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "pri must be between 0 and 4"})
			return
		}
		payload["pri"] = *req.Pri
	}
	if len(payload) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "empty update payload"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 25*time.Second)
	defer cancel()

	resp, err := callZentaoWrite(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APIUpdateStory(ctx, token, storyID, payload)
	})
	if err != nil {
		writeErr(c, err)
		return
	}
	if verifyErr := verifyStoryUpdatedPersisted(ctx, sub, storyID, payload); verifyErr != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": verifyErr.Error()})
		return
	}

	go etl.SyncStories()
	_ = db.WriteAudit(db.AuditInput{
		ActorUsername: sub,
		Action:        "zentao_update_story",
		TargetType:    "story",
		TargetID:      strconv.FormatInt(storyID, 10),
		Metadata: models.JSONB{
			"payload": payload,
			"result":  resp,
		},
		IP: c.ClientIP(),
		UA: c.GetHeader("User-Agent"),
	})
	c.JSON(http.StatusOK, gin.H{"ok": true, "result": resp})
}

// DeleteZentaoStory DELETE /api/zentao/stories/:id
func DeleteZentaoStory(c *gin.Context) {
	sub := currentSub(c)
	if sub == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing sub"})
		return
	}
	storyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || storyID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid story id"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 25*time.Second)
	defer cancel()

	resp, mode, err := deleteStoryFromZentao(ctx, sub, storyID)
	if err != nil {
		writeErr(c, err)
		return
	}
	if verifyErr := verifyStoryDeletedPersisted(ctx, sub, storyID, mode); verifyErr != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": verifyErr.Error()})
		return
	}

	go etl.SyncStories()
	_ = db.WriteAudit(db.AuditInput{
		ActorUsername: sub,
		Action:        "zentao_delete_story",
		TargetType:    "story",
		TargetID:      strconv.FormatInt(storyID, 10),
		Metadata: models.JSONB{
			"mode":   mode,
			"result": resp,
		},
		IP: c.ClientIP(),
		UA: c.GetHeader("User-Agent"),
	})
	c.JSON(http.StatusOK, gin.H{"ok": true, "mode": mode, "result": resp})
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
	_, err := callZentaoWrite(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APIGetStory(ctx, token, storyID)
	})
	if err != nil {
		return fmt.Errorf("需求创建后回读失败：%w", err)
	}
	return nil
}

func verifyStoryUpdatedPersisted(ctx context.Context, sub string, storyID int64, payload map[string]any) error {
	storyResp, err := callZentaoWrite(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APIGetStory(ctx, token, storyID)
	})
	if err != nil {
		return fmt.Errorf("需求更新后回读失败：%w", err)
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
	return diffs
}

func deleteStoryFromZentao(ctx context.Context, sub string, storyID int64) (map[string]any, string, error) {
	_, err := callZentaoWrite(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return map[string]any{"ok": true}, cli.APIDeleteStory(ctx, token, storyID)
	})
	if err == nil {
		return map[string]any{"ok": true}, "api_v1_delete", nil
	}
	if !zentao.IsStoryAPIMissing(err) {
		return nil, "", err
	}

	resp, closeErr := callZentaoWrite(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APIUpdateStory(ctx, token, storyID, map[string]any{"status": "closed"})
	})
	if closeErr != nil {
		return nil, "", fmt.Errorf("zentao api has no story delete route and close fallback failed: %w", closeErr)
	}
	return map[string]any{"ok": true, "close_result": resp}, "api_v1_close", nil
}

func verifyStoryDeletedPersisted(ctx context.Context, sub string, storyID int64, mode string) error {
	storyResp, err := callZentaoWrite(ctx, sub, func(ctx context.Context, cli *zentao.APIClient, token string) (map[string]any, error) {
		return cli.APIGetStory(ctx, token, storyID)
	})
	if err != nil {
		if he, ok := zentao.IsAPIHTTPError(err); ok && he.Status == http.StatusNotFound {
			return nil
		}
		return fmt.Errorf("需求删除后回读失败：%w", err)
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
