import http from './http'

// ---- Auth ----
export const login = (username: string, password: string) =>
  http.post('/login', { username, password }).then((r) => r.data)

export type MeResponse = {
  user: {
    id: number
    username: string
    display_name?: string
    role: string
    data_scope: string
    default_group_id?: number | null
    disabled: boolean
  }
  zentao_binding?: {
    id: number
    system_user_id: number
    zentao_account: string
  } | null
}

export const getMe = () => http.get<MeResponse>('/me').then((r) => r.data)

// ---- Personal calendar (我的工作台) ----
export type CalendarFeed = {
  id: number
  name: string
  feed_host: string
  color: string
}
export const listCalendarFeeds = () =>
  http.get<{ data: CalendarFeed[] }>('/me/calendar-feeds').then((r) => r.data)
export const createCalendarFeed = (data: { name: string; ical_url: string; color?: string }) =>
  http.post<{ id: number }>('/me/calendar-feeds', data).then((r) => r.data)
export const deleteCalendarFeed = (id: number) => http.delete(`/me/calendar-feeds/${id}`).then((r) => r.data)

export type CalendarAccountType = 'exchange' | 'caldav'
export type CalendarAccount = {
  id: number
  type: CalendarAccountType | string
  server: string
  username: string
  description: string
  created_at: string
}
export const listCalendarAccounts = () =>
  http.get<{ data: CalendarAccount[] }>('/me/calendar-accounts').then((r) => r.data)
export const createCalendarAccount = (data: {
  type: CalendarAccountType
  server?: string
  username: string
  password: string
  description?: string
}) => http.post<{ id: number }>('/me/calendar-accounts', data).then((r) => r.data)
export const deleteCalendarAccount = (id: number) => http.delete(`/me/calendar-accounts/${id}`).then((r) => r.data)

export type CalendarExternalEvent = {
  source_type: 'feed' | 'account' | string
  source_id: number
  source_name: string
  title: string
  start: string
  end: string
  all_day: boolean
  color: string
}
export type CalendarAggregate = {
  efforts: Array<{
    id: number
    work_date?: string
    consumed: number
    work: string
    object_type: string
    object_id: number
    object_title?: string
  }>
  external: CalendarExternalEvent[]
  feed_errors?: Array<{ feed_id: number; feed_name: string; error: string }>
  account_errors?: Array<{ account_id: number; type: string; username: string; error: string }>
}
export const getCalendarAggregate = (params: { date_from: string; date_to: string }) =>
  http.get<CalendarAggregate>('/me/calendar-aggregate', { params }).then((r) => r.data)

export type MCPAccessToken = {
  id: number
  user_id: number
  token_name: string
  token_prefix: string
  expires_at?: string | null
  last_used_at?: string | null
  created_at: string
}

export const listMyMCPTokens = () =>
  http.get<{ data: MCPAccessToken[] }>('/me/mcp-tokens').then((r) => r.data)
export const createMyMCPToken = (data: { name: string; expire_days?: number }) =>
  http.post<{ id: number; token: string; token_name: string; expires_at?: string }>('/me/mcp-tokens', data).then((r) => r.data)
export const deleteMyMCPToken = (id: number) => http.delete(`/me/mcp-tokens/${id}`).then((r) => r.data)

// ---- Admin: System Users ----
export type SystemUser = {
  id: number
  username: string
  display_name?: string
  role: 'super_admin' | 'admin' | 'user' | string
  data_scope: 'SELF' | 'GROUP' | 'ALL' | string
  default_group_id?: number | null
  disabled: boolean
  created_at: string
  updated_at: string
}

export const adminListSystemUsers = (params?: { q?: string; page?: number; page_size?: number }) =>
  http.get('/admin/system-users', { params }).then((r) => r.data)

export const adminCreateSystemUser = (data: {
  username: string
  display_name?: string
  password: string
  role?: string
  data_scope?: string
  default_group_id?: number
}) => http.post('/admin/system-users', data).then((r) => r.data)

