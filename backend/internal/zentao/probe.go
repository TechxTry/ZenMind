package zentao

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ProbeEndpointResult 记录单个 URL 探测结果，供前端诊断页面展示与后续代码对齐真实字段。
type ProbeEndpointResult struct {
	Label       string   `json:"label"`
	Method      string   `json:"method,omitempty"`
	URL         string   `json:"url"`
	Status      int      `json:"status"`
	FinalURL    string   `json:"final_url"`
	Redirected  bool     `json:"redirected"`
	IsLoginPage bool     `json:"is_login_page"`
	ContentLen  int      `json:"content_len"`
	ContentType string   `json:"content_type,omitempty"`
	FormAction  string   `json:"form_action,omitempty"`
	FormMethod  string   `json:"form_method,omitempty"`
	FoundFields []string `json:"found_fields,omitempty"`
	CSRFName    string   `json:"csrf_name,omitempty"`
	CSRFPresent bool     `json:"csrf_present"`
	ZinDetected bool     `json:"zin_detected,omitempty"`
	BodySnippet string   `json:"body_snippet,omitempty"`
	Error       string   `json:"error,omitempty"`
}

// ProbeResult 是一次诊断的完整报告。
type ProbeResult struct {
	BaseURL           string                `json:"base_url"`
	UsedTaskID        int64                 `json:"used_task_id"`
	SessionValid      bool                  `json:"session_valid"`
	SessionCheck      []ProbeEndpointResult `json:"session_check"`
	EffortEndpoints   []ProbeEndpointResult `json:"effort_endpoints"`
	APIEndpoints      []ProbeEndpointResult `json:"api_endpoints"`
	APILogin          *APILoginProbe        `json:"api_login,omitempty"`
	RecommendedURL    string                `json:"recommended_url,omitempty"`
	RecommendedFields []string              `json:"recommended_fields,omitempty"`
	RecommendedCSRF   string                `json:"recommended_csrf,omitempty"`
	RecommendedMode   string                `json:"recommended_mode,omitempty"` // "webform" | "api_v1"
	Notes             []string              `json:"notes,omitempty"`
}

// APILoginProbe 描述「用已保存的账号密码调 /api.php/v1/tokens」的实测结果。
// 绝不返回 Token 明文，只返回长度与是否成功。
type APILoginProbe struct {
	Attempted     bool   `json:"attempted"`
	OK            bool   `json:"ok"`
	TokenLength   int    `json:"token_length,omitempty"`
	TokenPreview  string `json:"token_preview,omitempty"` // 首尾各 3 字符
	ExpireSeconds int64  `json:"expire_seconds,omitempty"`
	Account       string `json:"account,omitempty"`
	Error         string `json:"error,omitempty"`
}

// ProbeOptions 控制探测行为。Probe 本身默认只读；若提供账号密码，则额外尝试调用
// POST /api.php/v1/tokens 实测换 Token（该调用对禅道也是只读 / 只创建临时 Token 会话）。
type ProbeOptions struct {
	APILoginAccount  string
	APILoginPassword string
}

// Probe 使用已绑定的 cookie，探测禅道实例的真实报工 URL / 表单字段 / CSRF 命名。
// 不做任何写入，纯 GET，永远不会产生数据副作用。
func Probe(ctx context.Context, baseURL string, cookies []*http.Cookie, taskID int64) (*ProbeResult, error) {
	return ProbeWithOptions(ctx, baseURL, cookies, taskID, ProbeOptions{})
}

