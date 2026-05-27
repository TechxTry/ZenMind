package mcp

import (
	"encoding/json"
	"net/http"
	"zenmind/internal/config"
	"zenmind/internal/db"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const protocolVersion = "2024-11-05"

// Server handles MCP JSON-RPC requests.
type Server struct {
	registry *Registry
}

func NewServer(reg *Registry) *Server {
	return &Server{registry: reg}
}

// RegisterRoutes mounts the MCP endpoint under the given Gin router group.
// Callers should apply JWTMiddleware before this group, or pass the middleware here.
func (s *Server) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("", jwtAuth(), s.handleRPC)
}

// handleRPC is the single entry point for all MCP JSON-RPC calls.
func (s *Server) handleRPC(c *gin.Context) {
	caller, ok := callerFromCtx(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, Response{
			JSONRPC: "2.0",
			Error:   &RPCError{Code: CodeInvalidRequest, Message: "unauthorized"},
		})
		return
	}

	var req Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, Response{
			JSONRPC: "2.0",
			Error:   &RPCError{Code: CodeParseError, Message: "parse error: " + err.Error()},
		})
		return
	}

	switch req.Method {
	case "initialize":
		c.JSON(http.StatusOK, Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: InitializeResult{
				ProtocolVersion: protocolVersion,
				ServerInfo:      ServerInfo{Name: "zenmind-mcp", Version: "0.1.0"},
				Capabilities:    Caps{Tools: &struct{}{}},
			},
		})

	case "tools/list":
		c.JSON(http.StatusOK, Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  ToolsListResult{Tools: s.registry.List()},
		})

	case "tools/call":
		s.handleToolCall(c, req, caller)

	default:
		c.JSON(http.StatusOK, Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: CodeMethodNotFound, Message: "method not found: " + req.Method},
		})
	}
}

func (s *Server) handleToolCall(c *gin.Context, req Request, caller CallerInfo) {
	raw, err := json.Marshal(req.Params)
	if err != nil {
		c.JSON(http.StatusOK, Response{
			JSONRPC: "2.0", ID: req.ID,
			Error: &RPCError{Code: CodeInvalidParams, Message: "invalid params"},
		})
		return
	}
	var params ToolCallParams
	if err := json.Unmarshal(raw, &params); err != nil {
		c.JSON(http.StatusOK, Response{
			JSONRPC: "2.0", ID: req.ID,
			Error: &RPCError{Code: CodeInvalidParams, Message: "invalid params: " + err.Error()},
		})
		return
	}

	tool, ok := s.registry.Get(params.Name)
	if !ok {
		c.JSON(http.StatusOK, Response{
			JSONRPC: "2.0", ID: req.ID,
			Error: &RPCError{Code: CodeMethodNotFound, Message: "unknown tool: " + params.Name},
		})
		return
	}

	result := tool.Execute(c.Request.Context(), caller, params.Arguments)
	c.JSON(http.StatusOK, Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	})
}

// ---- JWT auth middleware (standalone, for MCP endpoint) ----

func jwtAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if len(authHeader) < 8 || authHeader[:7] != "Bearer " {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}
		tokenStr := authHeader[7:]
		if len(tokenStr) > len(db.MCPTokenPrefix) && tokenStr[:len(db.MCPTokenPrefix)] == db.MCPTokenPrefix {
			if tok, ok, err := db.FindUserByMCPRawToken(tokenStr); err != nil {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid mcp token"})
				return
			} else if ok {
				user, found, uErr := db.GetSystemUserByID(tok.UserID)
				if uErr != nil || !found || user.Disabled {
					c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid user"})
					return
				}
				c.Set("mcp_caller", CallerInfo{
					UserID:   user.ID,
					Username: user.Username,
					Role:     user.Role,
					Scope:    user.DataScope,
				})
				db.TouchMCPAccessTokenLastUsed(tok.ID)
				c.Next()
				return
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid mcp token"})
			return
		}
		tok, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(config.Global.JWTSecret), nil
		})
		if err != nil || !tok.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		claims, ok := tok.Claims.(jwt.MapClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid claims"})
			return
		}
		var uid int64
		switch v := claims["uid"].(type) {
		case float64:
			uid = int64(v)
		case int64:
			uid = v
		}
		sub, _ := claims["sub"].(string)
		role, _ := claims["role"].(string)
		scope, _ := claims["scope"].(string)
		c.Set("mcp_caller", CallerInfo{UserID: uid, Username: sub, Role: role, Scope: scope})
		c.Next()
	}
}

func callerFromCtx(c *gin.Context) (CallerInfo, bool) {
	v, exists := c.Get("mcp_caller")
	if !exists {
		return CallerInfo{}, false
	}
	caller, ok := v.(CallerInfo)
	return caller, ok
}
