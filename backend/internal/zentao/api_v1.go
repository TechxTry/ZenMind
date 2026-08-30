package zentao

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// errAPIUnauthorized 表示 Token 过期/无效，由调用方负责重新登录换 Token。
var errAPIUnauthorized = errors.New("zentao api unauthorized")

func IsAPIUnauthorizedError(err error) bool {
	return errors.Is(err, errAPIUnauthorized)
}

// APIHTTPError 结构化承载禅道 API 非 2xx 响应，便于上层按 Status 决策（例如 404 才回落 webform）。
type APIHTTPError struct {
	Status int
	URL    string
	Body   string
}

func (e *APIHTTPError) Error() string {
	return fmt.Sprintf("zentao api http %d (%s): %s", e.Status, e.URL, e.Body)
}

func IsAPIHTTPError(err error) (*APIHTTPError, bool) {
	var he *APIHTTPError
	if errors.As(err, &he) {
		return he, true
	}
	return nil, false
}

// APIClient 对应禅道企业版/旗舰版 REST API v1。
type APIClient struct {
	BaseURL string
	HTTP    *http.Client
}

func NewAPIClient(baseURL string) *APIClient {
	return &APIClient{
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		HTTP: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

// APILoginResult 是 /api.php/v1/tokens 的成功响应。
// 禅道常见返回 {"token":"xxx","expire":"7200"} 或 {"token":"xxx"}；部分版本用数字秒数。
type APILoginResult struct {
	Token   string `json:"token"`
	Expire  int64  `json:"expire,omitempty"` // 秒；部分版本缺省
	Account string `json:"account,omitempty"`
}

// APILogin 使用账号密码换 Bearer Token。
func (c *APIClient) APILogin(ctx context.Context, account, password string) (*APILoginResult, error) {
	body := map[string]string{"account": account, "password": password}
	buf, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api.php/v1/tokens", bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, &APIHTTPError{Status: resp.StatusCode, URL: req.URL.String(), Body: snippet(string(raw))}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIHTTPError{Status: resp.StatusCode, URL: req.URL.String(), Body: snippet(string(raw))}
	}

	var out APILoginResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("zentao api login: invalid json: %w; body=%s", err, snippet(string(raw)))
	}
	if strings.TrimSpace(out.Token) == "" {
		return nil, fmt.Errorf("zentao api login: empty token in response: %s", snippet(string(raw)))
	}
	return &out, nil
}

// APIGetMe 用已有 Token 调 /api.php/v1/user 验证 Token 有效且返回账号名。
func (c *APIClient) APIGetMe(ctx context.Context, token string) (account string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api.php/v1/user", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Token", token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode == http.StatusUnauthorized {
		return "", errAPIUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &APIHTTPError{Status: resp.StatusCode, URL: req.URL.String(), Body: snippet(string(raw))}
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", fmt.Errorf("zentao api user: invalid json: %w; body=%s", err, snippet(string(raw)))
	}
	if v, ok := m["account"].(string); ok && v != "" {
		return v, nil
	}
	return "", nil
}

// APICreateTaskEffortInput 与 webform 版共用 CreateTaskEffortInput 结构不便，这里单独定义以对齐 JSON 字段。
type APICreateTaskEffortInput struct {
	TaskID   int64
	WorkDate string // YYYY-MM-DD
	Work     string
	Consumed float64
	Left     float64
}

// APICreateTaskEffortResult 包含 API 成功响应中常见的字段。
type APICreateTaskEffortResult struct {
	ID          int64          `json:"id,omitempty"`
	ObjectID    int64          `json:"objectID,omitempty"`
	ObjectType  string         `json:"objectType,omitempty"`
	RawBody     string         `json:"raw_body,omitempty"`
	Fields      map[string]any `json:"fields,omitempty"`
	UsedURL     string         `json:"used_url,omitempty"`
	UsedVariant string         `json:"used_variant,omitempty"`
	// TaskConsumedAfter：响应是 task 详情快照时，task.consumed 的当前值。弱信号。
	TaskConsumedAfter float64 `json:"task_consumed_after,omitempty"`
	// VerifyAttempted / VerifyMatched：POST 后 GET 任务日志列表做的二次校验结果。
	// 只有 VerifyMatched=true 才能证明 effort 真正插入到 zt_effort 表里。
	VerifyAttempted bool   `json:"verify_attempted,omitempty"`
	VerifyMatched   bool   `json:"verify_matched,omitempty"`
	VerifyError     string `json:"verify_error,omitempty"`
}

// 报工 API 的三种 body 变体。禅道版本之间字段名/路径不同，需要逐个试。
const (
	VariantEstimateModern  = "estimate-modern"  // 开源 ≥20.7 / 企业 ≥10.6 / 旗舰 ≥5.6：date / work / consumed / left（数组）
	VariantEstimateLegacy  = "estimate-legacy"  // 老版本：id / objectID / objectType / dates / work / consumed / left（数组）
	VariantEffortsFallback = "efforts-fallback" // 极少数自定义分支：POST /efforts 单条对象
)

// AllEffortVariants 给 handler 用的默认顺序。
var AllEffortVariants = []string{
	VariantEstimateModern,
	VariantEstimateLegacy,
	VariantEffortsFallback,
}

// ErrAPIEffortNotPersisted：API 返回 200 + task 详情，但 task.consumed 没有按预期增加。
// 表示这个变体的 body 字段被禅道忽略了，handler 应该尝试下一个变体。
var ErrAPIEffortNotPersisted = errors.New("zentao api returned 200 but task.consumed did not increase; effort likely NOT persisted")

// APITaskEffort 是 GET /api.php/v1/tasks/{id}/estimate 列表项。
type APITaskEffort struct {
	ID         int64   `json:"id"`
	ObjectType string  `json:"objectType"`
	ObjectID   int64   `json:"objectID"`
	Account    string  `json:"account"`
	Work       string  `json:"work"`
	Date       string  `json:"date"`
	Consumed   float64 `json:"consumed"`
	Left       float64 `json:"left"`
}

