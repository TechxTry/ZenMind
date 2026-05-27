package mcp

import "context"

// CallerInfo holds the authenticated caller identity extracted from JWT.
type CallerInfo struct {
	UserID   int64
	Username string
	Role     string
	Scope    string
}

// Tool is the interface every MCP tool must implement.
type Tool interface {
	Definition() ToolDef
	Execute(ctx context.Context, caller CallerInfo, args map[string]interface{}) ToolCallResult
}

// Registry holds all registered tools keyed by name.
type Registry struct {
	tools map[string]Tool
	order []string
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

func (r *Registry) Register(t Tool) {
	name := t.Definition().Name
	if _, dup := r.tools[name]; !dup {
		r.order = append(r.order, name)
	}
	r.tools[name] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) List() []ToolDef {
	defs := make([]ToolDef, 0, len(r.order))
	for _, name := range r.order {
		defs = append(defs, r.tools[name].Definition())
	}
	return defs
}
