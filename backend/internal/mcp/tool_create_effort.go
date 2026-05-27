package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
	"zenmind/internal/config"
	"zenmind/internal/db"
	"zenmind/internal/etl"
	"zenmind/internal/zentao"
	"zenmind/internal/zentaoauth"
)

// CreateEffortTool implements the MCP createEffort tool.
// Fields (禅道最小集): object_type, object_id, work_date, consumed, work.
type CreateEffortTool struct{}

func (t CreateEffortTool) Definition() ToolDef {
	return ToolDef{
		Name:        "createEffort",
		Description: "在禅道中填报工时（报工），本地同步记录一份，支持任务/Bug/需求类型。",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"client_request_id": {Type: "string", Description: "客户端幂等键，相同 ID 只会执行一次，建议用 UUID"},
				"object_type":       {Type: "string", Description: "对象类型：task | bug | story，默认 task"},
				"object_id":         {Type: "number", Description: "对象 ID（任务/Bug/需求编号）"},
				"work_date":         {Type: "string", Description: "工作日期 YYYY-MM-DD，默认今天"},
				"consumed":          {Type: "number", Description: "本次消耗工时（小时）"},
				"work":              {Type: "string", Description: "工作说明"},
			},
			Required: []string{"client_request_id", "object_id", "consumed", "work"},
		},
	}
}

func (t CreateEffortTool) Execute(ctx context.Context, caller CallerInfo, args map[string]interface{}) ToolCallResult {
	// ---- parse args ----
	clientReqID := stringArg(args, "client_request_id")
	if strings.TrimSpace(clientReqID) == "" {
		return ErrorResult("client_request_id is required")
	}
	objectType := strings.TrimSpace(stringArg(args, "object_type"))
	if objectType == "" {
		objectType = "task"
	}
	if objectType != "task" && objectType != "bug" && objectType != "story" {
		return ErrorResult("object_type must be task, bug, or story")
	}
	objectID := int64(floatArg(args, "object_id"))
	if objectID <= 0 {
		return ErrorResult("object_id must be a positive integer")
	}
	consumed := floatArg(args, "consumed")
	if consumed <= 0 {
		return ErrorResult("consumed must be greater than 0")
	}
	work := strings.TrimSpace(stringArg(args, "work"))
	if work == "" {
		return ErrorResult("work is required")
	}
	workDate := strings.TrimSpace(stringArg(args, "work_date"))
	if workDate == "" {
		workDate = time.Now().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", workDate); err != nil {
		return ErrorResult("work_date format must be YYYY-MM-DD")
	}

	sub := caller.Username

	// ---- idempotency check ----
	existing, found, err := db.GetMCPEffortLog(clientReqID)
	if err != nil {
		return ErrorResult("db error checking idempotency: " + err.Error())
	}
	if found {
		return TextResult(fmt.Sprintf(
			`{"idempotent":true,"status":"%s","zentao_effort_id":%d,"message":"已处理过，直接返回上次结果"}`,
			existing.Status, existing.ZentaoEffortID,
		))
	}

	// ---- save local log (pending) ----
	logID, err := db.CreateMCPEffortLog(db.MCPEffortLogInput{
		ClientRequestID: clientReqID,
		ActorUsername:   sub,
		ObjectType:      objectType,
		ObjectID:        objectID,
		WorkDate:        workDate,
		Consumed:        consumed,
		Work:            work,
	})
	if err != nil {
		return ErrorResult("failed to create local effort log: " + err.Error())
	}

	// ---- write to zentao ----
	// MCP only supports task effort currently; bug/story use the same variant flow.
	baseURL := strings.TrimSpace(config.Global.ZentaoBaseURL)
	if baseURL == "" {
		_ = db.FailMCPEffortLog(logID, "zentao base_url not configured")
		return ErrorResult("zentao base_url not configured")
	}

	token, loginErr := zentaoauth.EnsureAPIToken(ctx, sub)
	if loginErr != nil {
		_ = db.FailMCPEffortLog(logID, "api login failed: "+loginErr.Error())
		return ErrorResult("禅道 API 登录失败，请在「禅道授权」页面确认账号密码：" + loginErr.Error())
	}

	cli := zentao.NewAPIClient(baseURL)

	var effortID int64
	var mode string

	if objectType == "task" {
		in := zentao.APICreateTaskEffortInput{
			TaskID:   objectID,
			WorkDate: workDate,
			Work:     work,
			Consumed: consumed,
			Left:     0,
		}
		winner, _, fatal := runEffortVariants(ctx, cli, sub, &token, in)
		if fatal != nil {
			body, _ := json.Marshal(fatal.body)
			_ = db.FailMCPEffortLog(logID, string(body))
			return ErrorResult("禅道写入失败：" + fmt.Sprintf("%v", fatal.body["error"]))
		}
		if winner == nil {
			_ = db.FailMCPEffortLog(logID, "all variants returned false verify_matched")
			return ErrorResult("禅道 API 三种变体均未能确认写入，请手动核实")
		}
		effortID = winner.ID
		mode = "api_v1"
	} else {
		_ = db.FailMCPEffortLog(logID, "bug/story effort write not yet supported via MCP")
		return ErrorResult("当前 MCP 仅支持 task 类型报工，bug/story 支持开发中")
	}

	// ---- update local log (success) ----
	if err := db.SucceedMCPEffortLog(logID, effortID, mode); err != nil {
		log.Printf("[mcp/createEffort] update log success failed: %v", err)
	}

	// ---- async ETL sync ----
	go func() {
		etl.SyncEfforts()
		etl.SyncTasks()
	}()

	// ---- audit ----
	_ = db.WriteAudit(db.AuditInput{
		ActorUserID:   &caller.UserID,
		ActorUsername: caller.Username,
		Action:        "mcp_create_effort",
		TargetType:    objectType,
		TargetID:      fmt.Sprintf("%d", objectID),
		Metadata: map[string]interface{}{
			"client_request_id": clientReqID,
			"work_date":         workDate,
			"consumed":          consumed,
			"zentao_effort_id":  effortID,
			"mode":              mode,
		},
	})

	return TextResult(fmt.Sprintf(
		`{"ok":true,"zentao_effort_id":%d,"mode":"%s","work_date":"%s","consumed":%g,"object_type":"%s","object_id":%d}`,
		effortID, mode, workDate, consumed, objectType, objectID,
	))
}

