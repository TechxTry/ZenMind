package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"zenmind/internal/config"
	"zenmind/internal/db"
	"zenmind/internal/etl"
	"zenmind/internal/redisclient"
	"zenmind/internal/source"
	"zenmind/internal/zentao"

	"github.com/gin-gonic/gin"
)

// hourFormValue accepts JSON numbers (from frontend InputNumber) or strings.
type hourFormValue string

func (h *hourFormValue) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return fmt.Errorf("invalid hour value")
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*h = hourFormValue(strings.TrimSpace(s))
		return nil
	}
	var n float64
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*h = hourFormValue(strconv.FormatFloat(n, 'f', -1, 64))
	return nil
}

func (h hourFormValue) Float() (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(string(h)), 64)
}

type createZentaoEffortBody struct {
	TaskID   int64         `json:"task_id" binding:"required"`
	WorkDate string        `json:"work_date"` // YYYY-MM-DD, optional
	Work     string        `json:"work" binding:"required"`
	Consumed hourFormValue `json:"consumed" binding:"required"`
	Left     hourFormValue `json:"left" binding:"required"`
}

// CreateZentaoEffort POST /api/zentao/efforts
//
// 决策树：
//  1. 尝试 API 登录换 Token
//     - 成功 → 只走 API；POST /tasks/{id}/efforts；401 自动换 Token 重试一次；其它错误原样返回
//     - 失败且是 HTTP 404 / 405（说明没装 API 模块）→ 回落 webform
//     - 失败且是其它（密码错、网络）→ 直接报错
//  2. Zentao MySQL 可达时做落库校验
func CreateZentaoEffort(c *gin.Context) {
	sub := currentSub(c)
	if sub == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing sub"})
		return
	}
	var req createZentaoEffortBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.TaskID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task_id"})
		return
	}
	if strings.TrimSpace(req.Work) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "work is required"})
		return
	}
	if strings.TrimSpace(string(req.Consumed)) == "" || strings.TrimSpace(string(req.Left)) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "consumed and left are required"})
		return
	}
	consumedF, err := req.Consumed.Float()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid consumed: " + err.Error()})
		return
	}
	leftF, err := req.Left.Float()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid left: " + err.Error()})
		return
	}

	baseURL := strings.TrimSpace(config.Global.ZentaoBaseURL)
	if baseURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "zentao base_url not configured"})
		return
	}

	workDate := strings.TrimSpace(req.WorkDate)
	if workDate == "" {
		workDate = time.Now().Format("2006-01-02")
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 25*time.Second)
	defer cancel()

	// === Step 1: 先试 API 登录，决定本次走哪条路径 ===
	token, loginErr := ensureAPIToken(ctx, sub)
	if loginErr != nil {
		// 仅当 API 入口不存在（404/405）时才回落 webform，适配开源禅道 ≤12
		if isAPIEndpointMissing(loginErr) {
			log.Printf("[effort] api login endpoint missing, fallback to webform: %v", loginErr)
			tryWebformPath(c, ctx, sub, baseURL, req.TaskID, workDate, req.Work, string(req.Consumed), string(req.Left), consumedF, loginErr.Error())
			return
		}
		// 其它 API 登录错误（密码错 / 网络 / Redis 故障等）直接报告，不伪装
		log.Printf("[effort] api login failed: %v", loginErr)
		c.JSON(apiLoginErrorStatus(loginErr), gin.H{
			"ok":    false,
			"mode":  "api_v1_login_failed",
			"error": "禅道 API 登录失败，请去「禅道授权」确认账号密码是否仍有效：" + loginErr.Error(),
		})
		return
	}

	// === Step 2: API 登录成功 → 依次尝试 estimate-modern / estimate-legacy / efforts-fallback ===
	cli := zentao.NewAPIClient(baseURL)
	in := zentao.APICreateTaskEffortInput{
		TaskID:   req.TaskID,
		WorkDate: workDate,
		Work:     req.Work,
		Consumed: consumedF,
		Left:     leftF,
	}
	winner, attempts, fatal := runEffortVariants(ctx, cli, sub, &token, in)
	if fatal != nil {
		c.JSON(fatal.status, fatal.body)
		return
	}
	if winner == nil {
		// 所有变体都失败 / 假成功
		c.JSON(http.StatusBadGateway, gin.H{
			"ok":       false,
			"mode":     "api_v1",
			"error":    "禅道 API v1 三种变体都未能真正插入 effort 记录。",
			"hint":     "看下方 attempts 里每个 variant 的 verify_matched —— 如果都是 false，说明禅道 POST 后立即 GET 任务日志列表也找不到本次记录。这种实例通常对 estimate 入口做了定制，只更新 task 字段不创建日志。请在浏览器里手动报一次工，DevTools → Network 把那条 POST 的 URL + Request Payload 贴回来。",
			"attempts": attempts,
		})
		return
	}

	// === Step 3: 落库校验 + 回填 ===
	respondEffortSuccess(c, gin.H{
		"ok":        true,
		"message":   "effort submitted via api_v1",
		"mode":      "api_v1",
		"effort_id": winner.ID,
		"result":    winner,
		"attempts":  attempts,
	}, req.TaskID, workDate, req.Work, consumedF)
}

