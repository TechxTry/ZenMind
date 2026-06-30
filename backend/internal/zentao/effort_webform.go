package zentao

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type CreateTaskEffortInput struct {
	TaskID   int64
	WorkDate string // YYYY-MM-DD (optional)
	Work     string
	Consumed string
	Left     string
}

type CreateEffortResult struct {
	EndpointTried string `json:"endpoint_tried"`
	FinalURL      string `json:"final_url"`
}

// EditTaskEffortInput is used by task-editEffort webform (date/work/consumed/left).
type EditTaskEffortInput struct {
	EffortID int64
	WorkDate string
	Work     string
	Consumed string
	Left     string // optional
}

var (
	errAuthExpired = errors.New("zentao auth expired")
)

func IsAuthExpiredError(err error) bool {
	return errors.Is(err, errAuthExpired)
}

func CreateTaskEffortByWebForm(ctx context.Context, baseURL string, cookies []*http.Cookie, in CreateTaskEffortInput) (*CreateEffortResult, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("base url is empty")
	}
	if in.TaskID <= 0 {
		return nil, fmt.Errorf("task_id is invalid")
	}
	if strings.TrimSpace(in.Work) == "" {
		return nil, fmt.Errorf("work is required")
	}
	if strings.TrimSpace(in.Consumed) == "" || strings.TrimSpace(in.Left) == "" {
		return nil, fmt.Errorf("consumed and left are required")
	}

	client, err := newWebformClient(baseURL, cookies)
	if err != nil {
		return nil, err
	}

	endpoints := []string{
		fmt.Sprintf("%s/task-recordEstimate-%d.html", baseURL, in.TaskID),
		fmt.Sprintf("%s/task-recordestimate-%d.html", baseURL, in.TaskID),
	}

	var lastErr error
	for _, endpoint := range endpoints {
		r, err := tryCreateEffortAtEndpoint(ctx, client, endpoint, in)
		if err == nil {
			return &CreateEffortResult{EndpointTried: endpoint, FinalURL: r}, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("unknown error")
	}
	return nil, lastErr
}

// UpdateTaskEffortByWebForm edits one effort row via task-editEffort (no REST route in stock ZenTao).
func UpdateTaskEffortByWebForm(ctx context.Context, baseURL string, cookies []*http.Cookie, in EditTaskEffortInput) (*CreateEffortResult, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("base url is empty")
	}
	if in.EffortID <= 0 {
		return nil, fmt.Errorf("effort_id is invalid")
	}
	if strings.TrimSpace(in.Consumed) == "" {
		return nil, fmt.Errorf("consumed is required")
	}

	client, err := newWebformClient(baseURL, cookies)
	if err != nil {
		return nil, err
	}

	endpoints := []string{
		fmt.Sprintf("%s/task-editEffort-%d.html", baseURL, in.EffortID),
		fmt.Sprintf("%s/task-editeffort-%d.html", baseURL, in.EffortID),
	}
	var lastErr error
	for _, endpoint := range endpoints {
		final, err := tryEditEffortAtEndpoint(ctx, client, endpoint, in)
		if err == nil {
			return &CreateEffortResult{EndpointTried: endpoint, FinalURL: final}, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("unknown error")
	}
	return nil, lastErr
}

// DeleteTaskEffortByWebForm deletes one effort via task-deleteWorkhour (confirm=yes).
func DeleteTaskEffortByWebForm(ctx context.Context, baseURL string, cookies []*http.Cookie, effortID int64) (*CreateEffortResult, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("base url is empty")
	}
	if effortID <= 0 {
		return nil, fmt.Errorf("effort_id is invalid")
	}

	client, err := newWebformClient(baseURL, cookies)
	if err != nil {
		return nil, err
	}

	endpoints := []string{
		fmt.Sprintf("%s/task-deleteWorkhour-%d-yes.html", baseURL, effortID),
		fmt.Sprintf("%s/task-deleteworkhour-%d-yes.html", baseURL, effortID),
		fmt.Sprintf("%s/task-deleteWorkhour-effortID-%d-confirm-yes.html", baseURL, effortID),
		fmt.Sprintf("%s/task-deleteworkhour-effortID-%d-confirm-yes.html", baseURL, effortID),
		fmt.Sprintf("%s/effort-delete-%d-yes.html", baseURL, effortID),
		fmt.Sprintf("%s/effort-delete-%d-confirm-yes.html", baseURL, effortID),
		fmt.Sprintf("%s/effort-delete-effortID-%d-confirm-yes.html", baseURL, effortID),
	}
	var lastErr error
	for _, endpoint := range endpoints {
		final, err := tryDeleteEffortAtEndpoint(ctx, client, endpoint)
		if err == nil {
			return &CreateEffortResult{EndpointTried: endpoint, FinalURL: final}, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("unknown error")
	}
	return nil, lastErr
}

