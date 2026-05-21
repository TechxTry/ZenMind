package main

import (
	"context"
	"log"
	"net/http"
	"zenmind/internal/config"
	"zenmind/internal/db"
	"zenmind/internal/handlers"
	"zenmind/internal/redisclient"
	"zenmind/internal/scheduler"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	// 1. Load config from env
	config.Load()

	// 1b. Load ETL table mapping config (config/etl_tables.yaml)
	config.LoadETLConfig()

	// 2. Connect PostgreSQL
	db.InitPG()
	if err := db.RunPendingMigrations(); err != nil {
		log.Fatalf("[main] migrations: %v", err)
	}
	if err := db.EnsureBootstrapAdmin(); err != nil {
		log.Fatalf("[main] bootstrap admin: %v", err)
	}
	if err := db.EnsureAppSettings(); err != nil {
		log.Fatalf("[main] app_settings: %v", err)
	}
	config.Global.SyncIntervalMinutes = db.GetSyncIntervalMinutes()
	config.Global.ZentaoBaseURL = db.GetZentaoBaseURL()
	config.Global.ZentaoLoginURL = db.GetZentaoLoginURL()
	log.Printf("[main] sync interval: %d min (env default + app_settings)", config.Global.SyncIntervalMinutes)
	log.Printf("[main] zentao base url: %s (env default + app_settings)", config.Global.ZentaoBaseURL)
	log.Printf("[main] zentao login url: %s (env default + app_settings)", config.Global.ZentaoLoginURL)

	// 2b. Init Redis client (used by session auth & future async jobs)
	redisclient.Init()
	if err := redisclient.Ping(context.Background()); err != nil {
		log.Printf("[main] redis ping failed (continuing): %v", err)
	}

	// 3. Connect Zentao MySQL (optional at startup — user configures via UI)
	if config.Global.ZentaoHost != "" {
		if err := db.ConnectZentao(config.Global); err != nil {
			log.Printf("[main] Zentao MySQL connection failed (continuing): %v", err)
		}
	}

	// 4. Periodic ETL (interval persisted in PG, configurable via UI / SYNC_INTERVAL_MINUTES)
	scheduler.StartPeriodicETL(context.Background())
	scheduler.StartPeriodicZentaoAuthRefresh(context.Background())

	// 5. Build Gin router
	r := gin.Default()
	// 避免直接访问后端端口时只看到 404：浏览器应打开前端 Nginx（WEB_PORT，默认 80），/api 由 Nginx 反代到本服务
	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service": "zenmind-api",
			"hint":    "浏览器请访问前端端口（Docker 中 WEB_PORT，默认 80，例如 http://localhost），勿直接访问本后端端口；REST 接口前缀为 /api。",
		})
	})
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Authorization", "Content-Type"},
		AllowCredentials: false,
	}))

	// Public routes
	r.POST("/api/login", handlers.Login)

	// Protected routes
	api := r.Group("/api", handlers.JWTMiddleware())
	{
		api.GET("/me", handlers.Me)
		api.GET("/me/calendar-feeds", handlers.ListMyCalendarFeeds)
		api.POST("/me/calendar-feeds", handlers.CreateMyCalendarFeed)
		api.DELETE("/me/calendar-feeds/:id", handlers.DeleteMyCalendarFeed)
		api.GET("/me/calendar-aggregate", handlers.GetMyCalendarAggregate)
		api.GET("/me/calendar-accounts", handlers.ListMyCalendarAccounts)
		api.POST("/me/calendar-accounts", handlers.CreateMyCalendarAccount)
		api.DELETE("/me/calendar-accounts/:id", handlers.DeleteMyCalendarAccount)

		// Admin
		api.GET("/admin/system-users", handlers.AdminListSystemUsers)
		api.POST("/admin/system-users", handlers.AdminCreateSystemUser)
		api.POST("/admin/system-users/batch", handlers.AdminBatchCreateSystemUsers)
		api.PATCH("/admin/system-users/:id", handlers.AdminUpdateSystemUser)
		api.POST("/admin/system-users/:id/reset-password", handlers.AdminResetSystemUserPassword)
		api.GET("/admin/system-users/:id/zentao-binding", handlers.AdminGetZentaoBinding)
		api.PUT("/admin/system-users/:id/zentao-binding", handlers.AdminSetZentaoBinding)
		api.DELETE("/admin/system-users/:id/zentao-binding", handlers.AdminDeleteZentaoBinding)
		api.GET("/admin/audit-logs", handlers.AdminListAuditLogs)

		// Config
		api.GET("/config/datasource", handlers.GetDatasource)
		api.PUT("/config/datasource", handlers.PutDatasource)
		api.POST("/config/datasource/test", handlers.TestDatasource)
		api.GET("/config/zentao-api", handlers.GetZentaoAPIConfig)
		api.PUT("/config/zentao-api", handlers.PutZentaoAPIConfig)
		api.GET("/config/sync-settings", handlers.GetSyncSettings)
		api.PUT("/config/sync-settings", handlers.PutSyncSettings)
		api.GET("/config/local-stats", handlers.GetLocalStats)
		api.GET("/config/business", handlers.GetBusinessConfig)
		api.PUT("/config/business", handlers.PutBusinessConfig)

		// Users
		api.GET("/users", handlers.ListUsers)

		// Project Groups
		api.GET("/groups", handlers.ListGroups)
		api.POST("/groups", handlers.CreateGroup)
		api.PUT("/groups/:id", handlers.UpdateGroup)
		api.DELETE("/groups/:id", handlers.DeleteGroup)
		api.GET("/groups/:id/members", handlers.GetGroupMembers)
		api.PUT("/groups/:id/members", handlers.UpdateGroupMembers)

		// Analytics
		api.GET("/analytics/effort-heatmap", handlers.EffortHeatmap)
		api.GET("/analytics/user-load", handlers.UserLoad)
		api.GET("/analytics/workload-distribution", handlers.WorkloadDistribution)
		api.GET("/analytics/iteration/overview", handlers.IterationOverview)
		api.GET("/analytics/iteration/burndown", handlers.IterationBurndown)
		api.GET("/analytics/iteration/cfd", handlers.IterationCFD)
		api.GET("/analytics/iteration/cycle-time", handlers.IterationCycleTime)
		api.GET("/analytics/iteration/scope-change", handlers.IterationScopeChange)
		api.GET("/analytics/people/overview", handlers.PeopleOverview)
		api.GET("/analytics/people/wip-trend", handlers.PeopleWIPTrend)
		api.GET("/analytics/people/throughput", handlers.PeopleThroughput)
		api.GET("/analytics/people/bottleneck", handlers.PeopleBottleneck)

		// Workbench (tasks/:id must be registered before /workbench/tasks; projects/:id before /workbench/projects)
		api.GET("/workbench/tasks/:id", handlers.GetTask)
		api.GET("/workbench/tasks", handlers.ListTasks)
		api.GET("/workbench/stories", handlers.ListStories)
		api.GET("/workbench/bugs", handlers.ListBugs)
		api.GET("/workbench/efforts", handlers.ListEfforts)
		api.GET("/workbench/executions", handlers.ListExecutions)
		api.GET("/workbench/projects/:id", handlers.GetWorkbenchProject)
		api.GET("/workbench/projects", handlers.ListWorkbenchProjects)
		api.GET("/workbench/structure", handlers.GetWorkbenchStructure)

		// Sync
		api.POST("/sync/trigger", handlers.TriggerSync)
		api.GET("/sync/status", handlers.GetSyncStatus)

		// Zentao auth (session)
		api.GET("/zentao/auth/status", handlers.GetZentaoAuthStatus)
		api.POST("/zentao/auth/test", handlers.TestZentaoAuth)
		api.POST("/zentao/auth/test-saved", handlers.TestZentaoAuthSaved)
		api.POST("/zentao/auth/bind", handlers.BindZentaoAuth)
		api.POST("/zentao/auth/bind-saved", handlers.BindZentaoAuthSaved)
		api.POST("/zentao/auth/probe", handlers.ProbeZentaoAuth)
		api.DELETE("/zentao/auth/clear", handlers.ClearZentaoAuth)

		// Zentao write-back
		api.POST("/zentao/efforts", handlers.CreateZentaoEffort)
	}

	log.Printf("[main] ZenMind backend listening on :%s", config.Global.Port)
	if err := r.Run(":" + config.Global.Port); err != nil {
		log.Fatalf("[main] server failed: %v", err)
	}
}