export const adminUpdateSystemUser = (
  id: number,
  data: { display_name?: string; role?: string; data_scope?: string; default_group_id?: number | null; disabled?: boolean },
) => http.patch(`/admin/system-users/${id}`, data).then((r) => r.data)

export const adminResetSystemUserPassword = (id: number, data?: { new_password?: string }) =>
  http.post(`/admin/system-users/${id}/reset-password`, data ?? {}).then((r) => r.data)

export type AdminBatchCreateSystemUserItem = {
  username: string
  display_name: string
  password: string
}

export const adminBatchCreateSystemUsers = (data: {
  accounts: string[]
  role?: string
  data_scope?: string
  default_group_id?: number | null
}) =>
  http.post<{ created: AdminBatchCreateSystemUserItem[] }>('/admin/system-users/batch', data).then((r) => r.data)

// ---- Admin: Zentao Binding ----
export const adminGetZentaoBinding = (id: number) => http.get(`/admin/system-users/${id}/zentao-binding`).then((r) => r.data)
export const adminSetZentaoBinding = (id: number, zentao_account: string) =>
  http.put(`/admin/system-users/${id}/zentao-binding`, { zentao_account }).then((r) => r.data)
export const adminDeleteZentaoBinding = (id: number) =>
  http.delete(`/admin/system-users/${id}/zentao-binding`).then((r) => r.data)

// ---- Admin: Audit Logs ----
export type AuditLog = {
  id: number
  actor_user_id?: number | null
  actor_username?: string
  action: string
  target_type?: string
  target_id?: string
  metadata?: any
  ip?: string
  ua?: string
  created_at: string
}
export const adminListAuditLogs = (params?: {
  action?: string
  actor?: string
  from?: string
  to?: string
  page?: number
  page_size?: number
}) => http.get('/admin/audit-logs', { params }).then((r) => r.data)

// ---- Config ----
export const getDatasource = () => http.get('/config/datasource').then((r) => r.data)
export const putDatasource = (data: object) => http.put('/config/datasource', data).then((r) => r.data)
export const testDatasource = (data: object) => http.post('/config/datasource/test', data).then((r) => r.data)
export const getLocalStats = () => http.get('/config/local-stats').then((r) => r.data)

export const getSyncSettings = () => http.get('/config/sync-settings').then((r) => r.data)
export const putSyncSettings = (data: { interval_minutes: number }) =>
  http.put('/config/sync-settings', data).then((r) => r.data)

// ---- Business Config ----
export const getBusinessConfig = () => http.get('/config/business').then((r) => r.data)
export const putBusinessConfig = (data: { daily_standard_hours: number }) =>
  http.put('/config/business', data).then((r) => r.data)

// ---- Zentao API (write-back) ----
export const getZentaoAPIConfig = () => http.get('/config/zentao-api').then((r) => r.data)
export const putZentaoAPIConfig = (data: { base_url: string; login_url: string }) =>
  http.put('/config/zentao-api', data).then((r) => r.data)

// ---- Zentao Auth (session login) ----
export const testZentaoAuth = (data: { password: string }) =>
  http.post('/zentao/auth/test', data).then((r) => r.data)
/** 使用服务端已保存的禅道凭证测试登录（无需传密码） */
export const testZentaoAuthSaved = () => http.post('/zentao/auth/test-saved', {}).then((r) => r.data)
export const bindZentaoAuth = (data: { password: string }) =>
  http.post('/zentao/auth/bind', data).then((r) => r.data)
/** 使用服务端已保存的禅道凭证绑定/重建会话（无需传密码） */
export const bindZentaoAuthSaved = () => http.post('/zentao/auth/bind-saved', {}).then((r) => r.data)
export const getZentaoAuthStatus = () => http.get('/zentao/auth/status').then((r) => r.data)
export const clearZentaoAuth = () => http.delete('/zentao/auth/clear').then((r) => r.data)