type fatalResp struct {
	status int
	body   gin.H
}

// runEffortVariants 依次尝试 estimate-modern / estimate-legacy / efforts-fallback，
// 返回：胜出的 result（如果有）、所有尝试详情、致命错误（如 401 重登也失败、403、5xx）。
//
// `*token` 会在 Token 过期时被原地刷新。
func runEffortVariants(
	ctx context.Context,
	cli *zentao.APIClient,
	sub string,
	token *string,
	in zentao.APICreateTaskEffortInput,
) (*zentao.APICreateTaskEffortResult, []gin.H, *fatalResp) {
	var attempts []gin.H

	for _, variant := range zentao.AllEffortVariants {
		r, err := cli.APICreateTaskEffortByVariant(ctx, *token, in, variant)

		// 401：刷新 Token 一次，再用同一变体重试
		if err != nil && zentao.IsAPIUnauthorizedError(err) {
			log.Printf("[effort] variant=%s 401, refreshing token", variant)
			deleteAPIToken(ctx, sub)
			newToken, reloginErr := loginAndCacheAPIToken(ctx, sub)
			if reloginErr != nil {
				return nil, attempts, &fatalResp{
					status: http.StatusUnauthorized,
					body: gin.H{
						"ok":       false,
						"mode":     "api_v1_relogin_failed",
						"error":    "Token 过期后重新登录失败：" + reloginErr.Error(),
						"attempts": attempts,
					},
				}
			}
			*token = newToken
			r, err = cli.APICreateTaskEffortByVariant(ctx, *token, in, variant)
		}

		entry := gin.H{
			"variant": variant,
			"ok":      err == nil,
		}
		if r != nil {
			entry["used_url"] = r.UsedURL
			entry["task_consumed_after"] = r.TaskConsumedAfter
			entry["raw_body"] = r.RawBody
			entry["verify_attempted"] = r.VerifyAttempted
			entry["verify_matched"] = r.VerifyMatched
			if r.VerifyError != "" {
				entry["verify_error"] = r.VerifyError
			}
		}
		if err != nil {
			entry["error"] = err.Error()
			if errors.Is(err, zentao.ErrAPIEffortNotPersisted) {
				entry["reason"] = "api_returned_200_but_task_consumed_did_not_increase"
			}
			if he, ok := zentao.IsAPIHTTPError(err); ok {
				entry["api_status"] = he.Status
				entry["api_body"] = he.Body
				entry["api_url"] = he.URL
			}
		}
		attempts = append(attempts, entry)

		// 成功
		if err == nil {
			log.Printf("[effort] variant=%s succeeded, task_consumed_after=%v", variant, r.TaskConsumedAfter)
			return r, attempts, nil
		}

		log.Printf("[effort] variant=%s failed: %v", variant, err)

		// 假 200 成功 → 切下一个变体
		if errors.Is(err, zentao.ErrAPIEffortNotPersisted) {
			continue
		}
		// 路径或字段不对 → 切下一个变体
		if he, ok := zentao.IsAPIHTTPError(err); ok {
			if he.Status == http.StatusBadRequest ||
				he.Status == http.StatusNotFound ||
				he.Status == http.StatusMethodNotAllowed ||
				he.Status == http.StatusUnprocessableEntity {
				continue
			}
			// 其它 HTTP 错误（403 / 5xx）→ 致命，立即返回
			return nil, attempts, &fatalResp{
				status: http.StatusBadGateway,
				body: gin.H{
					"ok":         false,
					"mode":       "api_v1",
					"error":      err.Error(),
					"api_status": he.Status,
					"api_body":   he.Body,
					"api_url":    he.URL,
					"hint":       hintFromStatus(he.Status),
					"attempts":   attempts,
				},
			}
		}

		// 网络等其它错误 → 致命
		return nil, attempts, &fatalResp{
			status: http.StatusBadGateway,
			body: gin.H{
				"ok":       false,
				"mode":     "api_v1",
				"error":    err.Error(),
				"attempts": attempts,
			},
		}
	}

	return nil, attempts, nil
}

