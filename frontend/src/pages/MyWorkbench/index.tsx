import React, { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Alert,
  Button,
  Card,
  DatePicker,
  Divider,
  Drawer,
  Form,
  Grid,
  Input,
  InputNumber,
  List,
  Modal,
  Select,
  Space,
  Tabs,
  Table,
  Tag,
  Typography,
  message,
} from 'antd'
import type { Dayjs } from 'dayjs'
import dayjs from 'dayjs'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import type { CalendarAggregate, CalendarExternalEvent } from '../../api'
import {
  createZentaoEffort,
  getBusinessConfig,
  getCalendarAggregate,
  getZentaoAuthStatus,
  getSyncSettings,
  getSyncStatus,
  listEfforts,
  listTasks,
} from '../../api'
import { useAuthStore } from '../../store/auth'
import {
  CALENDAR_CATEGORY_COLORS,
  MacMonthCalendar,
  getCalendarEventDisplayColor,
} from '../../components/MacMonthCalendar'
import MyWorkbenchSettings from '../../components/MyWorkbenchSettings'
import TableColumnSettings from '../../components/TableColumnSettings'
import { resizableTableComponents } from '../../components/ResizableTableHeaderCell'
import {
  readWeekStartsOn,
  saveWeekStartsOn,
  type WeekStartsOn,
} from '../../utils/myWorkbenchCalendarPrefs'
import { useTableColumnPrefs } from '../../hooks/useTableColumnPrefs'
import type { ColumnMeta } from '../../utils/tableColumnPrefs'

const { Text } = Typography

const myWorkbenchTaskDetailHref = (taskId: number) =>
  `/workbench/task/${taskId}?from=/my-workbench`

type TaskRow = {
  id: number
  name: string
  status: string
  type?: string
  estimate?: number
  consumed?: number
  assigned_to?: string
  last_edited_date?: string
}

type EffortRow = {
  id: number
  work_date: string
  consumed: number
  work: string
  object_type: string
  object_id: number
  object_title?: string
}

type EffortLike = {
  id: number
  work_date?: string
  consumed: number
  work: string
  object_type: string
  object_id: number
  object_title?: string
}

type SyncInfo = {
  watermark: string
  last_count: number
  updated_at: string
}

const STATUS_OPTIONS = [
  { value: 'doing', label: '进行中' },
  { value: 'wait', label: '未开始' },
  { value: 'done', label: '已完成' },
  { value: 'closed', label: '已关闭' },
]

const MY_WORKBENCH_TASK_COLUMN_METAS: ColumnMeta[] = [
  { key: 'actions', title: '报工', defaultWidth: 88 },
  { key: 'id', title: 'ID', defaultWidth: 70 },
  { key: 'name', title: '任务名', defaultWidth: 180 },
  { key: 'status', title: '状态', defaultWidth: 100 },
  { key: 'estimate', title: '预估(h)', defaultWidth: 90 },
  { key: 'consumed', title: '消耗(h)', defaultWidth: 90 },
]

const MY_WORKBENCH_EFFORT_COLUMN_METAS: ColumnMeta[] = [
  { key: 'work_date', title: '日期', defaultWidth: 110 },
  { key: 'consumed', title: '耗时(h)', defaultWidth: 90 },
  { key: 'work', title: '工作内容', defaultWidth: 200 },
  { key: 'object_id', title: '任务', defaultWidth: 90 },
]

const statusColor: Record<string, string> = {
  doing: 'blue',
  wait: 'orange',
  done: 'green',
  closed: 'default',
}

function eventTouchesDay(d: Dayjs, startIso: string, endIso: string) {
  const lo = d.startOf('day')
  const hi = d.endOf('day')
  const es = dayjs(startIso)
  const ee = dayjs(endIso)
  return !ee.isBefore(lo) && !es.isAfter(hi)
}

function roundToHalfHour(x: number) {
  if (!Number.isFinite(x)) return 0
  return Math.round(x * 2) / 2
}

function sleep(ms: number) {
  return new Promise<void>((resolve) => {
    window.setTimeout(resolve, ms)
  })
}

function mergeEffortRows<T extends EffortLike>(rows: T[], next: T): T[] {
  const nextDate = String(next.work_date ?? '').slice(0, 10)
  const deduped = (rows ?? []).filter((item) => {
    if (next.id > 0 && item.id === next.id) return false
    return !(
      String(item.work_date ?? '').slice(0, 10) === nextDate &&
      Number(item.consumed ?? 0) === Number(next.consumed ?? 0) &&
      String(item.work ?? '').trim() === String(next.work ?? '').trim() &&
      String(item.object_type ?? '').trim() === String(next.object_type ?? '').trim() &&
      Number(item.object_id ?? 0) === Number(next.object_id ?? 0)
    )
  })
  return [next, ...deduped]
}