export type ZentaoProbeEndpoint = {
  label: string
  method?: string
  url: string
  status: number
  final_url: string
  redirected: boolean
  is_login_page: boolean
  content_len: number
  content_type?: string
  form_action?: string
  form_method?: string
  found_fields?: string[]
  csrf_name?: string
  csrf_present: boolean
  zin_detected?: boolean
  body_snippet?: string
  error?: string
}
export type ZentaoAPILoginProbe = {
  attempted: boolean
  ok: boolean
  token_length?: number
  token_preview?: string
  expire_seconds?: number
  account?: string
  error?: string
}
export type ZentaoProbeResult = {
  base_url: string
  used_task_id: number
  session_valid: boolean
  session_check: ZentaoProbeEndpoint[]
  effort_endpoints: ZentaoProbeEndpoint[]
  api_endpoints: ZentaoProbeEndpoint[]
  api_login?: ZentaoAPILoginProbe
  recommended_url?: string
  recommended_fields?: string[]
  recommended_csrf?: string
  recommended_mode?: 'webform' | 'api_v1'
  notes?: string[]
}
export const probeZentaoAuth = (data?: { task_id?: number }) =>
  http.post<{ ok: boolean; error?: string; result?: ZentaoProbeResult }>('/zentao/auth/probe', data ?? {}).then((r) => r.data)

// ---- Zentao Efforts (write-back) ----
export const createZentaoEffort = (data: {
  task_id: number
  work_date?: string
  work: string
  consumed: number | string
  left: number | string
}) => http.post('/zentao/efforts', data).then((r) => r.data)

// ---- Users ----
export const listUsers = (params?: { q?: string; page?: number; page_size?: number }) =>
  http.get('/users', { params }).then((r) => r.data)

// ---- Groups ----
export const listGroups = () => http.get('/groups').then((r) => r.data)
export const createGroup = (data: { name: string; description?: string }) =>
  http.post('/groups', data).then((r) => r.data)
export const updateGroup = (id: number, data: { name: string; description?: string }) =>
  http.put(`/groups/${id}`, data).then((r) => r.data)
export const deleteGroup = (id: number) => http.delete(`/groups/${id}`).then((r) => r.data)
export const getGroupMembers = (id: number) => http.get(`/groups/${id}/members`).then((r) => r.data)
export const updateGroupMembers = (id: number, accounts: string[]) =>
  http.put(`/groups/${id}/members`, { accounts }).then((r) => r.data)

// ---- Workbench ----
export type WorkbenchParams = {
  group_id?: number
  /** 1：仅返回账号管理中绑定的禅道账号相关数据（我的工作台） */
  my_binding?: 0 | 1
  /** 记录主键 ID（任务/需求/缺陷/报工等 Tab 精确筛选） */
  id?: number
  /** 迭代名（仅用于「迭代」Tab 模糊筛选） */
  name?: string
  status?: string
  assigned_to?: string
  severity?: string
  account?: string
  /** 禅道迭代 (zt_project) ID，用于筛选任务/需求/缺陷/日志 */
  execution_id?: number
  /** 项目 ID（结构树筛选或「项目」Tab 按 ID 搜索） */
  project_id?: number
  /** 仅报工：筛选关联到指定任务 ID 的报工（object_type=task） */
  task_id?: number
  date_from?: string
  date_to?: string
  page?: number
  page_size?: number
}
export const listTasks = (params: WorkbenchParams) => http.get('/workbench/tasks', { params }).then((r) => r.data)
export const getTask = (id: number, params?: { group_id?: number; my_binding?: 0 | 1 }) =>
  http.get(`/workbench/tasks/${id}`, { params }).then((r) => r.data)
