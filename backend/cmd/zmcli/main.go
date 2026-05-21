// zmcli：本地命令行调用 ZenMind 后端，由服务端代用户向禅道提交报工（POST /api/zentao/efforts）。
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "effort":
		os.Exit(runEffort(os.Args[2:]))
	case "help", "-h", "--help":
		printUsage(os.Stdout)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "未知子命令: %q\n\n", os.Args[1])
		printUsage(os.Stderr)
		os.Exit(2)
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintf(w, `用法:
  zmcli effort [选项]     向禅道提交一条任务报工（经 ZenMind 后端）

环境变量（可被同名命令行参数覆盖）:
  ZENMIND_URL           后端根地址，默认 http://127.0.0.1:8080
  ZENMIND_USERNAME      ZenMind 登录用户名
  ZENMIND_PASSWORD      ZenMind 登录密码
  ZENMIND_TOKEN         若已持有 JWT，可跳过登录（勿长期写入 shell 历史）

前置条件:
  - 后端已配置禅道 Base URL；当前用户在 Web「禅道授权」中已绑定有效禅道凭证（与浏览器报工一致）。

示例:
  export ZENMIND_URL=http://localhost:8080
  export ZENMIND_USERNAME=admin
  export ZENMIND_PASSWORD='***'
  zmcli effort -task 12345 -work "联调接口" -consumed 2 -left 6
  zmcli effort -task 12345 -work "修 bug" -consumed 1 -left 0 -date 2026-05-16

`)
}

func runEffort(args []string) int {
	fs := flag.NewFlagSet("effort", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "用法: zmcli effort [选项]\n\n选项:\n")
		fs.PrintDefaults()
	}

	baseURL := fs.String("url", strings.TrimSpace(os.Getenv("ZENMIND_URL")), "ZenMind 后端根 URL")
	username := fs.String("user", strings.TrimSpace(os.Getenv("ZENMIND_USERNAME")), "ZenMind 用户名")
	password := fs.String("password", strings.TrimSpace(os.Getenv("ZENMIND_PASSWORD")), "ZenMind 密码")
	token := fs.String("token", strings.TrimSpace(os.Getenv("ZENMIND_TOKEN")), "已有 JWT，非空则跳过登录")
	taskID := fs.Int64("task", 0, "禅道任务 ID（必填）")
	work := fs.String("work", "", "工作内容说明（必填）")
	consumed := fs.Float64("consumed", -1, "本次消耗工时（必填）")
	left := fs.Float64("left", -1, "剩余工时（必填）")
	workDate := fs.String("date", "", "工作日期 YYYY-MM-DD，默认当天")

	fs.Parse(args)
	if strings.TrimSpace(*baseURL) == "" {
		*baseURL = "http://127.0.0.1:8080"
	}
	if *taskID <= 0 {
		fmt.Fprintln(os.Stderr, "缺少或无效 -task")
		return 2
	}
	if strings.TrimSpace(*work) == "" {
		fmt.Fprintln(os.Stderr, "缺少 -work")
		return 2
	}
	if *consumed < 0 {
		fmt.Fprintln(os.Stderr, "缺少或无效 -consumed")
		return 2
	}
	if *left < 0 {
		fmt.Fprintln(os.Stderr, "缺少或无效 -left（剩余为 0 时请显式写 -left 0）")
		return 2
	}

	client := &http.Client{Timeout: 40 * time.Second}
	apiBase := strings.TrimRight(strings.TrimSpace(*baseURL), "/")

	var jwt string
	var err error
	if strings.TrimSpace(*token) != "" {
		jwt = strings.TrimSpace(*token)
	} else {
		if strings.TrimSpace(*username) == "" || strings.TrimSpace(*password) == "" {
			fmt.Fprintln(os.Stderr, "需要 ZenMind 凭证：设置 ZENMIND_USERNAME / ZENMIND_PASSWORD，或使用 -token")
			return 2
		}
		jwt, err = login(client, apiBase, *username, *password)
		if err != nil {
			fmt.Fprintf(os.Stderr, "登录失败: %v\n", err)
			return 1
		}
	}

	body := map[string]any{
		"task_id":  *taskID,
		"work":     strings.TrimSpace(*work),
		"consumed": *consumed,
		"left":     *left,
	}
	if d := strings.TrimSpace(*workDate); d != "" {
		body["work_date"] = d
	}
	raw, status, err := postJSON(client, apiBase+"/api/zentao/efforts", jwt, body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "请求失败: %v\n", err)
		return 1
	}

	var pretty bytes.Buffer
	if json.Valid(raw) {
		_ = json.Indent(&pretty, raw, "", "  ")
		fmt.Println(pretty.String())
	} else {
		fmt.Println(string(raw))
	}

	if status < 200 || status >= 300 {
		return 1
	}
	var envelope struct {
		OK *bool `json:"ok"`
	}
	if json.Unmarshal(raw, &envelope) == nil && envelope.OK != nil && !*envelope.OK {
		return 1
	}
	return 0
}

func login(client *http.Client, apiBase, user, pass string) (string, error) {
	payload := map[string]string{"username": user, "password": pass}
	raw, status, err := postJSON(client, apiBase+"/api/login", "", payload)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", status, string(raw))
	}
	var out struct {
		Token string `json:"token"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("解析登录响应: %w", err)
	}
	if out.Token == "" {
		if out.Error != "" {
			return "", fmt.Errorf("%s", out.Error)
		}
		return "", fmt.Errorf("响应中无 token")
	}
	return out.Token, nil
}

func postJSON(client *http.Client, url, bearer string, payload any) ([]byte, int, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "zenmind-zmcli")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return raw, resp.StatusCode, nil
}