// ProbeWithOptions 是 Probe 的扩展版本，当提供账号密码时会额外探测 API Token 换取。
func ProbeWithOptions(ctx context.Context, baseURL string, cookies []*http.Cookie, taskID int64, opt ProbeOptions) (*ProbeResult, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("base url is empty")
	}
	uBase, err := url.Parse(baseURL)
	if err != nil || uBase.Scheme == "" || uBase.Host == "" {
		return nil, fmt.Errorf("invalid base url")
	}
	if taskID <= 0 {
		taskID = 1
	}

	jar, _ := cookiejar.New(nil)
	for _, ck := range cookies {
		if ck == nil {
			continue
		}
		jar.SetCookies(uBase, []*http.Cookie{ck})
	}
	client := &http.Client{
		Timeout: 12 * time.Second,
		Jar:     jar,
	}

	result := &ProbeResult{
		BaseURL:    baseURL,
		UsedTaskID: taskID,
	}

	sessionURLs := []struct {
		label string
		url   string
	}{
		{"my", baseURL + "/my/"},
		{"user-view-myself", baseURL + "/user-view-myself.html"},
		{"index", baseURL + "/index.html"},
	}
	for _, s := range sessionURLs {
		r := doProbeGet(ctx, client, s.label, s.url)
		result.SessionCheck = append(result.SessionCheck, r)
		if r.Status >= 200 && r.Status < 400 && !r.IsLoginPage {
			result.SessionValid = true
		}
	}

	if !result.SessionValid {
		result.Notes = append(result.Notes,
			"会话检测未通过：所有页面都被跳转到登录页或返回非 2xx/3xx。请回到「禅道授权」重新保存一次再诊断。")
	}

	effortURLs := []struct {
		label string
		url   string
	}{
		{"task-recordEstimate (legacy, ≤12)", fmt.Sprintf("%s/task-recordEstimate-%d.html", baseURL, taskID)},
		{"task-recordestimate (lowercase)", fmt.Sprintf("%s/task-recordestimate-%d.html", baseURL, taskID)},
		{"effort-createForObject (15+ 单次)", fmt.Sprintf("%s/effort-createForObject-task-%d.html", baseURL, taskID)},
		{"effort-batchCreate (15+ 批量)", fmt.Sprintf("%s/effort-batchCreate-task-%d.html", baseURL, taskID)},
		{"my/effort-batchCreate (15+ 入口)", fmt.Sprintf("%s/my/effort-batchCreate.html", baseURL)},
	}
	for _, e := range effortURLs {
		r := doProbeGet(ctx, client, e.label, e.url)
		result.EffortEndpoints = append(result.EffortEndpoints, r)
	}

	// 追加 REST API 探测（Zentao Biz/Pro/Max 15+ 常见路径）
	apiURLs := []struct {
		label  string
		method string
		url    string
	}{
		{"api.php/v1/tokens (探测 API 是否存在)", "OPTIONS", baseURL + "/api.php/v1/tokens"},
		{"api.php/v1/user (当前用户身份)", "GET", baseURL + "/api.php/v1/user"},
		{fmt.Sprintf("api.php/v1/tasks/%d (任务详情)", taskID), "GET", fmt.Sprintf("%s/api.php/v1/tasks/%d", baseURL, taskID)},
		{fmt.Sprintf("api.php/v1/tasks/%d/estimate (报工资源·官方文档)", taskID), "GET", fmt.Sprintf("%s/api.php/v1/tasks/%d/estimate", baseURL, taskID)},
		{fmt.Sprintf("api.php/v1/tasks/%d/efforts (报工资源·历史路径)", taskID), "GET", fmt.Sprintf("%s/api.php/v1/tasks/%d/efforts", baseURL, taskID)},
	}
	for _, a := range apiURLs {
		r := doProbeWithMethod(ctx, client, a.label, a.method, a.url)
		result.APIEndpoints = append(result.APIEndpoints, r)
	}

	// 若提供了账号密码，实测 POST /api.php/v1/tokens 以验证 API 可实际用于登录
	if strings.TrimSpace(opt.APILoginAccount) != "" && strings.TrimSpace(opt.APILoginPassword) != "" {
		p := probeAPILogin(ctx, baseURL, opt.APILoginAccount, opt.APILoginPassword)
		result.APILogin = p
	}

	// 推荐策略：优先 webform 抓到字段 → 其次 API v1（出现 JSON）→ 否则无推荐
	if best := pickBestEffortEndpoint(result.EffortEndpoints); best != nil {
		if best.FormAction != "" {
			result.RecommendedURL = resolveRelative(baseURL, best.FormAction)
		} else {
			result.RecommendedURL = best.URL
		}
		result.RecommendedFields = best.FoundFields
		result.RecommendedCSRF = best.CSRFName
		result.RecommendedMode = "webform"
		result.Notes = append(result.Notes, fmt.Sprintf("推荐使用 %q 作为提交 URL（webform 模式）。", best.Label))
	} else if apiBest := pickBestAPI(result.APIEndpoints); apiBest != nil {
		result.RecommendedURL = fmt.Sprintf("%s/api.php/v1/tasks/%d/estimate", baseURL, taskID)
		result.RecommendedMode = "api_v1"
		result.Notes = append(result.Notes,
			"检测到禅道 REST API v1 可用（企业版/旗舰版 15+）。推荐改走 API：先 POST /api.php/v1/tokens 换 Token，再 POST /api.php/v1/tasks/{id}/estimate（注意：禅道把这个资源命名为 estimate，不是 efforts）。")
	} else {
		zinSeen := false
		for _, it := range result.EffortEndpoints {
			if it.ZinDetected {
				zinSeen = true
				break
			}
		}
		if zinSeen {
			result.Notes = append(result.Notes,
				"检测到 ZIN 框架（禅道 15+ 旗舰版/Biz 的新 UI），HTML 不包含真实表单字段。请手动抓一次报工的 POST 请求（DevTools → Network）贴给后端，或让后端改走 REST API。")
		} else {
			result.Notes = append(result.Notes,
				"未在候选 URL 中发现可用的报工表单，也未检测到可用的 REST API。请手动抓一次报工的 POST 请求贴给后端。")
		}
	}

	return result, nil
}