// APIListTaskEfforts 拉取一个任务的所有日志（effort）记录。
// 禅道返回格式有两种：{"effort":{"9":{...},...}} 或 {"efforts":[...]}，都兼容。
func (c *APIClient) APIListTaskEfforts(ctx context.Context, token string, taskID int64) ([]APITaskEffort, error) {
	url := fmt.Sprintf("%s/api.php/v1/tasks/%d/estimate", c.BaseURL, taskID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Token", token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, errAPIUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIHTTPError{Status: resp.StatusCode, URL: url, Body: snippet(string(raw))}
	}

	var wrapper struct {
		Effort  map[string]APITaskEffort `json:"effort"`
		Efforts []APITaskEffort          `json:"efforts"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, fmt.Errorf("zentao api list efforts: invalid json: %w; body=%s", err, snippet(string(raw)))
	}
	out := make([]APITaskEffort, 0, len(wrapper.Effort)+len(wrapper.Efforts))
	for _, v := range wrapper.Effort {
		out = append(out, v)
	}
	out = append(out, wrapper.Efforts...)
	return out, nil
}

// findMatchingEffort 在 effort 列表里找一条与本次提交匹配的：date 前缀相同、work trim 相等、consumed 近似相等。
func findMatchingEffort(efforts []APITaskEffort, in APICreateTaskEffortInput) *APITaskEffort {
	wantWork := strings.TrimSpace(in.Work)
	wantDate := strings.TrimSpace(in.WorkDate)
	for i := range efforts {
		e := &efforts[i]
		dateField := strings.TrimSpace(e.Date)
		// 禅道有时返回 "2026-04-27"、有时 "2026-04-27 00:00:00"，前缀匹配即可
		if !strings.HasPrefix(dateField, wantDate) {
			continue
		}
		if strings.TrimSpace(e.Work) != wantWork {
			continue
		}
		if in.Consumed > 0 && abs64(e.Consumed-in.Consumed) > 1e-3 {
			continue
		}
		return e
	}
	return nil
}

func abs64(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// APICreateTaskEffortByVariant 用指定变体调一次报工 API。
//
// 错误约定：
//   - 401 → errAPIUnauthorized
//   - 非 2xx → APIHTTPError
//   - 2xx 但 (a) 响应里 task.consumed 没增长 或 (b) 二次 GET 列表找不到本次记录 → ErrAPIEffortNotPersisted
//     这种"半生效"必须被识别（个别定制版禅道的 estimate 接口只更新 task 字段、不创建 effort 行）。
//   - 2xx 且二次校验通过 → result, nil（result.ID 会被回填为 effort 的真实 ID）
//
// 二次校验失败处理（GET 拉不到 / 网络抖动）：fail-open，按 task.consumed 判断兜底。
func (c *APIClient) APICreateTaskEffortByVariant(ctx context.Context, token string, in APICreateTaskEffortInput, variant string) (*APICreateTaskEffortResult, error) {
	if in.TaskID <= 0 {
		return nil, fmt.Errorf("invalid task id")
	}
	if strings.TrimSpace(in.Work) == "" {
		return nil, fmt.Errorf("work is required")
	}
	if strings.TrimSpace(in.WorkDate) == "" {
		in.WorkDate = time.Now().Format("2006-01-02")
	}

	url, body, err := c.buildEffortRequest(in, variant)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Token", token)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 128*1024))

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, errAPIUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIHTTPError{Status: resp.StatusCode, URL: url, Body: snippet(string(raw))}
	}

	r := &APICreateTaskEffortResult{
		RawBody:     snippet(string(raw)),
		UsedURL:     url,
		UsedVariant: variant,
	}
	var m map[string]any
	if jErr := json.Unmarshal(raw, &m); jErr == nil {
		r.Fields = m

		// 极少数版本即使 2xx 也通过 {"error":"..."} 表达错误
		if errVal, ok := m["error"].(string); ok && errVal != "" {
			return r, &APIHTTPError{Status: resp.StatusCode, URL: url, Body: snippet(string(raw))}
		}

		if v, ok := m["id"].(float64); ok {
			r.ID = int64(v)
		}
		if v, ok := m["objectID"].(float64); ok {
			r.ObjectID = int64(v)
		}
		if v, ok := m["objectType"].(string); ok {
			r.ObjectType = v
		}

		// 弱信号：响应里 task.consumed 字段，仅当严格小于本次提交值时能直接判定假成功。
		// 不能反过来用 "consumed >= input.Consumed" 判定真成功——
		// 因为某些定制版会更新 task.consumed 但不创建 effort（见下方二次 GET 校验）。
		if in.Consumed > 0 {
			respID, hasID := numFromAny(m["id"])
			respConsumed, hasConsumed := numFromAny(m["consumed"])
			if hasID && hasConsumed && int64(respID) == in.TaskID {
				r.TaskConsumedAfter = respConsumed
				if respConsumed < in.Consumed-1e-6 {
					return r, ErrAPIEffortNotPersisted
				}
			}
		}
	}

	// 强校验：立即 GET 一次任务的日志列表，确认本次记录真的写到 zt_effort 表了。
	// 这是为了识别"半生效假成功"——某些定制版禅道的 POST /tasks/{id}/estimate
	// 只更新 task 表的 consumed/left 字段，但根本不创建 effort 行。
	verifyCtx, verifyCancel := context.WithTimeout(ctx, 8*time.Second)
	defer verifyCancel()
	efforts, listErr := c.APIListTaskEfforts(verifyCtx, token, in.TaskID)
	if listErr == nil {
		matched := findMatchingEffort(efforts, in)
		if matched == nil {
			r.VerifyAttempted = true
			r.VerifyMatched = false
			return r, ErrAPIEffortNotPersisted
		}
		r.VerifyAttempted = true
		r.VerifyMatched = true
		r.ID = matched.ID
		r.ObjectID = matched.ObjectID
		if matched.ObjectType != "" {
			r.ObjectType = matched.ObjectType
		}
		return r, nil
	}
	// GET 失败（404 / 网络）→ fail-open：保持上面 task.consumed 弱判断的结果。
	r.VerifyAttempted = true
	r.VerifyMatched = false
	r.VerifyError = listErr.Error()
	return r, nil
}

// buildEffortRequest 按变体生成 URL 和 body。
func (c *APIClient) buildEffortRequest(in APICreateTaskEffortInput, variant string) (string, []byte, error) {
	estimateURL := fmt.Sprintf("%s/api.php/v1/tasks/%d/estimate", c.BaseURL, in.TaskID)
	effortsURL := fmt.Sprintf("%s/api.php/v1/tasks/%d/efforts", c.BaseURL, in.TaskID)

	var (
		url  string
		body map[string]any
	)
	switch variant {
	case VariantEstimateModern:
		url = estimateURL
		body = map[string]any{
			"date":     []string{in.WorkDate},
			"work":     []string{in.Work},
			"consumed": []float64{in.Consumed},
			"left":     []float64{in.Left},
		}
	case VariantEstimateLegacy:
		url = estimateURL
		body = map[string]any{
			"id":         []int{0},
			"objectID":   []int64{in.TaskID},
			"objectType": []string{"task"},
			"dates":      []string{in.WorkDate},
			"work":       []string{in.Work},
			"consumed":   []float64{in.Consumed},
			"left":       []float64{in.Left},
		}
	case VariantEffortsFallback:
		url = effortsURL
		body = map[string]any{
			"date":     in.WorkDate,
			"work":     in.Work,
			"consumed": in.Consumed,
			"left":     in.Left,
		}
	default:
		return "", nil, fmt.Errorf("unknown effort variant: %s", variant)
	}

	buf, err := json.Marshal(body)
	if err != nil {
		return "", nil, err
	}
	return url, buf, nil
}

// numFromAny 兼容 JSON 数字、字符串数字、json.Number。
func numFromAny(v any) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int8:
		return float64(x), true
	case int16:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint:
		return float64(x), true
	case uint8:
		return float64(x), true
	case uint16:
		return float64(x), true
	case uint32:
		return float64(x), true
	case uint64:
		return float64(x), true
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f, err == nil
	}
	return 0, false
}

// APICreateTask creates a task via API v1.
// Most instances accept POST /api.php/v1/tasks; some custom builds expose
// POST /api.php/v1/executions/{execution}/tasks. We try both when execution is present.
func (c *APIClient) APICreateTask(ctx context.Context, token string, payload map[string]any) (map[string]any, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty payload")
	}
	if strings.TrimSpace(fmt.Sprintf("%v", payload["name"])) == "" {
		return nil, fmt.Errorf("name is required")
	}
	ensureTaskEstStarted(payload)

	urls := make([]string, 0, 3)
	if execID, ok := taskExecutionID(payload); ok && execID > 0 {
		urls = appendCreateTaskURL(urls, fmt.Sprintf("%s/api.php/v1/executions/%d/tasks", c.BaseURL, execID))
	}
	if projectID, ok := taskProjectID(payload); ok && projectID > 0 {
		urls = appendCreateTaskURL(urls, fmt.Sprintf("%s/api.php/v1/projects/%d/tasks", c.BaseURL, projectID))
	}
	urls = appendCreateTaskURL(urls, fmt.Sprintf("%s/api.php/v1/tasks", c.BaseURL))

	var lastErr error
	for _, u := range urls {
		out, err := c.apiJSON(ctx, http.MethodPost, u, token, payload)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if shouldContinueCreateTaskTry(err) {
			continue
		}
		return nil, err
	}
	return nil, lastErr
}

func ensureTaskEstStarted(payload map[string]any) {
	if payload == nil {
		return
	}
	if s, ok := payload["estStarted"].(string); ok && strings.TrimSpace(s) != "" {
		return
	}
	if s, ok := payload["est_started"].(string); ok && strings.TrimSpace(s) != "" {
		payload["estStarted"] = strings.TrimSpace(s)
		return
	}
	payload["estStarted"] = time.Now().Format("2006-01-02")
}

func appendCreateTaskURL(urls []string, u string) []string {
	for _, existing := range urls {
		if existing == u {
			return urls
		}
	}
	return append(urls, u)
}

func taskExecutionID(payload map[string]any) (int64, bool) {
	if v, ok := payload["execution"]; ok {
		if f, ok2 := numFromAny(v); ok2 {
			return int64(f), true
		}
	}
	if v, ok := payload["execution_id"]; ok {
		if f, ok2 := numFromAny(v); ok2 {
			return int64(f), true
		}
	}
	return 0, false
}

func taskProjectID(payload map[string]any) (int64, bool) {
	if v, ok := payload["project"]; ok {
		if f, ok2 := numFromAny(v); ok2 {
			return int64(f), true
		}
	}
	if v, ok := payload["project_id"]; ok {
		if f, ok2 := numFromAny(v); ok2 {
			return int64(f), true
		}
	}
	return 0, false
}

func shouldContinueCreateTaskTry(err error) bool {
	he, ok := IsAPIHTTPError(err)
	if !ok {
		return false
	}
	if he.Status == http.StatusNotFound || he.Status == http.StatusMethodNotAllowed {
		return true
	}
	body := strings.ToLower(he.Body)
	if !strings.Contains(body, "tasksentry::post") {
		return false
	}
	return strings.Contains(body, "too few arguments") ||
		strings.Contains(body, "argumentcounterror") ||
		strings.Contains(body, "missing argument")
}

// APICreateBug creates a bug via API v1.
// Different Zentao builds may expose POST routes under executions/projects/products/bugs.
func (c *APIClient) APICreateBug(ctx context.Context, token string, payload map[string]any) (map[string]any, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty payload")
	}
	if strings.TrimSpace(fmt.Sprintf("%v", payload["title"])) == "" {
		return nil, fmt.Errorf("title is required")
	}

	urls := make([]string, 0, 4)
	if execID, ok := bugExecutionID(payload); ok && execID > 0 {
		urls = appendCreateTaskURL(urls, fmt.Sprintf("%s/api.php/v1/executions/%d/bugs", c.BaseURL, execID))
	}
	if projectID, ok := bugProjectID(payload); ok && projectID > 0 {
		urls = appendCreateTaskURL(urls, fmt.Sprintf("%s/api.php/v1/projects/%d/bugs", c.BaseURL, projectID))
	}
	if productID, ok := bugProductID(payload); ok && productID > 0 {
		urls = appendCreateTaskURL(urls, fmt.Sprintf("%s/api.php/v1/products/%d/bugs", c.BaseURL, productID))
	}
	urls = appendCreateTaskURL(urls, fmt.Sprintf("%s/api.php/v1/bugs", c.BaseURL))

	var lastErr error
	for _, u := range urls {
		out, err := c.apiJSON(ctx, http.MethodPost, u, token, payload)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if shouldContinueCreateBugTry(err) {
			continue
		}
		return nil, err
	}
	return nil, lastErr
}

func bugExecutionID(payload map[string]any) (int64, bool) {
	if v, ok := payload["execution"]; ok {
		if f, ok2 := numFromAny(v); ok2 {
			return int64(f), true
		}
	}
	if v, ok := payload["execution_id"]; ok {
		if f, ok2 := numFromAny(v); ok2 {
			return int64(f), true
		}
	}
	return 0, false
}

func bugProjectID(payload map[string]any) (int64, bool) {
	if v, ok := payload["project"]; ok {
		if f, ok2 := numFromAny(v); ok2 {
			return int64(f), true
		}
	}
	if v, ok := payload["project_id"]; ok {
		if f, ok2 := numFromAny(v); ok2 {
			return int64(f), true
		}
	}
	return 0, false
}

func bugProductID(payload map[string]any) (int64, bool) {
	if v, ok := payload["product"]; ok {
		if f, ok2 := numFromAny(v); ok2 {
			return int64(f), true
		}
	}
	if v, ok := payload["product_id"]; ok {
		if f, ok2 := numFromAny(v); ok2 {
			return int64(f), true
		}
	}
	return 0, false
}

func shouldContinueCreateBugTry(err error) bool {
	he, ok := IsAPIHTTPError(err)
	if !ok {
		return false
	}
	if he.Status == http.StatusNotFound || he.Status == http.StatusMethodNotAllowed {
		return true
	}
	body := strings.ToLower(he.Body)
	if !strings.Contains(body, "bugsentry::post") {
		return false
	}
	return strings.Contains(body, "too few arguments") ||
		strings.Contains(body, "argumentcounterror") ||
		strings.Contains(body, "missing argument")
}

// APICreateStory creates a story via API v1.
// Most builds expect product context, so we try products/{id}/stories first when product is present.
func (c *APIClient) APICreateStory(ctx context.Context, token string, payload map[string]any) (map[string]any, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty payload")
	}
	if strings.TrimSpace(fmt.Sprintf("%v", payload["title"])) == "" {
		return nil, fmt.Errorf("title is required")
	}

	urls := make([]string, 0, 2)
	if productID, ok := storyProductID(payload); ok && productID > 0 {
		urls = appendCreateTaskURL(urls, fmt.Sprintf("%s/api.php/v1/products/%d/stories", c.BaseURL, productID))
	}
	urls = appendCreateTaskURL(urls, fmt.Sprintf("%s/api.php/v1/stories", c.BaseURL))

	var lastErr error
	for _, u := range urls {
		out, err := c.apiJSON(ctx, http.MethodPost, u, token, payload)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if shouldContinueCreateStoryTry(err) {
			continue
		}
		return nil, err
	}
	return nil, lastErr
}

func storyProductID(payload map[string]any) (int64, bool) {
	if v, ok := payload["product"]; ok {
		if f, ok2 := numFromAny(v); ok2 {
			return int64(f), true
		}
	}
	if v, ok := payload["product_id"]; ok {
		if f, ok2 := numFromAny(v); ok2 {
			return int64(f), true
		}
	}
	return 0, false
}

func shouldContinueCreateStoryTry(err error) bool {
	he, ok := IsAPIHTTPError(err)
	if !ok {
		return false
	}
	if he.Status == http.StatusNotFound || he.Status == http.StatusMethodNotAllowed {
		return true
	}
	body := strings.ToLower(he.Body)
	if !strings.Contains(body, "storiesentry::post") {
		return false
	}
	return strings.Contains(body, "too few arguments") ||
		strings.Contains(body, "argumentcounterror") ||
		strings.Contains(body, "missing argument")
}

// APIGetTask fetches one task via API v1.
func (c *APIClient) APIGetTask(ctx context.Context, token string, taskID int64) (map[string]any, error) {
	if taskID <= 0 {
		return nil, fmt.Errorf("invalid task id")
	}
	url := fmt.Sprintf("%s/api.php/v1/tasks/%d", c.BaseURL, taskID)
	return c.apiJSON(ctx, http.MethodGet, url, token, nil)
}

// APIGetBug fetches one bug via API v1.
func (c *APIClient) APIGetBug(ctx context.Context, token string, bugID int64) (map[string]any, error) {
	if bugID <= 0 {
		return nil, fmt.Errorf("invalid bug id")
	}
	url := fmt.Sprintf("%s/api.php/v1/bugs/%d", c.BaseURL, bugID)
	return c.apiJSON(ctx, http.MethodGet, url, token, nil)
}

// APIGetStory fetches one story via API v1.
func (c *APIClient) APIGetStory(ctx context.Context, token string, storyID int64) (map[string]any, error) {
	if storyID <= 0 {
		return nil, fmt.Errorf("invalid story id")
	}
	url := fmt.Sprintf("%s/api.php/v1/stories/%d", c.BaseURL, storyID)
	return c.apiJSON(ctx, http.MethodGet, url, token, nil)
}

// APIGetEffort fetches one effort row via API v1.
func (c *APIClient) APIGetEffort(ctx context.Context, token string, effortID int64) (map[string]any, error) {
	if effortID <= 0 {
		return nil, fmt.Errorf("invalid effort id")
	}
	url := fmt.Sprintf("%s/api.php/v1/efforts/%d", c.BaseURL, effortID)
	return c.apiJSON(ctx, http.MethodGet, url, token, nil)
}

// APIUpdateTask updates task fields via API v1.
// Stock ZenTao exposes taskEntry::put (not patch); entry.class.php only parses JSON body on POST/PUT.
// Partial payloads are merged server-side with the existing task (batchSetPost + task->edit), same as the web edit form.
func (c *APIClient) APIUpdateTask(ctx context.Context, token string, taskID int64, payload map[string]any) (map[string]any, error) {
	if taskID <= 0 {
		return nil, fmt.Errorf("invalid task id")
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty payload")
	}
	url := fmt.Sprintf("%s/api.php/v1/tasks/%d", c.BaseURL, taskID)
	return c.apiUpdateEntity(ctx, url, token, payload)
}

// APIUpdateBug updates bug fields via API v1.
func (c *APIClient) APIUpdateBug(ctx context.Context, token string, bugID int64, payload map[string]any) (map[string]any, error) {
	if bugID <= 0 {
		return nil, fmt.Errorf("invalid bug id")
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty payload")
	}
	url := fmt.Sprintf("%s/api.php/v1/bugs/%d", c.BaseURL, bugID)
	return c.apiUpdateEntity(ctx, url, token, payload)
}

// APIUpdateStory updates story fields via API v1.
func (c *APIClient) APIUpdateStory(ctx context.Context, token string, storyID int64, payload map[string]any) (map[string]any, error) {
	if storyID <= 0 {
		return nil, fmt.Errorf("invalid story id")
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty payload")
	}
	url := fmt.Sprintf("%s/api.php/v1/stories/%d", c.BaseURL, storyID)
	return c.apiUpdateEntity(ctx, url, token, payload)
}

func apiUpdateMethodMissing(status int) bool {
	return status == http.StatusNotFound || status == http.StatusMethodNotAllowed
}

// apiUpdateEntity sends PUT for stock ZenTao API v1 entity updates; PATCH is kept as fallback for custom builds.
func (c *APIClient) apiUpdateEntity(ctx context.Context, url, token string, payload map[string]any) (map[string]any, error) {
	out, err := c.apiJSON(ctx, http.MethodPut, url, token, payload)
	if err == nil {
		return out, nil
	}
	if he, ok := IsAPIHTTPError(err); ok && apiUpdateMethodMissing(he.Status) {
		return c.apiJSON(ctx, http.MethodPatch, url, token, payload)
	}
	return nil, err
}

// APIDeleteBug deletes a bug record via API v1.
func (c *APIClient) APIDeleteBug(ctx context.Context, token string, bugID int64) error {
	if bugID <= 0 {
		return fmt.Errorf("invalid bug id")
	}
	url := fmt.Sprintf("%s/api.php/v1/bugs/%d", c.BaseURL, bugID)
	_, err := c.apiJSON(ctx, http.MethodDelete, url, token, nil)
	return err
}

// APIDeleteStory deletes a story record via API v1.
func (c *APIClient) APIDeleteStory(ctx context.Context, token string, storyID int64) error {
	if storyID <= 0 {
		return fmt.Errorf("invalid story id")
	}
	url := fmt.Sprintf("%s/api.php/v1/stories/%d", c.BaseURL, storyID)
	_, err := c.apiJSON(ctx, http.MethodDelete, url, token, nil)
	return err
}

// ExtractTaskAssignedTo pulls assigned-to account from common task response layouts.
func ExtractTaskAssignedTo(resp map[string]any) string {
	task := extractTaskMap(resp)
	if task == nil {
		return ""
	}
	return firstNonEmptyString(task, "assignedTo", "assigned_to")
}

// ExtractTaskStatus returns normalized task status from task response.
func ExtractTaskStatus(resp map[string]any) string {
	task := extractTaskMap(resp)
	if task == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(firstNonEmptyString(task, "status")))
}

// ExtractTaskDeadline returns YYYY-MM-DD from task response deadline fields.
func ExtractTaskDeadline(resp map[string]any) string {
	task := extractTaskMap(resp)
	if task == nil {
		return ""
	}
	return NormalizeDateYMD(firstNonEmptyString(task, "deadline", "deadline_date", "deadlineDate"))
}

// ExtractTaskPri returns task priority if present.
func ExtractTaskPri(resp map[string]any) (int, bool) {
	task := extractTaskMap(resp)
	if task == nil {
		return 0, false
	}
	for _, key := range []string{"pri", "priority"} {
		if v, ok := task[key]; ok {
			if f, ok2 := numFromAny(v); ok2 {
				return int(f), true
			}
		}
	}
	return 0, false
}

// ExtractTaskString returns the first non-empty string field from a task response.
func ExtractTaskString(resp map[string]any, keys ...string) string {
	task := extractTaskMap(resp)
	if task == nil {
		return ""
	}
	for _, key := range keys {
		if v, ok := task[key]; ok {
			if s := scalarToTrimmedString(v); s != "" {
				return s
			}
		}
	}
	return ""
}

// ExtractTaskFloat returns a numeric task field.
func ExtractTaskFloat(resp map[string]any, keys ...string) (float64, bool) {
	task := extractTaskMap(resp)
	if task == nil {
		return 0, false
	}
	for _, key := range keys {
		if v, ok := task[key]; ok {
			if f, ok2 := numFromAny(v); ok2 {
				return f, true
			}
		}
	}
	return 0, false
}

// ExtractTaskInt64 returns an integer task field.
func ExtractTaskInt64(resp map[string]any, keys ...string) (int64, bool) {
	f, ok := ExtractTaskFloat(resp, keys...)
	if !ok {
		return 0, false
	}
	return int64(f), true
}

// ExtractTaskRaw returns a raw task field value if present.
func ExtractTaskRaw(resp map[string]any, keys ...string) (any, bool) {
	task := extractTaskMap(resp)
	if task == nil {
		return nil, false
	}
	for _, key := range keys {
		if v, ok := task[key]; ok && v != nil {
			return v, true
		}
	}
	return nil, false
}

func scalarToTrimmedString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64, float32, int, int64, int32, json.Number, bool:
		return strings.TrimSpace(fmt.Sprintf("%v", t))
	case map[string]any:
		// assignedTo-like objects
		if s, ok := t["account"].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
		if s, ok := t["realname"].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// ExtractBugAssignedTo pulls bug assignee account from common bug response layouts.
func ExtractBugAssignedTo(resp map[string]any) string {
	bug := extractBugMap(resp)
	if bug == nil {
		return ""
	}
	return firstNonEmptyString(bug, "assignedTo", "assigned_to")
}

// ExtractBugTitle returns bug title.
func ExtractBugTitle(resp map[string]any) string {
	bug := extractBugMap(resp)
	if bug == nil {
		return ""
	}
	return strings.TrimSpace(firstNonEmptyString(bug, "title"))
}

// ExtractBugStatus returns normalized bug status from bug response.
func ExtractBugStatus(resp map[string]any) string {
	bug := extractBugMap(resp)
	if bug == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(firstNonEmptyString(bug, "status")))
}

// ExtractBugExecutionID returns execution id if present.
func ExtractBugExecutionID(resp map[string]any) (int64, bool) {
	bug := extractBugMap(resp)
	if bug == nil {
		return 0, false
	}
	for _, key := range []string{"execution", "executionID", "execution_id", "project"} {
		if v, ok := bug[key]; ok {
			if f, ok2 := numFromAny(v); ok2 {
				return int64(f), true
			}
		}
	}
	return 0, false
}

// ExtractBugStoryID returns linked story id if present.
func ExtractBugStoryID(resp map[string]any) (int64, bool) {
	bug := extractBugMap(resp)
	if bug == nil {
		return 0, false
	}
	for _, key := range []string{"story", "storyID", "story_id"} {
		if v, ok := bug[key]; ok {
			if f, ok2 := numFromAny(v); ok2 {
				return int64(f), true
			}
		}
	}
	return 0, false
}

// ExtractBugTaskID returns linked task id if present.
func ExtractBugTaskID(resp map[string]any) (int64, bool) {
	bug := extractBugMap(resp)
	if bug == nil {
		return 0, false
	}
	for _, key := range []string{"task", "taskID", "task_id"} {
		if v, ok := bug[key]; ok {
			if f, ok2 := numFromAny(v); ok2 {
				return int64(f), true
			}
		}
	}
	return 0, false
}

// ExtractBugResolution returns normalized bug resolution code.
func ExtractBugResolution(resp map[string]any) string {
	bug := extractBugMap(resp)
	if bug == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(firstNonEmptyString(bug, "resolution")))
}

// ExtractBugSeverity returns bug severity if present.
func ExtractBugSeverity(resp map[string]any) (int, bool) {
	bug := extractBugMap(resp)
	if bug == nil {
		return 0, false
	}
	for _, key := range []string{"severity"} {
		if v, ok := bug[key]; ok {
			if f, ok2 := numFromAny(v); ok2 {
				return int(f), true
			}
		}
	}
	return 0, false
}

// ExtractBugDeleted returns whether the bug has been marked deleted.
func ExtractBugDeleted(resp map[string]any) bool {
	bug := extractBugMap(resp)
	if bug == nil {
		return false
	}
	for _, key := range []string{"deleted", "is_deleted"} {
		if v, ok := bug[key]; ok {
			if b, ok2 := boolFromAny(v); ok2 {
				return b
			}
		}
	}
	return false
}

// ExtractStoryTitle returns story title.
func ExtractStoryTitle(resp map[string]any) string {
	story := extractStoryMap(resp)
	if story == nil {
		return ""
	}
	return strings.TrimSpace(firstNonEmptyString(story, "title"))
}

// ExtractStoryAssignedTo pulls story assignee account from common story response layouts.
func ExtractStoryAssignedTo(resp map[string]any) string {
	story := extractStoryMap(resp)
	if story == nil {
		return ""
	}
	return firstNonEmptyString(story, "assignedTo", "assigned_to")
}

// ExtractStoryStatus returns normalized story status from story response.
func ExtractStoryStatus(resp map[string]any) string {
	story := extractStoryMap(resp)
	if story == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(firstNonEmptyString(story, "status")))
}

// ExtractStoryProductID returns linked product id if present.
func ExtractStoryProductID(resp map[string]any) (int64, bool) {
	story := extractStoryMap(resp)
	if story == nil {
		return 0, false
	}
	for _, key := range []string{"product", "productID", "product_id"} {
		if v, ok := story[key]; ok {
			if f, ok2 := numFromAny(v); ok2 {
				return int64(f), true
			}
		}
	}
	return 0, false
}

// ExtractStoryEstimate returns story estimate if present.
func ExtractStoryEstimate(resp map[string]any) (float64, bool) {
	story := extractStoryMap(resp)
	if story == nil {
		return 0, false
	}
	for _, key := range []string{"estimate"} {
		if v, ok := story[key]; ok {
			if f, ok2 := numFromAny(v); ok2 {
				return f, true
			}
		}
	}
	return 0, false
}

// ParsePlanIDs extracts product-plan IDs from ZenTao plan field variants:
// number, "1,2,3", {id:1}, [{id:1}, ...].
func ParsePlanIDs(v any) []int64 {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case string:
		raw := strings.TrimSpace(x)
		if raw == "" || raw == "0" {
			return nil
		}
		parts := strings.Split(raw, ",")
		out := make([]int64, 0, len(parts))
		for _, part := range parts {
			if f, ok := numFromAny(strings.TrimSpace(part)); ok {
				id := int64(f)
				if id > 0 {
					out = append(out, id)
				}
			}
		}
		return out
	case []any:
		out := make([]int64, 0, len(x))
		for _, item := range x {
			out = append(out, ParsePlanIDs(item)...)
		}
		return out
	case map[string]any:
		for _, key := range []string{"id", "plan", "planID", "planId", "plan_id"} {
			if item, ok := x[key]; ok {
				if ids := ParsePlanIDs(item); len(ids) > 0 {
					return ids
				}
			}
		}
		return nil
	default:
		if f, ok := numFromAny(x); ok {
			id := int64(f)
			if id > 0 {
				return []int64{id}
			}
		}
		return nil
	}
}

// ContainsPlanID reports whether want appears in ids.
func ContainsPlanID(ids []int64, want int64) bool {
	if want <= 0 {
		return false
	}
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func extractPlanIDsFromMap(m map[string]any) []int64 {
	if m == nil {
		return nil
	}
	for _, key := range []string{"plan", "planID", "planId", "plan_id"} {
		if v, ok := m[key]; ok {
			if ids := ParsePlanIDs(v); len(ids) > 0 {
				return ids
			}
		}
	}
	return nil
}

// ExtractStoryPlanIDs returns product plan IDs linked to the story.
func ExtractStoryPlanIDs(resp map[string]any) []int64 {
	return extractPlanIDsFromMap(extractStoryMap(resp))
}

// ExtractStoryPlanID returns the first product plan ID if present.
func ExtractStoryPlanID(resp map[string]any) (int64, bool) {
	ids := ExtractStoryPlanIDs(resp)
	if len(ids) == 0 {
		return 0, false
	}
	return ids[0], true
}

// ExtractBugPlanIDs returns product plan IDs linked to the bug.
func ExtractBugPlanIDs(resp map[string]any) []int64 {
	return extractPlanIDsFromMap(extractBugMap(resp))
}

// ExtractBugPlanID returns the product plan ID if present.
func ExtractBugPlanID(resp map[string]any) (int64, bool) {
	ids := ExtractBugPlanIDs(resp)
	if len(ids) == 0 {
		return 0, false
	}
	return ids[0], true
}

// ExtractStoryDeleted returns whether the story has been marked deleted.
func ExtractStoryDeleted(resp map[string]any) bool {
	story := extractStoryMap(resp)
	if story == nil {
		return false
	}
	for _, key := range []string{"deleted", "is_deleted"} {
		if v, ok := story[key]; ok {
			if b, ok2 := boolFromAny(v); ok2 {
				return b
			}
		}
	}
	return false
}

// ExtractEffortTaskID returns task/object id from effort response.
func ExtractEffortTaskID(resp map[string]any) (int64, bool) {
	eff := extractEffortMap(resp)
	if eff == nil {
		return 0, false
	}
	for _, key := range []string{"objectID", "object_id", "task", "task_id"} {
		if v, ok := eff[key]; ok {
			if f, ok2 := numFromAny(v); ok2 {
				return int64(f), true
			}
		}
	}
	return 0, false
}

// ExtractEffortWork returns normalized effort work text from effort response.
func ExtractEffortWork(resp map[string]any) string {
	eff := extractEffortMap(resp)
	if eff == nil {
		return ""
	}
	return strings.TrimSpace(firstNonEmptyString(eff, "work"))
}

// ExtractEffortDate returns YYYY-MM-DD from effort response date fields.
func ExtractEffortDate(resp map[string]any) string {
	eff := extractEffortMap(resp)
	if eff == nil {
		return ""
	}
	return NormalizeDateYMD(firstNonEmptyString(eff, "date", "work_date", "workDate"))
}

// ExtractEffortConsumed returns consumed hours from effort response.
func ExtractEffortConsumed(resp map[string]any) (float64, bool) {
	eff := extractEffortMap(resp)
	if eff == nil {
		return 0, false
	}
	for _, key := range []string{"consumed"} {
		if v, ok := eff[key]; ok {
			if f, ok2 := numFromAny(v); ok2 {
				return f, true
			}
		}
	}
	return 0, false
}

// ExtractEffortLeft returns left hours from effort response.
func ExtractEffortLeft(resp map[string]any) (float64, bool) {
	eff := extractEffortMap(resp)
	if eff == nil {
		return 0, false
	}
	for _, key := range []string{"left"} {
		if v, ok := eff[key]; ok {
			if f, ok2 := numFromAny(v); ok2 {
				return f, true
			}
		}
	}
	return 0, false
}

// NormalizeDateYMD trims and normalizes date-like strings to YYYY-MM-DD.
func NormalizeDateYMD(raw string) string {
	s := strings.TrimSpace(raw)
	if len(s) >= 10 {
		prefix := s[:10]
		if _, err := time.Parse("2006-01-02", prefix); err == nil {
			return prefix
		}
	}
	if _, err := time.Parse("2006-01-02", s); err == nil {
		return s
	}
	return ""
}

func firstNonEmptyString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key].(string); ok {
			if s := strings.TrimSpace(v); s != "" {
				return s
			}
		}
	}
	return ""
}

func extractTaskMap(resp map[string]any) map[string]any {
	if resp == nil {
		return nil
	}
	if looksLikeTaskMap(resp) {
		return resp
	}
	if m, ok := resp["task"].(map[string]any); ok {
		return m
	}
	if m, ok := resp["data"].(map[string]any); ok {
		if looksLikeTaskMap(m) {
			return m
		}
		if mm, ok2 := m["task"].(map[string]any); ok2 {
			return mm
		}
	}
	return resp
}

func extractEffortMap(resp map[string]any) map[string]any {
	if resp == nil {
		return nil
	}
	if looksLikeEffortMap(resp) {
		return resp
	}
	if m, ok := resp["effort"].(map[string]any); ok {
		return m
	}
	if m, ok := resp["data"].(map[string]any); ok {
		if looksLikeEffortMap(m) {
			return m
		}
		if mm, ok2 := m["effort"].(map[string]any); ok2 {
			return mm
		}
	}
	return resp
}

func extractBugMap(resp map[string]any) map[string]any {
	if resp == nil {
		return nil
	}
	if looksLikeBugMap(resp) {
		return resp
	}
	if m, ok := resp["bug"].(map[string]any); ok {
		return m
	}
	if m, ok := resp["data"].(map[string]any); ok {
		if looksLikeBugMap(m) {
			return m
		}
		if mm, ok2 := m["bug"].(map[string]any); ok2 {
			return mm
		}
	}
	return resp
}

func extractStoryMap(resp map[string]any) map[string]any {
	if resp == nil {
		return nil
	}
	if looksLikeStoryMap(resp) {
		return resp
	}
	if m, ok := resp["story"].(map[string]any); ok {
		return m
	}
	if m, ok := resp["data"].(map[string]any); ok {
		if looksLikeStoryMap(m) {
			return m
		}
		if mm, ok2 := m["story"].(map[string]any); ok2 {
			return mm
		}
	}
	return resp
}

func looksLikeTaskMap(m map[string]any) bool {
	if m == nil {
		return false
	}
	_, hasName := m["name"]
	_, hasStatus := m["status"]
	_, hasAssigned := m["assignedTo"]
	_, hasAssigned2 := m["assigned_to"]
	return hasName || hasStatus || hasAssigned || hasAssigned2
}

func looksLikeBugMap(m map[string]any) bool {
	if m == nil {
		return false
	}
	_, hasTitle := m["title"]
	_, hasStatus := m["status"]
	_, hasAssigned := m["assignedTo"]
	_, hasAssigned2 := m["assigned_to"]
	_, hasSeverity := m["severity"]
	return hasTitle || hasStatus || hasAssigned || hasAssigned2 || hasSeverity
}

func looksLikeStoryMap(m map[string]any) bool {
	if m == nil {
		return false
	}
	_, hasTitle := m["title"]
	_, hasStatus := m["status"]
	_, hasAssigned := m["assignedTo"]
	_, hasAssigned2 := m["assigned_to"]
	_, hasProduct := m["product"]
	return hasTitle || hasStatus || hasAssigned || hasAssigned2 || hasProduct
}

func looksLikeEffortMap(m map[string]any) bool {
	if m == nil {
		return false
	}
	_, hasWork := m["work"]
	_, hasConsumed := m["consumed"]
	_, hasObject := m["objectID"]
	_, hasObject2 := m["object_id"]
	return hasWork || hasConsumed || hasObject || hasObject2
}

func boolFromAny(v any) (bool, bool) {
	switch x := v.(type) {
	case bool:
		return x, true
	case int:
		return x != 0, true
	case int64:
		return x != 0, true
	case float64:
		return x != 0, true
	case json.Number:
		i, err := x.Int64()
		if err == nil {
			return i != 0, true
		}
	case string:
		s := strings.ToLower(strings.TrimSpace(x))
		switch s {
		case "1", "true", "yes", "y":
			return true, true
		case "0", "false", "no", "n", "":
			return false, true
		}
	}
	return false, false
}

// APIUpdateEffort updates effort fields via API v1.
// Official routes only expose POST/GET on /tasks/{taskID}/estimate; some builds add
// /tasks/{taskID}/estimate/{effortID}. /efforts/{id} is not in stock apiv1.php.
func (c *APIClient) APIUpdateEffort(ctx context.Context, token string, taskID, effortID int64, payload map[string]any) (map[string]any, error) {
	if effortID <= 0 {
		return nil, fmt.Errorf("invalid effort id")
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty payload")
	}
	var lastErr error
	if taskID > 0 {
		for _, method := range []string{http.MethodPut, http.MethodPatch} {
			u := fmt.Sprintf("%s/api.php/v1/tasks/%d/estimate/%d", c.BaseURL, taskID, effortID)
			out, err := c.apiJSON(ctx, method, u, token, payload)
			if err == nil {
				return out, nil
			}
			lastErr = err
			if he, ok := IsAPIHTTPError(err); ok && isEffortAPIMissing(he.Status) {
				continue
			}
			return nil, err
		}
	}
	u := fmt.Sprintf("%s/api.php/v1/efforts/%d", c.BaseURL, effortID)
	out, err := c.apiJSON(ctx, http.MethodPatch, u, token, payload)
	if err == nil {
		return out, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, err
}

// APIDeleteEffort deletes an effort record via API v1 (best-effort URL variants).
func (c *APIClient) APIDeleteEffort(ctx context.Context, token string, taskID, effortID int64) error {
	if effortID <= 0 {
		return fmt.Errorf("invalid effort id")
	}
	var lastErr error
	if taskID > 0 {
		for _, method := range []string{http.MethodDelete, http.MethodPost} {
			u := fmt.Sprintf("%s/api.php/v1/tasks/%d/estimate/%d", c.BaseURL, taskID, effortID)
			_, err := c.apiJSON(ctx, method, u, token, nil)
			if err == nil {
				return nil
			}
			lastErr = err
			if he, ok := IsAPIHTTPError(err); ok && isEffortAPIMissing(he.Status) {
				continue
			}
			return err
		}
	}
	u := fmt.Sprintf("%s/api.php/v1/efforts/%d", c.BaseURL, effortID)
	_, err := c.apiJSON(ctx, http.MethodDelete, u, token, nil)
	if err == nil {
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return err
}

func isEffortAPIMissing(status int) bool {
	return status == http.StatusNotFound || status == http.StatusMethodNotAllowed
}

// IsEffortAPIMissing reports whether an API error indicates the write endpoint is absent.
func IsEffortAPIMissing(err error) bool {
	if he, ok := IsAPIHTTPError(err); ok {
		return isEffortAPIMissing(he.Status)
	}
	return false
}

func isBugAPIMissing(status int) bool {
	return status == http.StatusNotFound || status == http.StatusMethodNotAllowed
}

// IsBugAPIMissing reports whether an API error indicates the bug write endpoint is absent.
func IsBugAPIMissing(err error) bool {
	if he, ok := IsAPIHTTPError(err); ok {
		return isBugAPIMissing(he.Status)
	}
	return false
}

func isStoryAPIMissing(status int) bool {
	return status == http.StatusNotFound || status == http.StatusMethodNotAllowed
}

// IsStoryAPIMissing reports whether an API error indicates the story write endpoint is absent.
func IsStoryAPIMissing(err error) bool {
	if he, ok := IsAPIHTTPError(err); ok {
		return isStoryAPIMissing(he.Status)
	}
	return false
}

func (c *APIClient) apiJSON(ctx context.Context, method, url, token string, payload map[string]any) (map[string]any, error) {
	var body io.Reader
	if payload != nil {
		buf, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Token", token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, errAPIUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIHTTPError{Status: resp.StatusCode, URL: url, Body: snippet(string(raw))}
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{"ok": true}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{"raw": snippet(string(raw))}, nil
	}
	if errVal, ok := out["error"].(string); ok && strings.TrimSpace(errVal) != "" {
		return nil, &APIHTTPError{Status: resp.StatusCode, URL: url, Body: snippet(string(raw))}
	}
	return out, nil
}

func snippet(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 400 {
		return s[:400] + "…"
	}
	return s
}
