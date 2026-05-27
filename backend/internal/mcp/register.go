package mcp

// NewDefaultServer builds a Server with all built-in tools registered.
func NewDefaultServer() *Server {
	reg := NewRegistry()
	reg.Register(CreateEffortTool{})
	reg.Register(ListMyTasksTool{})
	reg.Register(ListMyEffortsTool{})
	return NewServer(reg)
}