func newWebformClient(baseURL string, cookies []*http.Cookie) (*http.Client, error) {
	uBase, err := url.Parse(baseURL)
	if err != nil || uBase.Scheme == "" || uBase.Host == "" {
		return nil, fmt.Errorf("invalid base url")
	}
	jar, _ := cookiejar.New(nil)
	for _, ck := range cookies {
		if ck == nil {
			continue
		}
		jar.SetCookies(uBase, []*http.Cookie{ck})
	}
	return &http.Client{Timeout: 20 * time.Second, Jar: jar}, nil
}

func tryEditEffortAtEndpoint(ctx context.Context, client *http.Client, endpoint string, in EditTaskEffortInput) (finalURL string, err error) {
	body, err := fetchWebformPage(ctx, client, endpoint)
	if err != nil {
		return "", err
	}
	tokenName, tokenValue := extractCSRFToken(body)
	form := buildEditEffortForm(in, tokenName, tokenValue)
	return submitForm(ctx, client, endpoint, form)
}

func tryDeleteEffortAtEndpoint(ctx context.Context, client *http.Client, endpoint string) (finalURL string, err error) {
	body, getFinalURL, err := fetchWebformPageWithFinalURL(ctx, client, endpoint)
	if err != nil {
		return "", err
	}
	// Some Zentao versions perform delete on GET and directly redirect away from delete route.
	if !looksLikeDeleteEndpoint(getFinalURL) {
		return getFinalURL, nil
	}
	tokenName, tokenValue := extractCSRFToken(body)
	form := url.Values{}
	if tokenName != "" && tokenValue != "" {
		form.Set(tokenName, tokenValue)
	}
	final, postBody, err := submitFormWithBody(ctx, client, endpoint, form)
	if err != nil {
		return "", err
	}
	// Guard against false positives: if still on delete confirmation page, treat as failure.
	if looksLikeDeleteEndpoint(final) && looksLikeDeleteConfirmationPage(postBody) {
		return "", fmt.Errorf("zentao delete effort stayed on confirmation page")
	}
	return final, nil
}

func fetchWebformPage(ctx context.Context, client *http.Client, endpoint string) (body string, err error) {
	body, _, err = fetchWebformPageWithFinalURL(ctx, client, endpoint)
	return body, err
}

func fetchWebformPageWithFinalURL(ctx context.Context, client *http.Client, endpoint string) (body string, finalURL string, err error) {
	reqGet, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", "", err
	}
	reqGet.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := client.Do(reqGet)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	bodyB, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	body = string(bodyB)
	if looksLikeLoginPage(resp, body) {
		return "", "", errAuthExpired
	}
	if resp.StatusCode >= 400 {
		return "", "", fmt.Errorf("zentao returned http %d", resp.StatusCode)
	}
	return body, resp.Request.URL.String(), nil
}

func buildEditEffortForm(in EditTaskEffortInput, tokenName, tokenValue string) url.Values {
	v := url.Values{}
	if tokenName != "" && tokenValue != "" {
		v.Set(tokenName, tokenValue)
	}
	if strings.TrimSpace(in.WorkDate) != "" {
		v.Set("date", strings.TrimSpace(in.WorkDate))
	}
	if strings.TrimSpace(in.Work) != "" {
		v.Set("work", in.Work)
	}
	v.Set("consumed", strings.TrimSpace(in.Consumed))
	if strings.TrimSpace(in.Left) != "" {
		v.Set("left", strings.TrimSpace(in.Left))
	}
	return v
}

func tryCreateEffortAtEndpoint(ctx context.Context, client *http.Client, endpoint string, in CreateTaskEffortInput) (finalURL string, err error) {
	body, err := fetchWebformPage(ctx, client, endpoint)
	if err != nil {
		return "", err
	}

	tokenName, tokenValue := extractCSRFToken(body)

	formVariants := []url.Values{
		buildEffortFormVariantA(in, tokenName, tokenValue),
		buildEffortFormVariantB(in, tokenName, tokenValue),
	}

	for _, form := range formVariants {
		final, postErr := submitForm(ctx, client, endpoint, form)
		if postErr == nil {
			return final, nil
		}
		if errors.Is(postErr, errAuthExpired) {
			return "", postErr
		}
		err = postErr
	}
	if err == nil {
		err = fmt.Errorf("submit failed")
	}
	return "", err
}

func submitForm(ctx context.Context, client *http.Client, endpoint string, form url.Values) (finalURL string, err error) {
	finalURL, _, err = submitFormWithBody(ctx, client, endpoint, form)
	return finalURL, err
}

