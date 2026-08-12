// Package mcp implements a minimal MCP (Model Context Protocol) server over HTTP/JSON-RPC 2.0.
// Spec reference: https://modelcontextprotocol.io/specification
package mcp

// ---- JSON-RPC 2.0 envelope ----

type Request struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Standard JSON-RPC error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// ---- MCP protocol types ----

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type InitializeResult struct {
	ProtocolVersion string     `json:"protocolVersion"`
	ServerInfo      ServerInfo `json:"serverInfo"`
	Capabilities    Caps       `json:"capabilities"`
}

type Caps struct {
	Tools *struct{} `json:"tools,omitempty"`
}

type ToolDef struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

type Property struct {
	Type                 string `json:"type"`
	Description          string `json:"description"`
	AdditionalProperties *bool  `json:"additionalProperties,omitempty"`
}

type ToolsListResult struct {
	Tools []ToolDef `json:"tools"`
}

type ToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ToolCallResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

func TextResult(text string) ToolCallResult {
	return ToolCallResult{Content: []ContentItem{{Type: "text", Text: text}}}
}

func ErrorResult(text string) ToolCallResult {
	return ToolCallResult{IsError: true, Content: []ContentItem{{Type: "text", Text: text}}}
}
