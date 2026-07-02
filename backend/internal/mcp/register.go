package mcp

// NewDefaultServer builds a Server with all built-in tools registered.
func NewDefaultServer() *Server {
	reg := NewRegistry()
	reg.Register(CreateEffortTool{})
	reg.Register(CreateTaskTool{})
	reg.Register(UpdateEffortTool{})
	reg.Register(DeleteEffortTool{})
	reg.Register(UpdateTaskTool{})
	reg.Register(CreateStoryTool{})
	reg.Register(UpdateStoryTool{})
	reg.Register(DeleteStoryTool{})
	reg.Register(CreateBugTool{})
	reg.Register(UpdateBugTool{})
	reg.Register(DeleteBugTool{})
	reg.Register(ListMyTasksTool{})
	reg.Register(ListMyBugsTool{})
	reg.Register(ListMyStoriesTool{})
	reg.Register(ListMyExecutionsTool{})
	reg.Register(ListMyEffortsTool{})
	return NewServer(reg)
}