func hintFromStatus(status int) string {
	switch status {
	case http.StatusForbidden:
		return "API 返回 403：当前账号可能无权对这个任务写报工，换一个自己 assignedTo 的任务试试。"
	case http.StatusUnprocessableEntity:
		return "API 返回 422：字段校验失败，常见原因是 consumed/left 为 0 或 date 格式不符；请把 api_body 的 JSON 贴回后端调整。"
	case http.StatusNotFound:
		return "API 返回 404：路径不存在或任务在当前账号视角下不可见。"
	}
	return ""
}

// tryWebformPath 仅在 API 入口不存在时被调用（开源禅道 ≤12）。
func tryWebformPath(c *gin.Context, ctx context.Context, sub, baseURL string, taskID int64, workDate, work, consumed, left string, consumedF float64, apiErrStr string) {
	webResult, webErr := submitViaWebform(ctx, sub, baseURL, taskID, workDate, work, consumed, left)
	if webErr != nil {
		status := http.StatusBadGateway
		if zentao.IsAuthExpiredError(webErr) {
			status = http.StatusUnauthorized
		}
		c.JSON(status, gin.H{
			"ok":            false,
			"mode":          "webform_fallback_failed",
			"error":         webErr.Error(),
			"api_error":     apiErrStr,
			"webform_error": webErr.Error(),
		})
		return
	}
	respondEffortSuccess(c, gin.H{
		"ok":        true,
		"message":   "effort submitted via webform",
		"mode":      "webform",
		"result":    webResult,
		"api_error": apiErrStr,
	}, taskID, workDate, work, consumedF)
}

// loadZentaoSessionCookies returns cookies saved by 「禅道授权」 bind.
func loadZentaoSessionCookies(ctx context.Context, sub string) ([]*http.Cookie, error) {
	if err := ensureRedis(ctx); err != nil {
		return nil, fmt.Errorf("redis unavailable: %w", err)
	}
	key := ztSessKeyPrefix + sub
	raw, err := redisclient.Client.Get(ctx, key).Result()
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("zentao session not bound or expired")
	}
	var cookies []*http.Cookie
	if err := json.Unmarshal([]byte(raw), &cookies); err != nil {
		return nil, fmt.Errorf("invalid session cookie payload: %w", err)
	}
	return cookies, nil
}