// runEffortVariants is a package-level helper that wraps the handler-layer logic,
// keeping MCP and HTTP paths consistent without duplicating the variant loop.
func runEffortVariants(
	ctx context.Context,
	cli *zentao.APIClient,
	sub string,
	token *string,
	in zentao.APICreateTaskEffortInput,
) (*zentao.APICreateTaskEffortResult, []map[string]interface{}, *effortFatal) {
	for _, variant := range zentao.AllEffortVariants {
		r, err := cli.APICreateTaskEffortByVariant(ctx, *token, in, variant)

		if err != nil && zentao.IsAPIUnauthorizedError(err) {
			zentaoauth.DeleteAPIToken(ctx, sub)
			newToken, reloginErr := zentaoauth.LoginAndCacheAPIToken(ctx, sub)
			if reloginErr != nil {
				return nil, nil, &effortFatal{
					status: http.StatusUnauthorized,
					body:   map[string]interface{}{"error": "Token 过期后重新登录失败：" + reloginErr.Error()},
				}
			}
			*token = newToken
			r, err = cli.APICreateTaskEffortByVariant(ctx, *token, in, variant)
		}

		if err == nil {
			return r, nil, nil
		}

		if errors.Is(err, zentao.ErrAPIEffortNotPersisted) {
			continue
		}
		if he, ok := zentao.IsAPIHTTPError(err); ok {
			switch he.Status {
			case http.StatusBadRequest, http.StatusNotFound,
				http.StatusMethodNotAllowed, http.StatusUnprocessableEntity:
				continue
			}
			return nil, nil, &effortFatal{
				status: http.StatusBadGateway,
				body:   map[string]interface{}{"error": err.Error(), "api_status": he.Status},
			}
		}
		return nil, nil, &effortFatal{
			status: http.StatusBadGateway,
			body:   map[string]interface{}{"error": err.Error()},
		}
	}
	return nil, nil, nil
}

type effortFatal struct {
	status int
	body   map[string]interface{}
}

// ---- arg helpers ----

func stringArg(args map[string]interface{}, key string) string {
	v, _ := args[key].(string)
	return v
}

func floatArg(args map[string]interface{}, key string) float64 {
	switch v := args[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	}
	return 0
}