export const listStories = (params: WorkbenchParams) => http.get('/workbench/stories', { params }).then((r) => r.data)
export const listBugs = (params: WorkbenchParams) => http.get('/workbench/bugs', { params }).then((r) => r.data)
export const listEfforts = (params: WorkbenchParams) => http.get('/workbench/efforts', { params }).then((r) => r.data)
export const listExecutions = (params: WorkbenchParams) => http.get('/workbench/executions', { params }).then((r) => r.data)
export type LocalProject = {
  id: number
  name: string
  status: string
  parent_id?: number | null
  path?: string
  grade?: number | null
  begin_date?: string | null
  end_date?: string | null
  deleted?: boolean
  raw_data?: unknown
  synced_at?: string
}
export type WorkbenchProjectDetail = {
  project: LocalProject
  program_name?: string
  executions: Array<{
    id: number
    name: string
    status: string
    begin_date?: string | null
    end_date?: string | null
  }>
}
export const listProjects = (params: WorkbenchParams) => http.get('/workbench/projects', { params }).then((r) => r.data)
export const getWorkbenchProject = (id: number, params?: { group_id?: number }) =>
  http.get<WorkbenchProjectDetail>(`/workbench/projects/${id}`, { params }).then((r) => r.data)
export const getWorkbenchStructure = () => http.get('/workbench/structure').then((r) => r.data)

// ---- Analytics ----
export type EffortHeatmapParams = {
  group_id: number
  start: string
  end: string
  exclude_weekend?: boolean
  target_hours?: number
  overload_hours?: number
  overload_streak?: number
}
export const getEffortHeatmap = (params: EffortHeatmapParams) =>
  http.get('/analytics/effort-heatmap', { params }).then((r) => r.data)

export type UserLoadParams = {
  group_id: number
  execution_id?: number
}
export const getUserLoad = (params: UserLoadParams) => http.get('/analytics/user-load', { params }).then((r) => r.data)

export type WorkloadDistributionParams = {
  group_id: number
  start: string
  end: string
  account?: string
}
export const getWorkloadDistribution = (params: WorkloadDistributionParams) =>
  http.get('/analytics/workload-distribution', { params }).then((r) => r.data)

// ---- Iteration Analytics ----
export type IterationAnalyticsParams = {
  group_id?: number
  execution_id: number
  date_from?: string
  date_to?: string
}
export const getIterationOverview = (params: IterationAnalyticsParams) =>
  http.get('/analytics/iteration/overview', { params }).then((r) => r.data)
export const getIterationBurndown = (params: IterationAnalyticsParams) =>
  http.get('/analytics/iteration/burndown', { params }).then((r) => r.data)
export const getIterationCFD = (params: IterationAnalyticsParams) =>
  http.get('/analytics/iteration/cfd', { params }).then((r) => r.data)
export const getIterationCycleTime = (params: IterationAnalyticsParams) =>
  http.get('/analytics/iteration/cycle-time', { params }).then((r) => r.data)
export const getIterationScopeChange = (params: IterationAnalyticsParams) =>
  http.get('/analytics/iteration/scope-change', { params }).then((r) => r.data)

// ---- People Analytics ----
export type PeopleAnalyticsParams = {
  group_id?: number
  date_from: string
  date_to: string
}
export const getPeopleOverview = (params: PeopleAnalyticsParams) =>
  http.get('/analytics/people/overview', { params }).then((r) => r.data)
export const getPeopleWipTrend = (params: PeopleAnalyticsParams) =>
  http.get('/analytics/people/wip-trend', { params }).then((r) => r.data)
export const getPeopleThroughput = (params: PeopleAnalyticsParams) =>
  http.get('/analytics/people/throughput', { params }).then((r) => r.data)
export const getPeopleBottleneck = (params: PeopleAnalyticsParams) =>
  http.get('/analytics/people/bottleneck', { params }).then((r) => r.data)

// ---- Sync ----
export const triggerSync = () => http.post('/sync/trigger').then((r) => r.data)
export const triggerEffortReconcile = () => http.post('/sync/effort-reconcile').then((r) => r.data)
export const getSyncStatus = () => http.get('/sync/status').then((r) => r.data)