const MyWorkbenchPage: React.FC = () => {
  const navigate = useNavigate()
  const location = useLocation()
  const screens = Grid.useBreakpoint()
  const me = useAuthStore((s) => s.me)
  const systemUserId = me?.user?.id
  const bindingAccount = (me?.zentao_binding?.zentao_account ?? '').trim()
  const [zentaoBound, setZentaoBound] = useState<boolean | null>(null)
  const [weekStartsOn, setWeekStartsOn] = useState<WeekStartsOn>(readWeekStartsOn)

  const [status, setStatus] = useState<string>('doing')
  const [tasksPage, setTasksPage] = useState(1)
  const [tasksPageSize, setTasksPageSize] = useState(50)
  const [tasksTotal, setTasksTotal] = useState(0)
  const [tasksLoading, setTasksLoading] = useState(false)
  const [tasks, setTasks] = useState<TaskRow[]>([])

  const [tasksLastSyncedAt, setTasksLastSyncedAt] = useState<string | null>(null)
  const [syncIntervalMinutes, setSyncIntervalMinutes] = useState<number>(15)
  const [syncHintLoading, setSyncHintLoading] = useState(false)

  const [effortsLoading, setEffortsLoading] = useState(false)
  const [todayEfforts, setTodayEfforts] = useState<EffortRow[]>([])

  const today = useMemo(() => dayjs().format('YYYY-MM-DD'), [])
  const todayHours = useMemo(() => {
    return (todayEfforts ?? []).reduce((sum, e) => sum + Number(e.consumed ?? 0), 0)
  }, [todayEfforts])

  const [drawerOpen, setDrawerOpen] = useState(false)
  const [drawerTaskId, setDrawerTaskId] = useState<number | undefined>()
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()

  // --- 快速报工（Drawer）联动 ---
  const watchedTaskId = Form.useWatch<number | undefined>('task_id', form)
  const watchedWorkDate = Form.useWatch<Dayjs | null>('work_date', form)
  const watchedConsumed = Form.useWatch<number | undefined>('consumed', form)
  const watchedLeft = Form.useWatch<number | undefined>('left', form)

  const [quickDayHoursLoading, setQuickDayHoursLoading] = useState(false)
  const [quickDayHours, setQuickDayHours] = useState<number>(0)
  const [dailyStandardHours, setDailyStandardHours] = useState<number>(8)

  const workDateStr = useMemo(() => {
    if (!watchedWorkDate) return null
    const d = dayjs(watchedWorkDate)
    return d.isValid() ? d.format('YYYY-MM-DD') : null
  }, [watchedWorkDate])

  const selectedQuickTask = useMemo(() => {
    const tid = Number(watchedTaskId ?? 0)
    if (!Number.isFinite(tid) || tid <= 0) return null
    return tasks.find((t) => t.id === tid) ?? null
  }, [watchedTaskId, tasks])

  // 计算"剩余(h)"：标准工时 - 本次consumed - 今天已报工；下限为 0，并四舍五入到 0.5。
  const computedLeft = useMemo(() => {
    const consumedThis = Number(watchedConsumed ?? 0)
    if (!Number.isFinite(consumedThis)) return 0
    const raw = dailyStandardHours - consumedThis - quickDayHours
    const rounded = roundToHalfHour(raw)
    return Math.max(0, rounded)
  }, [dailyStandardHours, quickDayHours, watchedConsumed])
  
  // 以"任务剩余工时"是否为 0 来触发"任务将自动完成"横幅。
  const shouldWarnAutoComplete = useMemo(() => {
    if (!selectedQuickTask) return false
    const estimate = Number(selectedQuickTask.estimate ?? 0)
    const taskConsumed = Number(selectedQuickTask.consumed ?? 0)
    const consumedThis = Number(watchedConsumed ?? 0)
    const taskLeft = estimate - taskConsumed - consumedThis
    return estimate > 0 && Number.isFinite(taskLeft) && taskLeft <= 0
  }, [selectedQuickTask, watchedConsumed])

  const handleQuickFormValuesChange = useCallback(
    (_changed: any, all: any) => {
      const tid = Number(all?.task_id ?? NaN)
      const consumedInput = Number(all?.consumed ?? NaN)
      if (!Number.isFinite(tid) || tid <= 0) return
      if (!Number.isFinite(consumedInput)) return
      const t = tasks.find((x) => x.id === tid)
      if (!t) return

      const raw = dailyStandardHours - consumedInput - quickDayHours
      const nextLeft = Math.max(0, roundToHalfHour(raw))

      const cur = Number(form.getFieldValue('left') ?? 0)
      if (!Number.isFinite(cur) || Math.abs(cur - nextLeft) > 1e-9) {
        form.setFieldsValue({ left: nextLeft })
      }
    },
    [tasks, form, dailyStandardHours, quickDayHours],
  )

  useEffect(() => {
    const cur = Number(watchedLeft ?? 0)
    if (!Number.isFinite(cur) || Math.abs(cur - computedLeft) > 1e-9) {
      form.setFieldsValue({ left: computedLeft })
    }
  }, [computedLeft, watchedLeft, form])

  useEffect(() => {
    // 展示“当前日期已报工时数”：优先复用 todayHours，其它日期才额外拉取。
    if (!bindingAccount || !workDateStr) {
      setQuickDayHours(0)
      return
    }
    if (workDateStr === today) {
      setQuickDayHours(todayHours)
      return
    }

    let cancelled = false
    const run = async () => {
      setQuickDayHoursLoading(true)
      try {
        const res = await listEfforts({
          my_binding: 1,
          date_from: workDateStr,
          date_to: workDateStr,
          page: 1,
          page_size: 50,
        })
        const sum = (res?.data ?? []).reduce((s: number, e: any) => s + Number(e?.consumed ?? 0), 0)
        if (cancelled) return
        setQuickDayHours(sum)
      } catch {
        if (cancelled) return
        setQuickDayHours(0)
      } finally {
        if (cancelled) return
        setQuickDayHoursLoading(false)
      }
    }
    void run()
    return () => {
      cancelled = true
    }
  }, [bindingAccount, workDateStr, today, todayHours])

  const [calPanel, setCalPanel] = useState<Dayjs>(() => dayjs())
  const [selectedDay, setSelectedDay] = useState<Dayjs>(() => dayjs())
  const [aggLoading, setAggLoading] = useState(false)
  const [aggregate, setAggregate] = useState<CalendarAggregate | null>(null)
  const [dayDetailModalOpen, setDayDetailModalOpen] = useState(false)

  const taskOptions = useMemo(() => {
    return (tasks ?? []).map((t) => ({ value: t.id, label: `${t.id} · ${t.name}` }))
  }, [tasks])

  const taskNameById = useMemo(() => {
    const m = new Map<number, string>()
    for (const t of tasks) m.set(t.id, t.name)
    for (const e of aggregate?.external ?? []) {
      if (e.source_type === 'task' && e.source_id > 0 && e.title) {
        m.set(e.source_id, e.title)
      }
    }
    return m
  }, [tasks, aggregate])

  const refreshAuth = async () => {
    try {
      const r = await getZentaoAuthStatus()
      setZentaoBound(!!r?.bound)
    } catch {
      setZentaoBound(null)
    }
  }

  const refreshTasks = async (account: string, page: number, page_size: number, silent = false) => {
    if (!account) {
      setTasks([])
      setTasksTotal(0)
      return
    }
    setTasksLoading(true)
    try {
      const res = await listTasks({
        my_binding: 1,
        status,
        page,
        page_size,
      })
      setTasks(res?.data ?? [])
      const total = typeof res?.total === 'number' ? res.total : Array.isArray(res?.data) ? res.data.length : 0
      setTasksTotal(total)
    } catch (e: any) {
      if (!silent) message.error(e.response?.data?.error ?? '加载任务失败')
      setTasks([])
      setTasksTotal(0)
    } finally {
      setTasksLoading(false)
    }
  }

  const refreshTodayEfforts = async (account: string, silent = false) => {
    if (!account) {
      setTodayEfforts([])
      return
    }
    setEffortsLoading(true)
    try {
      const res = await listEfforts({
        my_binding: 1,
        date_from: today,
        date_to: today,
        page: 1,
        page_size: 50,
      })
      setTodayEfforts(res?.data ?? [])
    } catch (e: any) {
      if (!silent) message.error(e.response?.data?.error ?? '加载今日报工失败')
      setTodayEfforts([])
    } finally {
      setEffortsLoading(false)
    }
  }

  const refreshAggregate = useCallback(async (silent = false) => {
    const from = calPanel.startOf('month').format('YYYY-MM-DD')
    const to = calPanel.endOf('month').format('YYYY-MM-DD')
    setAggLoading(true)
    try {
      const r = await getCalendarAggregate({ date_from: from, date_to: to })
      setAggregate(r)
    } catch (e: any) {
      if (!silent) message.error(e.response?.data?.error ?? '加载日历数据失败')
      setAggregate(null)
    } finally {
      setAggLoading(false)
    }
  }, [calPanel])

  const getDailyHours = useCallback(
    (date: Dayjs) => {
      const key = date.format('YYYY-MM-DD')
      if (key === today) return todayHours
      if (!aggregate?.efforts?.length) return 0
      return (aggregate.efforts ?? [])
        .filter((e) => dayjs(e.work_date).format('YYYY-MM-DD') === key)
        .reduce((s, e) => s + Number(e.consumed ?? 0), 0)
    },
    [aggregate, today, todayHours],
  )

  const cellDots = useCallback(
    (date: Dayjs) => {
      if (!aggregate) return { n: 0, colors: [] as string[] }
      const isTodayCell = date.isSame(today, 'day')
      const effortRows = isTodayCell
        ? (todayEfforts ?? [])
        : (aggregate.efforts ?? []).filter((e) => dayjs(e.work_date).isSame(date, 'day'))
      const taskPlanRows = (aggregate.external ?? []).filter(
        (x) => x.source_type === 'task' && eventTouchesDay(date, x.start, x.end),
      )
      const externalRows = (aggregate.external ?? []).filter(
        (x) => x.source_type !== 'task' && eventTouchesDay(date, x.start, x.end),
      )

      const colors: string[] = []
      if (effortRows.length > 0) colors.push(CALENDAR_CATEGORY_COLORS.effort)
      if (taskPlanRows.length > 0) colors.push(CALENDAR_CATEGORY_COLORS.taskPlan)
      if (externalRows.length > 0) colors.push(CALENDAR_CATEGORY_COLORS.external)

      return { n: colors.length, colors }
    },
    [aggregate, today, todayEfforts],
  )

  const dayDetail = useMemo(() => {
    const isToday = selectedDay.format('YYYY-MM-DD') === today

    // 右侧报工列表优先展示“今天”的实时数据（todayEfforts），避免 calendar-aggregate
    // 在 ETL 同步/刷新竞态时返回旧数据导致“点今天却看不到报工”。
    const efforts = isToday
      ? (todayEfforts ?? [])
      : (aggregate?.efforts ?? []).filter((e) => dayjs(e.work_date).isSame(selectedDay, 'day'))

    const external = (aggregate?.external ?? []).filter((x) => eventTouchesDay(selectedDay, x.start, x.end))
    return { efforts, external }
  }, [aggregate, selectedDay, today, todayEfforts])

  const taskPlanEvents = dayDetail.external.filter((e) => e.source_type === 'task')
  const otherCalendarEvents = dayDetail.external.filter((e) => e.source_type !== 'task')
  const desktopLayout = !!screens.xl
  const rightPaneReferenceHeight = desktopLayout ? (screens.xxl ? 860 : 760) : undefined
  const todayPanelHeight = rightPaneReferenceHeight
    ? Math.round((rightPaneReferenceHeight - 12) * 0.4)
    : undefined
  const todayTableScrollY = todayPanelHeight ? Math.max(180, todayPanelHeight - 128) : undefined
  const rightPaneCardBodyPadding = 12

  useEffect(() => {
    void refreshAuth()
  }, [])

  useEffect(() => {
    getBusinessConfig()
      .then((d: any) => {
        if (typeof d?.daily_standard_hours === 'number') {
          setDailyStandardHours(d.daily_standard_hours)
        }
      })
      .catch(() => {})
  }, [])

  useEffect(() => {
    let cancelled = false
    const fetchSyncHint = async () => {
      setSyncHintLoading(true)
      try {
        const [tables, sync] = await Promise.all([
          getSyncStatus().then((d: { tables: Record<string, SyncInfo> }) => d.tables ?? {}),
          getSyncSettings().then((d: { interval_minutes: number }) => d),
        ])
        if (cancelled) return
        const lastAt = tables?.local_tasks?.updated_at ?? null
        setTasksLastSyncedAt(typeof lastAt === 'string' ? lastAt : null)
        if (typeof sync?.interval_minutes === 'number') setSyncIntervalMinutes(sync.interval_minutes)
      } catch {
        if (cancelled) return
        setTasksLastSyncedAt(null)
      } finally {
        if (cancelled) return
        setSyncHintLoading(false)
      }
    }
    void fetchSyncHint()
    return () => {
      cancelled = true
    }
  }, [])

  const nextTasksSyncTimeText = useMemo(() => {
    if (syncHintLoading) return '计算中...'
    if (!tasksLastSyncedAt) return '—'
    if (!(syncIntervalMinutes > 0)) return '—'
    const last = dayjs(tasksLastSyncedAt)
    if (!last.isValid()) return '—'

    const now = dayjs()
    let next = last.add(syncIntervalMinutes, 'minute')
    // 如果上次同步时间过早导致“下次”已经落后，则按间隔继续向后推到未来
    let guard = 0
    while ((next.isBefore(now) || next.isSame(now)) && guard < 1000) {
      next = next.add(syncIntervalMinutes, 'minute')
      guard++
    }
    return next.format('YYYY-MM-DD HH:mm')
  }, [syncHintLoading, tasksLastSyncedAt, syncIntervalMinutes])

  useEffect(() => {
    const sp = new URLSearchParams(location.search)
    const raw = sp.get('task_id')
    const n = raw ? Number(raw) : NaN
    if (Number.isFinite(n) && n > 0) {
      openDrawer(Math.trunc(n))
    }
  }, [location.search])

  useEffect(() => {
    void refreshTasks(bindingAccount, tasksPage, tasksPageSize)
  }, [bindingAccount, status, tasksPage, tasksPageSize])

  useEffect(() => {
    if (!bindingAccount) return
    void refreshTodayEfforts(bindingAccount)
  }, [bindingAccount])

  useEffect(() => {
    void refreshAggregate()
  }, [refreshAggregate, bindingAccount])

  const openDrawer = (taskId?: number) => {
    setDrawerTaskId(taskId)
    setDrawerOpen(true)

    const consumedDefault = 1
    const nextLeft = roundToHalfHour(dailyStandardHours - consumedDefault - quickDayHours)

    form.setFieldsValue({
      task_id: taskId,
      work_date: dayjs(),
      consumed: consumedDefault,
      left: Math.max(0, nextLeft),
      work: '',
    })
  }

  const applyOptimisticEffort = useCallback((payload: {
    task_id: number
    work_date?: string
    work: string
    consumed: number
    left: number
  }, effortId?: number) => {
    const workDate = payload.work_date || today
    const taskId = Number(payload.task_id)
    const optimisticRow: EffortRow = {
      id: typeof effortId === 'number' && effortId > 0 ? effortId : -Date.now(),
      work_date: workDate,
      consumed: Number(payload.consumed ?? 0),
      work: String(payload.work ?? ''),
      object_type: 'task',
      object_id: taskId,
      object_title: tasks.find((t) => t.id === taskId)?.name,
    }

    if (workDate === today) {
      setTodayEfforts((prev) => mergeEffortRows(prev, optimisticRow))
    }
    setAggregate((prev) => {
      if (!prev) return prev
      return {
        ...prev,
        efforts: mergeEffortRows(prev.efforts, optimisticRow),
      }
    })
    setTasks((prev) => prev.map((task) => {
      if (task.id !== optimisticRow.object_id) return task
      return {
        ...task,
        consumed: Number(task.consumed ?? 0) + optimisticRow.consumed,
      }
    }))
  }, [today, tasks])

  const refreshAfterEffortSubmit = useCallback(async (account: string, page: number, pageSize: number) => {
    await Promise.allSettled([
      refreshTodayEfforts(account, true),
      refreshAggregate(true),
      refreshTasks(account, page, pageSize, true),
    ])
    await sleep(1200)
    await Promise.allSettled([
      refreshTodayEfforts(account, true),
      refreshAggregate(true),
      refreshTasks(account, page, pageSize, true),
    ])
    await sleep(2500)
    await Promise.allSettled([
      refreshTodayEfforts(account, true),
      refreshAggregate(true),
      refreshTasks(account, page, pageSize, true),
    ])
  }, [refreshAggregate])

  const handleSubmit = async () => {
    if (!bindingAccount || !zentaoBound) {
      message.error('禅道未授权或会话已过期，或未完成账号绑定，无法提交报工')
      return
    }
    const v = await form.validateFields()
    setSubmitting(true)
    try {
      const payload = {
        task_id: Number(v.task_id),
        work_date: v.work_date ? dayjs(v.work_date).format('YYYY-MM-DD') : undefined,
        work: String(v.work ?? ''),
        consumed: v.consumed,
        left: v.left,
      }
      const r = await createZentaoEffort(payload)
      if (r?.ok === false) {
        showEffortErrorModal(r)
      } else {
        message.success('已提交到禅道')
        applyOptimisticEffort(payload, typeof r?.effort_id === 'number' ? r.effort_id : undefined)
        setDrawerOpen(false)
        void refreshAfterEffortSubmit(bindingAccount, tasksPage, tasksPageSize)
      }
    } catch (e: any) {
      const data = e.response?.data
      if (data && typeof data === 'object') {
        showEffortErrorModal(data)
      } else {
        message.error(data?.error ?? '提交失败')
      }
    } finally {
      setSubmitting(false)
    }
  }

  const showEffortErrorModal = (payload: any) => {
    const mode = payload?.mode ?? '-'
    const apiStatus = payload?.api_status
    const apiBody = payload?.api_body
    const apiError = payload?.api_error
    const webformError = payload?.webform_error
    const hint = payload?.hint
    const attempts: any[] = Array.isArray(payload?.attempts) ? payload.attempts : []
    const resultJSON = payload?.result ? JSON.stringify(payload.result, null, 2) : null
    Modal.error({
      title: '报工提交失败',
      width: 760,
      content: (
        <div style={{ maxHeight: 520, overflow: 'auto' }}>
          <p><b>{payload?.error ?? '提交失败'}</b></p>
          <p style={{ marginTop: 8 }}>
            <Tag color="geekblue">mode: {mode}</Tag>
            {typeof apiStatus === 'number' && <Tag color="volcano">API status: {apiStatus}</Tag>}
          </p>
          {hint && <Alert type="info" showIcon message={hint} style={{ marginTop: 8 }} />}
          {attempts.length > 0 && (
            <div style={{ marginTop: 12 }}>
              <Text style={{ color: 'var(--zm-text-muted)', fontSize: 12 }}>API variant 尝试明细（按顺序）：</Text>
              <div style={{ marginTop: 6, display: 'flex', flexDirection: 'column', gap: 6 }}>
                {attempts.map((a, idx) => (
                  <div key={idx} style={{ background: '#f6f8fa', padding: 8, borderRadius: 4, fontSize: 12 }}>
                    <div style={{ display: 'flex', gap: 6, alignItems: 'center', flexWrap: 'wrap' }}>
                      <Tag color={a.ok ? 'green' : 'red'}>{a.variant}</Tag>
                      {typeof a.api_status === 'number' && <Tag color="volcano">HTTP {a.api_status}</Tag>}
                      {typeof a.task_consumed_after === 'number' && (
                        <Tag color={a.task_consumed_after > 0 ? 'cyan' : 'default'}>
                          task.consumed = {a.task_consumed_after}
                        </Tag>
                      )}
                      {a.verify_attempted === true && (
                        <Tag color={a.verify_matched ? 'green' : 'red'}>
                          {a.verify_matched ? 'GET 列表已确认' : 'GET 列表未找到'}
                        </Tag>
                      )}
                      {a.reason && <Tag color="orange">{a.reason}</Tag>}
                    </div>
                    {a.used_url && <div style={{ marginTop: 4, color: '#666', wordBreak: 'break-all' }}>URL: {a.used_url}</div>}
                    {a.verify_error && <div style={{ marginTop: 4, color: '#a8071a' }}>verify error: {a.verify_error}</div>}
                    {a.error && <div style={{ marginTop: 4, color: '#a8071a' }}>error: {a.error}</div>}
                    {a.api_body && (
                      <pre style={{ marginTop: 4, fontSize: 12, whiteSpace: 'pre-wrap', margin: 0 }}>{a.api_body}</pre>
                    )}
                  </div>
                ))}
              </div>
            </div>
          )}
          {apiBody && (
            <div style={{ marginTop: 12 }}>
              <Text style={{ color: 'var(--zm-text-muted)', fontSize: 12 }}>禅道 API 响应体：</Text>
              <pre style={{ background: '#f6f8fa', padding: 8, borderRadius: 4, fontSize: 12, whiteSpace: 'pre-wrap' }}>{typeof apiBody === 'string' ? apiBody : JSON.stringify(apiBody, null, 2)}</pre>
            </div>
          )}
          {apiError && !apiBody && (
            <div style={{ marginTop: 12 }}>
              <Text style={{ color: 'var(--zm-text-muted)', fontSize: 12 }}>API 错误详情：</Text>
              <pre style={{ background: '#f6f8fa', padding: 8, borderRadius: 4, fontSize: 12, whiteSpace: 'pre-wrap' }}>{apiError}</pre>
            </div>
          )}
          {webformError && (
            <div style={{ marginTop: 12 }}>
              <Text style={{ color: 'var(--zm-text-muted)', fontSize: 12 }}>Webform 兜底错误：</Text>
              <pre style={{ background: '#f6f8fa', padding: 8, borderRadius: 4, fontSize: 12, whiteSpace: 'pre-wrap' }}>{webformError}</pre>
            </div>
          )}
          {resultJSON && (
            <div style={{ marginTop: 12 }}>
              <Text style={{ color: 'var(--zm-text-muted)', fontSize: 12 }}>原始 result：</Text>
              <pre style={{ background: '#f6f8fa', padding: 8, borderRadius: 4, fontSize: 12, whiteSpace: 'pre-wrap' }}>{resultJSON}</pre>
            </div>
          )}
        </div>
      ),
    })
  }

  const allTaskColumns = useMemo(
    () => [
      {
        key: 'actions',
        title: '',
        width: 88,
        render: (_: unknown, r: TaskRow) => (
          <Button
            type="primary"
            size="small"
            onClick={() => openDrawer(r.id)}
            style={{ background: 'var(--zm-brand-gradient)', border: 'none' }}
          >
            报工
          </Button>
        ),
      },
      { key: 'id', title: 'ID', dataIndex: 'id', width: 70 },
      {
        key: 'name',
        title: '任务名',
        dataIndex: 'name',
        width: 180,
        render: (v: string, r: TaskRow) => (
          <Link to={myWorkbenchTaskDetailHref(r.id)} style={{ color: 'var(--zm-text-primary)' }}>
            {v}
          </Link>
        ),
      },
      {
        key: 'status',
        title: '状态',
        dataIndex: 'status',
        width: 100,
        render: (v: string) => <Tag color={statusColor[v] ?? 'default'}>{v}</Tag>,
      },
      { key: 'estimate', title: '预估(h)', dataIndex: 'estimate', width: 90 },
      { key: 'consumed', title: '消耗(h)', dataIndex: 'consumed', width: 90 },
    ],
    [openDrawer],
  )

  const allEffortColumns = useMemo(
    () => [
      {
        key: 'work_date',
        title: '日期',
        dataIndex: 'work_date',
        width: 110,
        render: (v: string) => (v ? dayjs(v).format('YYYY-MM-DD') : '-'),
      },
      { key: 'consumed', title: '耗时(h)', dataIndex: 'consumed', width: 90 },
      {
        key: 'work',
        title: '工作内容',
        dataIndex: 'work',
        render: (v: string) => <Text style={{ color: 'var(--zm-text-secondary)' }}>{v}</Text>,
      },
      { key: 'object_id', title: '任务', dataIndex: 'object_id', width: 90 },
    ],
    [],
  )

  const {
    columns: taskColumns,
    prefs: taskColumnPrefs,
    setPrefs: setTaskColumnPrefs,
    resetPrefs: resetTaskColumnPrefs,
    metas: taskColumnMetas,
  } = useTableColumnPrefs(systemUserId, 'my-workbench.tasks', allTaskColumns, MY_WORKBENCH_TASK_COLUMN_METAS)

  const {
    columns: effortColumns,
    prefs: effortColumnPrefs,
    setPrefs: setEffortColumnPrefs,
    resetPrefs: resetEffortColumnPrefs,
    metas: effortColumnMetas,
  } = useTableColumnPrefs(
    systemUserId,
    'my-workbench.today-efforts',
    allEffortColumns,
    MY_WORKBENCH_EFFORT_COLUMN_METAS,
  )

  const renderEffortList = (rows: EffortLike[]) => {
    if (rows.length === 0) {
      return <div style={{ marginTop: 6, color: 'var(--zm-text-muted)', fontSize: 12 }}>无记录</div>
    }
    return (
      <List
        size="small"
        style={{ marginTop: 6 }}
        dataSource={rows}
        renderItem={(it) => {
          const isTask = String(it.object_type ?? '').trim().toLowerCase() === 'task'
          const taskId = isTask ? Number(it.object_id ?? 0) : 0
          const taskName =
            it.object_title?.trim() ||
            (taskId > 0 ? taskNameById.get(taskId) : undefined) ||
            (taskId > 0 ? `#${taskId}` : '')
          return (
            <List.Item style={{ padding: '6px 0' }}>
              <div>
                <Tag color="blue">{Number(it.consumed ?? 0).toFixed(1)}h</Tag>
                <Text style={{ fontSize: 13 }}>{it.work}</Text>
                {taskId > 0 ? (
                  <div style={{ marginTop: 2, fontSize: 12 }}>
                    <Text type="secondary">任务：</Text>
                    <Link
                      to={myWorkbenchTaskDetailHref(taskId)}
                      style={{ color: 'var(--zm-text-primary)' }}
                      onClick={() => setDayDetailModalOpen(false)}
                    >
                      {taskName}
                    </Link>
                  </div>
                ) : null}
              </div>
            </List.Item>
          )
        }}
      />
    )
  }

  const renderExternalEventList = (rows: CalendarExternalEvent[], emptyText: string) => {
    if (rows.length === 0) {
      return <div style={{ marginTop: 6, color: 'var(--zm-text-muted)', fontSize: 12 }}>{emptyText}</div>
    }
    return (
      <List
        size="small"
        style={{ marginTop: 6 }}
        dataSource={rows}
        renderItem={(it) => (
          <List.Item style={{ padding: '6px 0' }}>
            <div style={{ display: 'flex', gap: 8, alignItems: 'flex-start' }}>
              <span
                style={{
                  width: 8,
                  marginTop: 6,
                  flexShrink: 0,
                  height: 8,
                  borderRadius: 2,
                  background: getCalendarEventDisplayColor(it),
                }}
              />
              <div style={{ flex: 1, minWidth: 0 }}>
                {it.source_type === 'task' && it.source_id > 0 ? (
                  <Link
                    to={myWorkbenchTaskDetailHref(it.source_id)}
                    style={{ fontSize: 13, color: 'var(--zm-text-primary)' }}
                    onClick={() => setDayDetailModalOpen(false)}
                  >
                    {it.title}
                  </Link>
                ) : (
                  <Text style={{ fontSize: 13 }}>{it.title}</Text>
                )}
                <div style={{ marginTop: 2 }}>
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    {it.source_name}
                    {it.all_day
                      ? ' · 全天'
                      : ` · ${dayjs(it.start).format('HH:mm')}–${dayjs(it.end).format('HH:mm')}`}
                  </Text>
                </div>
              </div>
            </div>
          </List.Item>
        )}
      />
    )
  }

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}>
        <div>
          <span style={{ display: 'inline-flex', alignItems: 'center' }}>
            <Text style={{ color: 'var(--zm-text-primary)', fontSize: 18, fontWeight: 600 }}>我的工作台</Text>
            <MyWorkbenchSettings
              weekStartsOn={weekStartsOn}
              onWeekStartsOnChange={(v) => {
                setWeekStartsOn(v)
                saveWeekStartsOn(v)
              }}
            />
          </span>
          <div style={{ marginTop: 6 }}>
            <Text style={{ color: 'var(--zm-text-muted)', fontSize: 12 }}>
              今日已报工 <b>{todayHours.toFixed(1)}</b> 小时
            </Text>
          </div>
        </div>
        <Space wrap>
          <Button type="primary" onClick={() => openDrawer(drawerTaskId)}
            style={{ background: 'var(--zm-brand-gradient)', border: 'none' }}>
            快速报工
          </Button>
        </Space>
      </div>

      <div style={{ height: 12 }} />

      {zentaoBound === false ? (
        <Alert
          type="warning"
          showIcon
          message="禅道未授权或会话已过期"
          description={<span>请先到「禅道授权」完成绑定后再提交报工。</span>}
          action={<Button size="small" onClick={() => navigate('/zentao-auth')}>去授权</Button>}
        />
      ) : null}

      <div style={{ height: 12 }} />

      <div
        style={{
          display: 'grid',
          gridTemplateColumns: desktopLayout ? 'minmax(0, 7fr) minmax(320px, 3fr)' : '1fr',
          gap: 12,
          alignItems: 'start',
          minWidth: 0,
        }}
      >
        <Card
          title={<Text style={{ color: 'var(--zm-text-primary)' }}>日历</Text>}
          styles={{
            header: { borderBottom: '1px solid var(--zm-border-subtle)' },
          }}
          style={{
            background: 'var(--zm-bg-surface)',
            border: '1px solid var(--zm-border-subtle)',
            borderRadius: 12,
            minWidth: 0,
          }}
          extra={
            <Space>
              <Button size="small" loading={aggLoading} onClick={() => void refreshAggregate()}>
                刷新日历
              </Button>
            </Space>
          }
        >
          {aggregate?.account_errors && aggregate.account_errors.length > 0 ? (
            <Alert
              type="warning"
              showIcon
              style={{ marginBottom: 12 }}
              message="部分日历账户未能拉取"
              description={
                <ul style={{ margin: 0, paddingLeft: 18 }}>
                  {aggregate.account_errors.map((ae, i) => (
                    <li key={i}>
                      {ae.username} ({ae.type}): {ae.error}
                    </li>
                  ))}
                </ul>
              }
            />
          ) : null}

          {aggregate?.feed_errors && aggregate.feed_errors.length > 0 ? (
            <Alert
              type="warning"
              showIcon
              style={{ marginBottom: 12 }}
              message="部分订阅未能拉取"
              description={
                <ul style={{ margin: 0, paddingLeft: 18 }}>
                  {aggregate.feed_errors.map((fe, i) => (
                    <li key={i}>
                      {fe.feed_name ?? fe.feed_id}: {fe.error}
                    </li>
                  ))}
                </ul>
              }
            />
          ) : null}

          <div>
            <MacMonthCalendar
              month={calPanel}
              selectedDay={selectedDay}
              events={aggregate?.external ?? []}
              loading={aggLoading}
              getCellDots={cellDots}
              getDailyHours={getDailyHours}
              heatmapMaxHours={dailyStandardHours}
              getTaskDetailHref={myWorkbenchTaskDetailHref}
              weekStartsOn={weekStartsOn}
              onMonthChange={(d) => {
                setCalPanel(d)
                setSelectedDay((prev) => (prev.isSame(d, 'month') ? prev : d.startOf('month')))
              }}
              onSelectDay={(d) => {
                setSelectedDay(d)
                setCalPanel(d)
                setDayDetailModalOpen(true)
              }}
            />
            <div style={{ marginTop: 10, color: 'var(--zm-text-muted)', fontSize: 12 }}>
              选中日：外部事件 <b>{dayDetail.external.length}</b> 条 · 报工 <b>{dayDetail.efforts.length}</b> 条
              {(() => {
                const { n } = cellDots(selectedDay)
                return n > 0 ? <span> · 合计 <b>{n}</b></span> : null
              })()}
              <span> · 单击日期查看详情</span>
            </div>
          </div>
        </Card>

        <div
          style={{
            display: 'grid',
            gridTemplateRows: 'auto auto',
            gap: 12,
            minWidth: 0,
          }}
        >
          <Card
            title={<Text style={{ color: 'var(--zm-text-primary)' }}>今日报工</Text>}
            styles={{
              header: { borderBottom: '1px solid var(--zm-border-subtle)' },
              body: desktopLayout
                ? {
                    padding: rightPaneCardBodyPadding,
                    display: 'flex',
                    flexDirection: 'column',
                    height: '100%',
                    overflow: 'hidden',
                    minHeight: 0,
                  }
                : { padding: rightPaneCardBodyPadding },
            }}
            style={{
              background: 'var(--zm-bg-surface)',
              border: '1px solid var(--zm-border-subtle)',
              borderRadius: 12,
              height: todayPanelHeight,
              minWidth: 0,
            }}
            extra={
              <Space size="small">
                <TableColumnSettings
                  metas={effortColumnMetas}
                  prefs={effortColumnPrefs}
                  onChange={setEffortColumnPrefs}
                  onReset={resetEffortColumnPrefs}
                />
                <Button onClick={() => void refreshTodayEfforts(bindingAccount)}>刷新</Button>
              </Space>
            }
          >
            <Table
              rowKey="id"
              size="small"
              tableLayout="fixed"
              loading={effortsLoading}
              dataSource={todayEfforts}
              components={resizableTableComponents}
              columns={effortColumns as any}
              pagination={false}
              scroll={{ x: 520, ...(todayTableScrollY ? { y: todayTableScrollY } : {}) }}
            />
          </Card>

          <Card
            title={
              <div style={{ display: 'flex', flexDirection: 'column', gap: 2, minWidth: 0 }}>
                <Text style={{ color: 'var(--zm-text-primary)' }}>我的任务</Text>
                <Text
                  style={{
                    fontSize: 11,
                    lineHeight: 1.4,
                    color: 'var(--zm-text-disabled)',
                    wordBreak: 'break-word',
                  }}
                >
                  任务信息需要等待后台数据同步，下次同步时间：{nextTasksSyncTimeText}
                </Text>
              </div>
            }
            styles={{
              header: { borderBottom: '1px solid var(--zm-border-subtle)' },
              body: { padding: rightPaneCardBodyPadding },
            }}
            style={{
              background: 'var(--zm-bg-surface)',
              border: '1px solid var(--zm-border-subtle)',
              borderRadius: 12,
              minWidth: 0,
            }}
            extra={
              <Space size="small">
                <TableColumnSettings
                  metas={taskColumnMetas}
                  prefs={taskColumnPrefs}
                  onChange={setTaskColumnPrefs}
                  onReset={resetTaskColumnPrefs}
                />
                <Button onClick={() => void refreshTasks(bindingAccount, tasksPage, tasksPageSize)}>刷新</Button>
              </Space>
            }
          >
            <Tabs
              size="small"
              activeKey={status}
              onChange={(k) => {
                const next = String(k)
                setStatus(next)
                setTasksPage(1)
              }}
              items={STATUS_OPTIONS.map((o) => ({
                key: o.value,
                label: o.label,
                children: <div style={{ display: 'none' }} />,
              }))}
            />
            <div style={{ height: 8 }} />
            <Table
              rowKey="id"
              size="small"
              tableLayout="fixed"
              loading={tasksLoading}
              dataSource={tasks}
              components={resizableTableComponents}
              columns={taskColumns as any}
              pagination={{
                current: tasksPage,
                pageSize: tasksPageSize,
                total: tasksTotal,
                showSizeChanger: false,
                showTotal: (t) => `共 ${t} 条`,
                onChange: (p) => setTasksPage(p),
              }}
              scroll={{ x: 720 }}
            />
          </Card>
        </div>
      </div>

      <Modal
        open={dayDetailModalOpen}
        onCancel={() => setDayDetailModalOpen(false)}
        footer={null}
        width={760}
        title={`日期详情 · ${selectedDay.format('YYYY-MM-DD')}`}
      >
        <div style={{ maxHeight: '70vh', overflow: 'auto', paddingRight: 4 }}>
          <Text type="secondary" style={{ fontSize: 12 }}>
            禅道报工
          </Text>
          {renderEffortList(dayDetail.efforts)}
          <Divider style={{ margin: '12px 0' }} />
          <Text type="secondary" style={{ fontSize: 12 }}>
            任务计划
          </Text>
          {renderExternalEventList(taskPlanEvents, '无任务计划')}
          <Divider style={{ margin: '12px 0' }} />
          <Text type="secondary" style={{ fontSize: 12 }}>
            外部日历事件
          </Text>
          {renderExternalEventList(otherCalendarEvents, '无事件')}
        </div>
      </Modal>

      <Drawer
        title="快捷报工"
        open={drawerOpen}
        onClose={() => setDrawerOpen(false)}
        width={520}
        destroyOnClose
        extra={
          <Space>
            <Button onClick={() => setDrawerOpen(false)}>取消</Button>
            <Button type="primary" loading={submitting} onClick={handleSubmit} disabled={!zentaoBound}
              style={{ background: 'var(--zm-brand-gradient)', border: 'none' }}>
              提交到禅道
            </Button>
          </Space>
        }
      >
        {zentaoBound === false ? (
          <>
            <Alert
              type="warning"
              showIcon
              message="禅道未授权或会话已过期"
              description="请先到「禅道授权」完成绑定，再回来提交（已填写内容会保留在当前抽屉中）。"
              action={<Button size="small" onClick={() => navigate('/zentao-auth')}>去授权</Button>}
              style={{ marginBottom: 12 }}
            />
          </>
        ) : null}
        {shouldWarnAutoComplete ? (
          <Alert
            type="warning"
            showIcon
            style={{ marginBottom: 12 }}
            message="本次提交后任务将自动完成"
          />
        ) : null}
        <Form form={form} layout="vertical" onValuesChange={handleQuickFormValuesChange}>
          <div style={{ marginBottom: 12, color: 'var(--zm-text-muted)', fontSize: 12 }}>
            当前日期已报工{' '}
            <b>{quickDayHoursLoading ? '计算中…' : `${quickDayHours.toFixed(1)}h`}</b>
          </div>
          <Form.Item
            name="task_id"
            label="任务"
            rules={[{ required: true, message: '请选择任务' }]}
          >
            <Select
              options={taskOptions}
              placeholder="请选择任务"
              showSearch
              optionFilterProp="label"
              onChange={(v) => {
                const tid = Number(v)
                setDrawerTaskId(tid)
                form.setFieldsValue({ task_id: tid })
              }}
            />
          </Form.Item>
          <Form.Item name="work_date" label="日期">
            <DatePicker style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item
            name="work"
            label="工作内容"
            rules={[{ required: true, message: '请输入工作内容' }]}
          >
            <Input.TextArea rows={4} placeholder="例如：开发任务 X，完成接口联调与自测" />
          </Form.Item>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
            <Form.Item
              name="consumed"
              label="耗时(h)"
              rules={[{ required: true, message: '请输入耗时' }]}
            >
              <InputNumber min={0} step={0.5} style={{ width: '100%' }} />
            </Form.Item>
            <Form.Item
              name="left"
              label="剩余(h)"
              rules={[{ required: true, message: '请输入剩余工时' }]}
            >
              <InputNumber min={0} step={0.5} style={{ width: '100%' }} disabled={selectedQuickTask == null} />
            </Form.Item>
          </div>
        </Form>
      </Drawer>

    </div>
  )
}

export default MyWorkbenchPage