// submitViaWebform：保留给开源禅道 ≤12 的 form POST 路径。
func submitViaWebform(ctx context.Context, sub, baseURL string, taskID int64, workDate, work, consumed, left string) (*zentao.CreateEffortResult, error) {
	cookies, err := loadZentaoSessionCookies(ctx, sub)
	if err != nil {
		return nil, err
	}
	return zentao.CreateTaskEffortByWebForm(ctx, baseURL, cookies, zentao.CreateTaskEffortInput{
		TaskID:   taskID,
		WorkDate: workDate,
		Work:     work,
		Consumed: strings.TrimSpace(consumed),
		Left:     strings.TrimSpace(left),
	})
}

// isAPIEndpointMissing：只在 /api.php/v1/tokens 明确返回 404/405/路由缺失时才判定为「无 API 模块」。
func isAPIEndpointMissing(err error) bool {
	if err == nil {
		return false
	}
	if he, ok := zentao.IsAPIHTTPError(err); ok {
		return he.Status == http.StatusNotFound || he.Status == http.StatusMethodNotAllowed
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such route") || strings.Contains(msg, "page not found")
}

func apiLoginErrorStatus(err error) int {
	if he, ok := zentao.IsAPIHTTPError(err); ok {
		if he.Status == http.StatusUnauthorized {
			return http.StatusUnauthorized
		}
	}
	return http.StatusBadGateway
}

// respondEffortSuccess 决定是否需要 MySQL 兜底，再返回 200。
//
//   - mode=api_v1：APIClient 内部已经识别过「假 200」（task.consumed 没增长 → 切下一变体）。
//     能走到这里说明禅道源端的 task 详情已经反映了本次提交，是来自禅道自身的强信号。
//     再去查禅道 MySQL 容易被主从延迟坑（刚写入 master、从库还没同步），误报「未落库」。
//     因此 API 路径完全跳过 MySQL 兜底，仅触发本地 ETL 异步同步。
//
//   - mode=webform：webform 的 200 OK 不可信（之前栽过跟头），保留 MySQL 兜底校验。
func respondEffortSuccess(c *gin.Context, payload gin.H, taskID int64, workDate, work string, consumedF float64) {
	mode, _ := payload["mode"].(string)

	if mode == "api_v1" {
		// 报工会同时影响 effort 明细和 task 的 consumed/status。
		// 这里异步顺手同步两张本地表，避免前端立刻刷新时只能看到旧任务状态。
		go func() {
			etl.SyncEfforts()
			etl.SyncTasks()
		}()
		c.JSON(http.StatusOK, payload)
		return
	}

	// webform 路径：MySQL 是必要兜底
	if ztDB := db.GetZentao(); ztDB != nil {
		parsedDate, dateErr := time.ParseInLocation("2006-01-02", workDate, time.Local)
		if dateErr == nil {
			var rows []source.ZtEffort
			_ = ztDB.Table(source.ZtEffort{}.TableName()).
				Where("(deleted = '0' OR deleted = 0 OR deleted IS NULL)").
				Where("objectType = ? AND objectID = ?", "task", taskID).
				Where("DATE(`date`) = DATE(?)", parsedDate).
				Order("id DESC").
				Limit(20).
				Find(&rows).Error

			var matched *source.ZtEffort
			for i := range rows {
				it := &rows[i]
				if strings.TrimSpace(it.Work) != strings.TrimSpace(work) {
					continue
				}
				if math.Abs(it.Consumed-consumedF) > 1e-6 {
					continue
				}
				matched = it
				break
			}
			if matched == nil {
				c.JSON(http.StatusBadGateway, gin.H{
					"ok":        false,
					"error":     "Webform 已 POST 但禅道 MySQL 中未查到对应记录（webform 的 200 OK 偶发不可信）。",
					"mode":      payload["mode"],
					"result":    payload["result"],
					"api_error": payload["api_error"],
					"hint":      "如能复现请把上方 result 的 final_url 贴给后端，看是否被 ZIN 框架伪装成功了。",
				})
				return
			}
			payload["effort_id"] = matched.ID
		}
	}

	go func() {
		etl.SyncEfforts()
		etl.SyncTasks()
	}()
	c.JSON(http.StatusOK, payload)
}