func doProbeGet(ctx context.Context, client *http.Client, label, target string) ProbeEndpointResult {
	return doProbeWithMethod(ctx, client, label, http.MethodGet, target)
}

func doProbeWithMethod(ctx context.Context, client *http.Client, label, method, target string) ProbeEndpointResult {
	r := ProbeEndpointResult{Label: label, Method: method, URL: target}
	req, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	req.Header.Set("Accept", "application/json,text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("User-Agent", "Mozilla/5.0 ZenMind-Probe")

	resp, err := client.Do(req)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	defer resp.Body.Close()

	r.Status = resp.StatusCode
	r.ContentType = resp.Header.Get("Content-Type")
	if resp.Request != nil && resp.Request.URL != nil {
		r.FinalURL = resp.Request.URL.String()
		r.Redirected = r.FinalURL != target
	}

	bodyB, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	body := string(bodyB)
	r.ContentLen = len(bodyB)
	r.IsLoginPage = looksLikeLoginPage(resp, body)

	if r.Status >= 200 && r.Status < 400 && !r.IsLoginPage {
		action, fmethod := extractFormInfo(body)
		r.FormAction = action
		r.FormMethod = fmethod
		r.FoundFields = extractFormFieldNames(body)
		name, _ := extractCSRFToken(body)
		r.CSRFName = name
		r.CSRFPresent = name != ""
		r.ZinDetected = detectZinFramework(body)
	}

	// 保留 head 片段供前端展示，先做一次简单压缩（折叠多余空白）
	r.BodySnippet = snippetBody(body, 1600)
	return r
}

func detectZinFramework(body string) bool {
	lb := strings.ToLower(body)
	return strings.Contains(lb, "zin-") ||
		strings.Contains(lb, "data-zin") ||
		strings.Contains(lb, "zin/core") ||
		strings.Contains(lb, "/zin.js") ||
		strings.Contains(lb, "zentaojs/zin")
}

var whitespaceRe = regexp.MustCompile(`\s+`)

func snippetBody(body string, n int) string {
	s := strings.TrimSpace(whitespaceRe.ReplaceAllString(body, " "))
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func probeAPILogin(ctx context.Context, baseURL, account, password string) *APILoginProbe {
	out := &APILoginProbe{Attempted: true}
	cli := NewAPIClient(baseURL)
	r, err := cli.APILogin(ctx, account, password)
	if err != nil {
		out.OK = false
		out.Error = err.Error()
		return out
	}
	out.OK = true
	out.Account = r.Account
	out.ExpireSeconds = r.Expire
	if t := strings.TrimSpace(r.Token); t != "" {
		out.TokenLength = len(t)
		if len(t) >= 6 {
			out.TokenPreview = t[:3] + "…" + t[len(t)-3:]
		}
	}
	return out
}

func pickBestAPI(results []ProbeEndpointResult) *ProbeEndpointResult {
	for i := range results {
		it := &results[i]
		if it.Status <= 0 || it.Status >= 500 || it.IsLoginPage {
			continue
		}
		ct := strings.ToLower(it.ContentType)
		// 禅道 API v1：即使 401 也会返回 JSON 格式 {"error":"..."}，能判定 API 存在
		if strings.Contains(ct, "json") {
			return it
		}
	}
	return nil
}

func pickBestEffortEndpoint(results []ProbeEndpointResult) *ProbeEndpointResult {
	effortFieldHints := []string{
		"work", "work[]", "work[0]",
		"consumed", "hours", "hours[]", "hours[0]",
		"dates[]", "dates[0]", "date",
		"objectID", "objectID[]",
	}
	var candidates []*ProbeEndpointResult
	for i := range results {
		it := &results[i]
		if it.Status < 200 || it.Status >= 400 || it.IsLoginPage {
			continue
		}
		if !containsAny(it.FoundFields, effortFieldHints) {
			continue
		}
		candidates = append(candidates, it)
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return len(candidates[i].FoundFields) > len(candidates[j].FoundFields)
	})
	return candidates[0]
}

func containsAny(haystack []string, needles []string) bool {
	if len(haystack) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(haystack))
	for _, h := range haystack {
		set[h] = struct{}{}
	}
	for _, n := range needles {
		if _, ok := set[n]; ok {
			return true
		}
	}
	return false
}