func submitFormWithBody(ctx context.Context, client *http.Client, endpoint string, form url.Values) (finalURL, body string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Referer", endpoint)

	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	body = string(b)
	if looksLikeLoginPage(resp, body) {
		return "", "", errAuthExpired
	}
	if resp.StatusCode >= 400 {
		return "", "", fmt.Errorf("zentao returned http %d", resp.StatusCode)
	}

	// Zentao often returns a page with JS message; treat obvious error as failure.
	lb := strings.ToLower(body)
	if strings.Contains(lb, "alert(") && (strings.Contains(body, "失败") || strings.Contains(lb, "fail") || strings.Contains(lb, "error")) {
		return "", "", fmt.Errorf("zentao returned failure alert")
	}
	// Common error containers (Zentao UI / bootstrap)
	if strings.Contains(lb, "alert alert-danger") || strings.Contains(lb, "text-danger") {
		// If it looks like we are still on the recordEstimate form with an error, fail fast.
		if strings.Contains(lb, "recordestimate") || strings.Contains(lb, "record estimate") || strings.Contains(lb, "name=\"consumed\"") {
			return "", "", fmt.Errorf("zentao returned validation error page")
		}
	}
	// CSRF/token mismatch hints
	if strings.Contains(lb, "token") && (strings.Contains(body, "无效") || strings.Contains(body, "错误") || strings.Contains(lb, "invalid")) {
		return "", "", fmt.Errorf("zentao csrf token invalid")
	}
	// Some instances show "login" text without standard form markers; keep this extra heuristic.
	if strings.Contains(body, "登录") && strings.Contains(lb, "password") {
		return "", "", errAuthExpired
	}
	return resp.Request.URL.String(), body, nil
}

func looksLikeDeleteEndpoint(raw string) bool {
	l := strings.ToLower(raw)
	return strings.Contains(l, "task-deleteworkhour") ||
		strings.Contains(l, "task-deletework-hour") ||
		strings.Contains(l, "effort-delete") ||
		strings.Contains(l, "deleteeffort")
}

func looksLikeDeleteConfirmationPage(body string) bool {
	lb := strings.ToLower(body)
	if !strings.Contains(lb, "deleteworkhour") && !strings.Contains(lb, "effort-delete") && !strings.Contains(lb, "delete effort") {
		return false
	}
	return strings.Contains(lb, "confirm=yes") ||
		strings.Contains(lb, "确认删除") ||
		strings.Contains(lb, "确定删除") ||
		strings.Contains(lb, "are you sure")
}

func looksLikeLoginPage(resp *http.Response, body string) bool {
	if resp != nil && resp.Request != nil && resp.Request.URL != nil {
		u := resp.Request.URL.String()
		if strings.Contains(u, "user-login") || strings.Contains(u, "login") {
			return true
		}
	}
	lb := strings.ToLower(body)
	// 经典表单登录页：包含 account + password 字段
	if strings.Contains(lb, "name=\"account\"") && strings.Contains(lb, "name=\"password\"") {
		return true
	}
	// 禅道企业版/旗舰版的 JS 跳转兜底：<script>self.location='/user-login-...'</script>
	if strings.Contains(lb, "self.location") &&
		(strings.Contains(lb, "user-login") || strings.Contains(lb, "/login")) {
		return true
	}
	// 页面很短并且内容只是一段跳转脚本（<script> 出现在前 300 字节里且 body 长度很小）
	if len(body) > 0 && len(body) < 600 && strings.Contains(lb, "<script") && strings.Contains(lb, "location") {
		return true
	}
	return false
}

func extractCSRFToken(html string) (name string, value string) {
	// Common Zentao hidden tokens: token / verifyRand
	re := regexp.MustCompile(`(?is)<input[^>]+type=['"]hidden['"][^>]+name=['"](token|verifyRand)['"][^>]*value=['"]([^'"]+)['"]`)
	m := re.FindStringSubmatch(html)
	if len(m) == 3 {
		return m[1], m[2]
	}
	return "", ""
}

func buildEffortFormVariantA(in CreateTaskEffortInput, tokenName, tokenValue string) url.Values {
	v := url.Values{}
	if tokenName != "" && tokenValue != "" {
		v.Set(tokenName, tokenValue)
	}
	if strings.TrimSpace(in.WorkDate) != "" {
		v.Set("date", strings.TrimSpace(in.WorkDate))
	}
	v.Set("work", in.Work)
	v.Set("consumed", strings.TrimSpace(in.Consumed))
	v.Set("left", strings.TrimSpace(in.Left))
	return v
}

func buildEffortFormVariantB(in CreateTaskEffortInput, tokenName, tokenValue string) url.Values {
	// Some Zentao pages use array-like fields when recording multiple estimates.
	v := url.Values{}
	if tokenName != "" && tokenValue != "" {
		v.Set(tokenName, tokenValue)
	}
	date := strings.TrimSpace(in.WorkDate)
	if date == "" {
		// Leave empty and let Zentao default to today.
		date = ""
	}
	v.Set("dates[0]", date)
	v.Set("work[0]", in.Work)
	v.Set("consumed[0]", strings.TrimSpace(in.Consumed))
	v.Set("left[0]", strings.TrimSpace(in.Left))
	return v
}