// extractFormInfo 从报工页面里找第一个 POST 表单的 action + method。
func extractFormInfo(html string) (action, method string) {
	re := regexp.MustCompile(`(?is)<form[^>]*>`)
	reAct := regexp.MustCompile(`(?is)\baction=['"]([^'"]+)['"]`)
	reMet := regexp.MustCompile(`(?is)\bmethod=['"]([^'"]+)['"]`)
	for _, tag := range re.FindAllString(html, -1) {
		m := reMet.FindStringSubmatch(tag)
		meth := ""
		if len(m) == 2 {
			meth = strings.ToUpper(strings.TrimSpace(m[1]))
		}
		if meth != "" && meth != "POST" {
			continue
		}
		a := reAct.FindStringSubmatch(tag)
		if len(a) == 2 && strings.TrimSpace(a[1]) != "" {
			return strings.TrimSpace(a[1]), meth
		}
	}
	return "", ""
}

// extractFormFieldNames 提取 <input>/<textarea>/<select> 的 name 属性，去重后返回。
func extractFormFieldNames(html string) []string {
	re := regexp.MustCompile(`(?is)<(?:input|textarea|select)\b[^>]*\bname=['"]([^'"]+)['"]`)
	m := re.FindAllStringSubmatch(html, -1)
	seen := make(map[string]struct{}, len(m))
	out := make([]string, 0, len(m))
	for _, it := range m {
		if len(it) < 2 {
			continue
		}
		n := strings.TrimSpace(it[1])
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func resolveRelative(baseURL, action string) string {
	action = strings.TrimSpace(action)
	if action == "" {
		return ""
	}
	if strings.HasPrefix(action, "http://") || strings.HasPrefix(action, "https://") {
		return action
	}
	b, err := url.Parse(strings.TrimRight(baseURL, "/") + "/")
	if err != nil {
		return baseURL + "/" + strings.TrimLeft(action, "/")
	}
	ref, err := url.Parse(action)
	if err != nil {
		return baseURL + "/" + strings.TrimLeft(action, "/")
	}
	return b.ResolveReference(ref).String()
}
