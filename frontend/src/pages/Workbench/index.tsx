import React, { useState, useEffect, useLayoutEffect, useCallback, useMemo, useRef } from 'react'
import { Link } from 'react-router-dom'
import { Tabs, Table, Space, Button, Select, DatePicker, Modal, Typography,
  Tag, Tooltip, message, Segmented, Input, Spin } from 'antd'
import { EyeOutlined, SearchOutlined } from '@ant-design/icons'
import JsonView from '@uiw/react-json-view'
import { listTasks, listStories, listBugs, listEfforts, listExecutions,
  getGroupMembers, listProjects, getWorkbenchProject } from '../../api'
import type { WorkbenchParams, WorkbenchProjectDetail } from '../../api'
import dayjs, { Dayjs } from 'dayjs'
import { GroupSelect } from '../../components/GroupSelect'
import { WorkbenchStructureSelect } from '../../components/WorkbenchStructureSelect'
import { useAuthStore } from '../../store/auth'
import {
  taskTypeLabel,
  taskStatusLabel,
  storyStatusLabel,
  STORY_STATUS_TAG_COLOR,
  bugStatusLabel,
  BUG_STATUS_TAG_COLOR,
  bugResolutionLabel,
  executionStatusLabel,
  EXECUTION_STATUS_TAG_COLOR,
  useMemberPersonDisplay,
} from './workbenchDisplay'

const { RangePicker } = DatePicker
const { Text } = Typography

type WorkbenchScope = 'group' | 'all'

type WorkbenchGroupCtx = {
  scope: WorkbenchScope
  groupId: number | undefined
  groupName: string
  structureKey?: string
  structureMeta?: { type: string; id: number }
  setGroup: (id: number | undefined, name: string) => void
  setScope: (s: WorkbenchScope) => void
  setStructure: (key: string | undefined, meta?: { type: string; id: number }) => void
}

const WorkbenchGroupContext = React.createContext<WorkbenchGroupCtx | null>(null)
function useWorkbenchGroup() {
  const ctx = React.useContext(WorkbenchGroupContext)
  if (!ctx) throw new Error('WorkbenchGroupContext missing')
  return ctx
}

/** 当前小组下可选迭代（与「迭代」Tab 同源接口） */
function useExecutionOptions(groupId: number | undefined) {
  const [options, setOptions] = useState<{ value: number; label: string }[]>([])
  useEffect(() => {
    let cancelled = false
    listExecutions({ group_id: groupId ?? undefined, page: 1, page_size: 200 })
      .then((res) => {
        if (cancelled) return
        const rows = res.data ?? []
        setOptions(
          rows.map((e: { id: number; name: string }) => ({
            value: e.id,
            label: e.name ? `${e.id} · ${e.name}` : String(e.id),
          })),
        )
      })
      .catch(() => {
        if (!cancelled) setOptions([])
      })
    return () => {
      cancelled = true
    }
  }, [groupId])
  return options
}

/** 当前小组成员（账号 + 姓名），用于「按人员」筛选 */
function useGroupMemberOptions(groupId: number | undefined) {
  const [options, setOptions] = useState<{ value: string; label: string }[]>([])
  useEffect(() => {
    if (!groupId) {
      setOptions([])
      return
    }
    let cancelled = false
    getGroupMembers(groupId)
      .then((d: { members?: { account: string; realname: string }[] }) => {
        if (cancelled) return
        const rows = d.members ?? []
        setOptions(
          rows.map((m) => ({
            value: m.account,
            label: m.realname?.trim()
              ? `${m.realname.trim()}（${m.account}）`
              : m.account,
          })),
        )
      })
      .catch(() => {
        if (!cancelled) setOptions([])
      })
    return () => {
      cancelled = true
    }
  }, [groupId])
  return options
}

const MemberSelect: React.FC<{
  groupId: number | undefined
  value?: string
  placeholder?: string
  onChange: (account: string | undefined) => void
}> = ({ groupId, value, placeholder = '按人员筛选', onChange }) => {
  const options = useGroupMemberOptions(groupId)
  return (
    <Select
      placeholder={placeholder}
      allowClear
      showSearch
      optionFilterProp="label"
      disabled={!groupId}
      style={{ minWidth: 220 }}
      options={options}
      value={value}
      onChange={(v) => onChange(v as string | undefined)}
    />
  )
}

const IdSearchInput: React.FC<{
  placeholder?: string
  value?: number
  width?: number
  onChange: (id: number | undefined) => void
}> = ({ placeholder = 'ID', value, width = 140, onChange }) => (
  <Input
    allowClear
    placeholder={placeholder}
    style={{ width }}
    value={value?.toString() ?? ''}
    onChange={(e) => {
      const raw = e.target.value.trim()
      if (!raw) {
        onChange(undefined)
        return
      }
      const n = Number(raw)
      if (!Number.isFinite(n) || n <= 0) {
        onChange(undefined)
        return
      }
      onChange(Math.trunc(n))
    }}
  />
)

const AccountInput: React.FC<{
  value?: string
  placeholder?: string
  onChange: (account: string | undefined) => void
}> = ({ value, placeholder = '按账号筛选（手输）', onChange }) => (
  <Input
    allowClear
    placeholder={placeholder}
    style={{ width: 220 }}
    value={value}
    onChange={(e) => {
      const v = e.target.value.trim()
      onChange(v ? v : undefined)
    }}
  />
)

const IterationSelect: React.FC<{
  groupId: number | undefined
  /** 受控：与查询参数 execution_id 同步，避免界面与请求参数不一致 */
  value?: number
  onChange: (executionId: number | undefined) => void
}> = ({ groupId, value, onChange }) => {
  const options = useExecutionOptions(groupId)
  return (
    <Select
      placeholder="按迭代筛选"
      allowClear
      showSearch
      optionFilterProp="label"
      style={{ minWidth: 220 }}
      options={options}
      value={value}
      onChange={(v) => onChange(v as number | undefined)}
    />
  )
}

// ---- Status tag colors ----
const STATUS_COLORS: Record<string, string> = {
  done: 'green', closed: 'default', active: 'blue',
  wait: 'orange', doing: 'blue', resolved: 'cyan', rejected: 'red',
  pause: 'default', cancel: 'red',
}

const RawDataModal: React.FC<{ data: object | null; onClose: () => void }> = ({ data, onClose }) => (
  <Modal
    open={!!data}
    title={<Text style={{ color: 'var(--zm-text-primary)' }}>原始数据 (raw_data)</Text>}
    onCancel={onClose}
    footer={null}
    width={700}
    styles={{
      content: { background: 'var(--zm-bg-surface)', border: '1px solid var(--zm-border-subtle)', borderRadius: 12 },
      header: { background: 'var(--zm-bg-surface)' },
    }}
  >
    {data && (
      <JsonView
        value={data}
        collapsed={2}
        style={{ background: 'transparent', fontSize: 13, fontFamily: 'monospace' }}
      />
    )}
  </Modal>
)

// ---- Generic workbench tab ----
interface ColDef { title: string; dataIndex?: string; key?: string; width?: number; render?: (v: any, r: any) => React.ReactNode }

type WorkbenchFilterCtx = {
  params: WorkbenchParams
  setParams: React.Dispatch<React.SetStateAction<WorkbenchParams>>
}

function useWorkbenchTab(
  fetcher: (p: WorkbenchParams) => Promise<any>,
  columns: ColDef[],
  extraFilters?: React.ReactNode | ((ctx: WorkbenchFilterCtx) => React.ReactNode),
  requireDateRange?: boolean,
) {
  const { groupId, scope } = useWorkbenchGroup()
  const { structureMeta } = useWorkbenchGroup()
  const effectiveGroupId = scope === 'group' ? (groupId ?? undefined) : undefined
  const [data, setData] = useState<any[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useState<WorkbenchParams>({})
  const [rawData, setRawData] = useState<object | null>(null)
  /** 供「仅依赖 group/page 的 effect」读取最新筛选条件，避免闭包陈旧 */
  const paramsRef = useRef(params)
  paramsRef.current = params

  const fetch = useCallback(async (p?: WorkbenchParams, pg?: number) => {
    const merged = p ?? paramsRef.current
    const pageNum = pg ?? page
    if (requireDateRange && (!merged.date_from || !merged.date_to)) {
      setData([]); setTotal(0); return
    }
    setLoading(true)
    try {
      const structureParams: any = {}
      if (structureMeta?.type === 'execution') structureParams.execution_id = structureMeta.id
      if (structureMeta?.type === 'project') structureParams.project_id = structureMeta.id
      if (structureMeta?.type === 'program') structureParams.program_id = structureMeta.id
      if (structureMeta?.type === 'product') structureParams.product_id = structureMeta.id
      const res = await fetcher({ ...merged, ...structureParams, group_id: effectiveGroupId, page: pageNum, page_size: 20 })
      setData(res.data ?? [])
      setTotal(res.total ?? 0)
    } catch (e: any) {
      message.error(e.response?.data?.error ?? '查询失败')
    } finally {
      setLoading(false)
    }
  }, [page, effectiveGroupId, fetcher, requireDateRange, structureMeta])

  useEffect(() => { void fetch() }, [effectiveGroupId, page, fetch])

  const allColumns = [
    ...columns,
    {
      title: '',
      key: 'actions',
      width: 60,
      render: (_: any, row: any) => (
        <Tooltip title="查看原始数据">
          <Button
            size="small" type="text" icon={<EyeOutlined />}
            style={{ color: 'var(--zm-text-muted)' }}
            onClick={() => setRawData(row.raw_data ?? row)}
          />
        </Tooltip>
      ),
    },
  ]

  const handleSearch = () => {
    setPage(1)
    void fetch(paramsRef.current, 1)
  }

  const filterSlot = typeof extraFilters === 'function'
    ? extraFilters({ params, setParams })
    : extraFilters

  return {
    node: (
      <div>
        <Space wrap style={{ marginBottom: 16 }}>
          {filterSlot}
          <Button type="primary" icon={<SearchOutlined />} onClick={handleSearch}
            style={{ background: 'var(--zm-brand-gradient)', border: 'none' }}>
            查询
          </Button>
        </Space>
        <Table
          dataSource={data}
          columns={allColumns}
          rowKey="id"
          loading={loading}
          size="small"
          pagination={{
            current: page, total, pageSize: 20, showTotal: (t) => `共 ${t} 条`,
            onChange: setPage, showSizeChanger: false,
          }}
          scroll={{ x: 900 }}
          style={{ background: 'transparent' }}
        />
        <RawDataModal data={rawData} onClose={() => setRawData(null)} />
      </div>
    ),
    setParams,
    params,
  }
}

// ---- Task Tab ----
const TASK_STATUS_FILTER_OPTIONS = [
  { value: 'wait', label: '未开始' },
  { value: 'doing', label: '进行中' },
  { value: 'done', label: '已完成' },
  { value: 'closed', label: '已关闭' },
]

const TaskTab: React.FC = () => {
  const { groupId, scope } = useWorkbenchGroup()
  const me = useAuthStore((s) => s.me)
  const dataScope = (me?.user?.data_scope ?? '').toUpperCase()
  const boundAccount = me?.zentao_binding?.zentao_account
  const effectiveGroupId = scope === 'group' ? (groupId ?? undefined) : undefined
  const personOf = useMemberPersonDisplay(groupId ?? undefined)
  const taskColumns = useMemo(
    () => [
      { title: 'ID', dataIndex: 'id', width: 70 },
      {
        title: '任务名',
        dataIndex: 'name',
        render: (v: string, r: { id: number }) => (
          <Link
            to={effectiveGroupId ? `/workbench/task/${r.id}?group_id=${effectiveGroupId}&from=/workbench` : `/workbench/task/${r.id}?from=/workbench`}
            style={{ color: 'var(--zm-text-primary)' }}
          >
            {v}
          </Link>
        ),
      },
      {
        title: '类型',
        dataIndex: 'type',
        width: 90,
        render: (v: string) => <Text style={{ color: 'var(--zm-text-secondary)' }}>{taskTypeLabel(v)}</Text>,
      },
      {
        title: '状态',
        dataIndex: 'status',
        width: 96,
        render: (v: string) => (
          <Tag color={STATUS_COLORS[v] ?? 'default'}>{taskStatusLabel(v)}</Tag>
        ),
      },
      {
        title: '指派给',
        dataIndex: 'assigned_to',
        width: 160,
        render: (v: string) => (
          <Text style={{ color: 'var(--zm-text-secondary)' }}>{personOf(v)}</Text>
        ),
      },
      { title: '预估(h)', dataIndex: 'estimate', width: 80 },
      { title: '消耗(h)', dataIndex: 'consumed', width: 80 },
      {
        title: '',
        key: 'quickEffort',
        width: 90,
        render: (_: any, r: { id: number }) => (
          <Link to={`/my-workbench?task_id=${r.id}`}>
            <Button
              size="small"
              type="primary"
              style={{ background: 'var(--zm-brand-gradient)', border: 'none' }}
            >
              报工
            </Button>
          </Link>
        ),
      },
      { title: '最后编辑', dataIndex: 'last_edited_date', width: 130, render: (v: string) => v ? dayjs(v).format('YYYY-MM-DD HH:mm') : '-' },
    ],
    [personOf],
  )
  const { node, setParams } = useWorkbenchTab(
    listTasks,
    taskColumns,
    ({ params, setParams }) => (
      <>
        <IdSearchInput
          placeholder="任务ID"
          value={params.id}
          onChange={(id) => setParams((p) => ({ ...p, id }))}
        />
        <IterationSelect
          groupId={effectiveGroupId}
          value={params.execution_id}
          onChange={(id) => setParams((p) => ({ ...p, execution_id: id }))}
        />
        {dataScope === 'SELF' ? (
          <Tag>仅本人</Tag>
        ) : scope === 'group' ? (
          <MemberSelect
            groupId={effectiveGroupId}
            value={params.assigned_to}
            onChange={(acc) => setParams((p) => ({ ...p, assigned_to: acc }))}
          />
        ) : (
          <AccountInput
            value={params.assigned_to}
            placeholder="指派给(账号)"
            onChange={(acc) => setParams((p) => ({ ...p, assigned_to: acc }))}
          />
        )}
        <Select
          placeholder="状态"
          allowClear
          style={{ width: 120 }}
          value={params.status}
          onChange={(v) => setParams((p) => ({ ...p, status: v }))}
          options={TASK_STATUS_FILTER_OPTIONS}
        />
      </>
    ),
  )
  useLayoutEffect(() => {
    setParams((p) => ({ ...p, assigned_to: undefined }))
  }, [scope, groupId])
  useLayoutEffect(() => {
    if (dataScope === 'SELF' && boundAccount) {
      setParams((p) => ({ ...p, assigned_to: boundAccount }))
    }
  }, [boundAccount, dataScope, setParams])
  return node
}

// ---- Story Tab ----
const StoryTab: React.FC = () => {
  const { groupId, scope } = useWorkbenchGroup()
  const me = useAuthStore((s) => s.me)
  const dataScope = (me?.user?.data_scope ?? '').toUpperCase()
  const boundAccount = me?.zentao_binding?.zentao_account
  const effectiveGroupId = scope === 'group' ? (groupId ?? undefined) : undefined
  const personOf = useMemberPersonDisplay(groupId ?? undefined)
  const storyColumns = useMemo(
    () => [
      { title: 'ID', dataIndex: 'id', width: 70 },
      { title: '需求标题', dataIndex: 'title', render: (v: string) => <Text style={{ color: 'var(--zm-text-primary)' }}>{v}</Text> },
      {
        title: '状态',
        dataIndex: 'status',
        width: 96,
        render: (v: string) => (
          <Tag color={STORY_STATUS_TAG_COLOR[v] ?? STATUS_COLORS[v] ?? 'default'}>
            {storyStatusLabel(v)}
          </Tag>
        ),
      },
      {
        title: '指派给',
        dataIndex: 'assigned_to',
        width: 160,
        render: (v: string) => <Text style={{ color: 'var(--zm-text-secondary)' }}>{personOf(v)}</Text>,
      },
      { title: '预估(h)', dataIndex: 'estimate', width: 80 },
      { title: '最后编辑', dataIndex: 'last_edited_date', width: 130, render: (v: string) => v ? dayjs(v).format('YYYY-MM-DD HH:mm') : '-' },
    ],
    [personOf],
  )
  const { node, setParams } = useWorkbenchTab(
    listStories,
    storyColumns,
    ({ params, setParams }) => (
      <>
        <IdSearchInput
          placeholder="需求ID"
          value={params.id}
          onChange={(id) => setParams((p) => ({ ...p, id }))}
        />
        <IterationSelect
          groupId={effectiveGroupId}
          value={params.execution_id}
          onChange={(id) => setParams((p) => ({ ...p, execution_id: id }))}
        />
        {dataScope === 'SELF' ? (
          <Tag>仅本人</Tag>
        ) : scope === 'group' ? (
          <MemberSelect
            groupId={effectiveGroupId}
            value={params.assigned_to}
            onChange={(acc) => setParams((p) => ({ ...p, assigned_to: acc }))}
          />
        ) : (
          <AccountInput
            value={params.assigned_to}
            placeholder="指派给(账号)"
            onChange={(acc) => setParams((p) => ({ ...p, assigned_to: acc }))}
          />
        )}
      </>
    ),
  )
  useLayoutEffect(() => {
    setParams((p) => ({ ...p, assigned_to: undefined }))
  }, [scope, groupId])
  useLayoutEffect(() => {
    if (dataScope === 'SELF' && boundAccount) {
      setParams((p) => ({ ...p, assigned_to: boundAccount }))
    }
  }, [boundAccount, dataScope, setParams])
  return node
}

// ---- Bug Tab ----
const BugTab: React.FC = () => {
  const { groupId, scope } = useWorkbenchGroup()
  const me = useAuthStore((s) => s.me)
  const dataScope = (me?.user?.data_scope ?? '').toUpperCase()
  const boundAccount = me?.zentao_binding?.zentao_account
  const effectiveGroupId = scope === 'group' ? (groupId ?? undefined) : undefined
  const personOf = useMemberPersonDisplay(groupId ?? undefined)
  const bugColumns = useMemo(
    () => [
      { title: 'ID', dataIndex: 'id', width: 70 },
      { title: '缺陷标题', dataIndex: 'title', render: (v: string) => <Text style={{ color: 'var(--zm-text-primary)' }}>{v}</Text> },
      { title: '严重性', dataIndex: 'severity', width: 70, render: (v: number) => <Tag color={v <= 2 ? 'red' : v <= 3 ? 'orange' : 'default'}>P{v}</Tag> },
      {
        title: '状态',
        dataIndex: 'status',
        width: 96,
        render: (v: string) => (
          <Tag color={BUG_STATUS_TAG_COLOR[v] ?? STATUS_COLORS[v] ?? 'default'}>
            {bugStatusLabel(v)}
          </Tag>
        ),
      },
      {
        title: '指派给',
        dataIndex: 'assigned_to',
        width: 160,
        render: (v: string) => <Text style={{ color: 'var(--zm-text-secondary)' }}>{personOf(v)}</Text>,
      },
      {
        title: '解决人',
        dataIndex: 'resolved_by',
        width: 160,
        render: (v: string) => <Text style={{ color: 'var(--zm-text-secondary)' }}>{personOf(v)}</Text>,
      },
      {
        title: '解决方案',
        dataIndex: 'resolution',
        width: 120,
        render: (v: string) => (
          <Text style={{ color: 'var(--zm-text-secondary)' }}>{bugResolutionLabel(v)}</Text>
        ),
      },
      { title: '最后编辑', dataIndex: 'last_edited_date', width: 130, render: (v: string) => v ? dayjs(v).format('YYYY-MM-DD HH:mm') : '-' },
    ],
    [personOf],
  )
  const { node, setParams } = useWorkbenchTab(
    listBugs,
    bugColumns,
    ({ params, setParams }) => (
      <>
        <IdSearchInput
          placeholder="缺陷ID"
          value={params.id}
          onChange={(id) => setParams((p) => ({ ...p, id }))}
        />
        <IterationSelect
          groupId={effectiveGroupId}
          value={params.execution_id}
          onChange={(id) => setParams((p) => ({ ...p, execution_id: id }))}
        />
        {dataScope === 'SELF' ? (
          <Tag>仅本人</Tag>
        ) : scope === 'group' ? (
          <MemberSelect
            groupId={effectiveGroupId}
            value={params.assigned_to}
            onChange={(acc) => setParams((p) => ({ ...p, assigned_to: acc }))}
          />
        ) : (
          <AccountInput
            value={params.assigned_to}
            placeholder="指派给(账号)"
            onChange={(acc) => setParams((p) => ({ ...p, assigned_to: acc }))}
          />
        )}
      </>
    ),
  )
  useLayoutEffect(() => {
    setParams((p) => ({ ...p, assigned_to: undefined }))
  }, [scope, groupId])
  useLayoutEffect(() => {
    if (dataScope === 'SELF' && boundAccount) {
      setParams((p) => ({ ...p, assigned_to: boundAccount }))
    }
  }, [boundAccount, dataScope, setParams])
  return node
}

// ---- Effort Tab ----
const EffortTab: React.FC = () => {
  const [dateRange, setDateRange] = useState<[Dayjs, Dayjs] | null>(null)
  const [taskIdFilter, setTaskIdFilter] = useState<number | undefined>()
  const [taskOptions, setTaskOptions] = useState<{ value: number; label: string }[]>([])
  const { groupId, scope } = useWorkbenchGroup()
  const me = useAuthStore((s) => s.me)
  const dataScope = (me?.user?.data_scope ?? '').toUpperCase()
  const boundAccount = me?.zentao_binding?.zentao_account
  const effectiveGroupId = scope === 'group' ? (groupId ?? undefined) : undefined
  const personOf = useMemberPersonDisplay(groupId ?? undefined)
  const effortColumns = useMemo(
    () => [
      { title: 'ID', dataIndex: 'id', width: 70 },
      {
        title: '登记人',
        dataIndex: 'account',
        width: 160,
        render: (v: string) => <Text style={{ color: 'var(--zm-text-secondary)' }}>{personOf(v)}</Text>,
      },
      { title: '日期', dataIndex: 'work_date', width: 100, render: (v: string) => v ? dayjs(v).format('YYYY-MM-DD') : '-' },
      { title: '消耗(h)', dataIndex: 'consumed', width: 80 },
      { title: '工作内容', dataIndex: 'work', render: (v: string) => <Text style={{ color: 'var(--zm-text-secondary)' }}>{v}</Text> },
      { title: '关联类型', dataIndex: 'object_type', width: 80 },
      { title: '关联ID', dataIndex: 'object_id', width: 80 },
    ],
    [personOf],
  )
  const { node, setParams, params } = useWorkbenchTab(
    listEfforts,
    effortColumns,
    ({ params, setParams }) => (
      <>
        <IdSearchInput
          placeholder="报工ID"
          value={params.id}
          onChange={(id) => setParams((p) => ({ ...p, id }))}
        />
        <IterationSelect
          groupId={effectiveGroupId}
          value={params.execution_id}
          onChange={(id) => {
            setTaskIdFilter(undefined)
            setParams((p) => ({ ...p, execution_id: id, task_id: undefined }))
          }}
        />
        {dataScope === 'SELF' ? (
          <Tag>仅本人</Tag>
        ) : scope === 'group' ? (
          <MemberSelect
            groupId={effectiveGroupId}
            value={params.account}
            placeholder="按登记人筛选"
            onChange={(acc) => setParams((p) => ({ ...p, account: acc }))}
          />
        ) : (
          <AccountInput
            value={params.account}
            placeholder="登记人(账号)"
            onChange={(acc) => setParams((p) => ({ ...p, account: acc }))}
          />
        )}
        <Select
          placeholder="按任务筛选"
          allowClear
          showSearch
          optionFilterProp="label"
          disabled={scope === 'group' && !effectiveGroupId}
          style={{ minWidth: 280 }}
          options={taskOptions}
          value={taskIdFilter}
          onChange={(v) => {
            if (v === undefined) {
              setTaskIdFilter(undefined)
              setParams((p) => ({ ...p, task_id: undefined }))
              return
            }
            const id = Number(v)
            if (!Number.isFinite(id)) {
              setTaskIdFilter(undefined)
              setParams((p) => ({ ...p, task_id: undefined }))
              return
            }
            setTaskIdFilter(id)
            setParams((p) => ({ ...p, task_id: id }))
          }}
        />
        <RangePicker
          onChange={(dates) => {
            if (dates && dates[0] && dates[1]) {
              const from = dates[0].format('YYYY-MM-DD')
              const to = dates[1].format('YYYY-MM-DD')
              setParams((p) => ({ ...p, date_from: from, date_to: to }))
              setDateRange([dates[0], dates[1]])
            } else {
              setParams((p) => ({ ...p, date_from: undefined, date_to: undefined }))
              setDateRange(null)
            }
          }}
          disabledDate={(current) => {
            if (!dateRange) return false
            return Math.abs(current.diff(dateRange[0], 'day')) > 180
          }}
          placeholder={['开始日期', '结束日期 (最多半年)']}
        />
      </>
    ),
    true, // requireDateRange
  )

  useEffect(() => {
    if (scope === 'group' && !effectiveGroupId) {
      setTaskOptions([])
      return
    }
    let cancelled = false
    listTasks({
      group_id: effectiveGroupId,
      execution_id: params.execution_id,
      page: 1,
      page_size: 200,
    })
      .then((res) => {
        if (cancelled) return
        const rows = res.data ?? []
        setTaskOptions(
          rows.map((t: { id: number; name: string }) => ({
            value: t.id,
            label: t.name ? `${t.id} · ${t.name}` : String(t.id),
          })),
        )
      })
      .catch(() => {
        if (!cancelled) setTaskOptions([])
      })
    return () => {
      cancelled = true
    }
  }, [scope, effectiveGroupId, params.execution_id])

  useLayoutEffect(() => {
    setTaskIdFilter(undefined)
    setParams((p) => ({ ...p, task_id: undefined, account: undefined }))
  }, [scope, groupId])
  useLayoutEffect(() => {
    if (dataScope === 'SELF' && boundAccount) {
      setParams((p) => ({ ...p, account: boundAccount }))
    }
  }, [boundAccount, dataScope, setParams])

  return (
    <div>
      <div style={{ marginBottom: 8, color: 'var(--zm-text-muted)', fontSize: 12 }}>
        展示当前小组（成员）在禅道登记的报工明细；可按迭代、登记人、任务筛选；数据量大，时间跨度限制为 6 个月内，请务必选择时间范围。
        登记人与任务、迭代等条件为「且」关系；若选任务后无数据，可尝试清空「按登记人筛选」后再查（任务详情页不按登记人过滤）。
      </div>
      {node}
    </div>
  )
}

// ---- Execution Tab ----
const ExecutionTab: React.FC = () => {
  const executionColumns = useMemo(
    () => [
      { title: 'ID', dataIndex: 'id', width: 70 },
      { title: '迭代名', dataIndex: 'name', render: (v: string) => <Text style={{ color: 'var(--zm-text-primary)' }}>{v}</Text> },
      {
        title: '状态',
        dataIndex: 'status',
        width: 96,
        render: (v: string) => (
          <Tag color={EXECUTION_STATUS_TAG_COLOR[v] ?? STATUS_COLORS[v] ?? 'default'}>
            {executionStatusLabel(v)}
          </Tag>
        ),
      },
      { title: '开始', dataIndex: 'begin_date', width: 110, render: (v: string) => v ? dayjs(v).format('YYYY-MM-DD') : '-' },
      { title: '结束', dataIndex: 'end_date', width: 110, render: (v: string) => v ? dayjs(v).format('YYYY-MM-DD') : '-' },
    ],
    [],
  )
  const { node } = useWorkbenchTab(
    listExecutions,
    executionColumns,
    ({ params, setParams }) => (
      <>
        <IdSearchInput
          placeholder="迭代ID"
          value={params.execution_id}
          onChange={(id) => setParams((p) => ({ ...p, execution_id: id }))}
        />
        <Input
          allowClear
          placeholder="迭代名"
          style={{ width: 220 }}
          value={params.name ?? ''}
          onChange={(e) => {
            const v = e.target.value.trim()
            setParams((p) => ({ ...p, name: v ? v : undefined }))
          }}
        />
      </>
    ),
  )
  return node
}

// ---- Project Tab ----
const ProjectTab: React.FC = () => {
  const { groupId, scope } = useWorkbenchGroup()
  const effectiveGroupId = scope === 'group' ? (groupId ?? undefined) : undefined
  const [detailOpen, setDetailOpen] = useState(false)
  const [detailLoading, setDetailLoading] = useState(false)
  const [detail, setDetail] = useState<WorkbenchProjectDetail | null>(null)

  const openDetail = useCallback((id: number) => {
    setDetailOpen(true)
    setDetailLoading(true)
    setDetail(null)
    getWorkbenchProject(id, effectiveGroupId ? { group_id: effectiveGroupId } : undefined)
      .then((d) => setDetail(d))
      .catch((e: any) => {
        message.error(e.response?.data?.error ?? '加载项目详情失败')
        setDetailOpen(false)
      })
      .finally(() => setDetailLoading(false))
  }, [effectiveGroupId])

  const projectColumns = useMemo(
    () => [
      { title: 'ID', dataIndex: 'id', width: 80 },
      {
        title: '项目名称',
        dataIndex: 'name',
        render: (v: string, r: { id: number }) => (
          <Button
            type="link"
            style={{ padding: 0, height: 'auto', color: 'var(--zm-text-primary)' }}
            onClick={() => openDetail(r.id)}
          >
            {v || '—'}
          </Button>
        ),
      },
      {
        title: '状态',
        dataIndex: 'status',
        width: 96,
        render: (v: string) => (
          <Tag color={STATUS_COLORS[v] ?? 'default'}>{v || '—'}</Tag>
        ),
      },
      {
        title: '项目集 ID',
        dataIndex: 'parent_id',
        width: 100,
        render: (v: number | null | undefined) => (v != null && v > 0 ? v : '—'),
      },
      {
        title: '开始',
        dataIndex: 'begin_date',
        width: 110,
        render: (v: string) => (v ? dayjs(v).format('YYYY-MM-DD') : '-'),
      },
      {
        title: '结束',
        dataIndex: 'end_date',
        width: 110,
        render: (v: string) => (v ? dayjs(v).format('YYYY-MM-DD') : '-'),
      },
    ],
    [openDetail],
  )

  const { node } = useWorkbenchTab(
    listProjects,
    projectColumns,
    ({ params, setParams }) => (
      <>
        <IdSearchInput
          placeholder="项目ID"
          value={params.project_id}
          onChange={(project_id) => setParams((p) => ({ ...p, project_id }))}
        />
        <Input
          allowClear
          placeholder="项目名称（模糊）"
          style={{ width: 240 }}
          value={params.name ?? ''}
          onChange={(e) => {
            const v = e.target.value.trim()
            setParams((p) => ({ ...p, name: v ? v : undefined }))
          }}
        />
      </>
    ),
  )

  const p = detail?.project

  return (
    <>
      <div style={{ marginBottom: 8, color: 'var(--zm-text-muted)', fontSize: 12 }}>
        展示已同步的禅道项目；支持按 ID 或名称搜索。选择「按小组」时，仅列出该小组成员在任务或缺陷中出现过的迭代所属项目（与「迭代」Tab 可见范围一致）。右上方结构树筛选对本列表同样生效。
      </div>
      {node}
      <Modal
        open={detailOpen}
        title={p?.name ? `项目 · ${p.name}` : '项目详情'}
        onCancel={() => setDetailOpen(false)}
        footer={null}
        width={760}
        destroyOnClose
        styles={{
          content: { background: 'var(--zm-bg-surface)', border: '1px solid var(--zm-border-subtle)', borderRadius: 12 },
          header: { background: 'var(--zm-bg-surface)' },
        }}
      >
        {detailLoading ? (
          <div style={{ textAlign: 'center', padding: 32 }}>
            <Spin />
          </div>
        ) : detail && p ? (
          <div>
            <Space direction="vertical" size="middle" style={{ width: '100%' }}>
              <div>
                <Text type="secondary" style={{ fontSize: 12 }}>状态</Text>
                <div>
                  <Tag color={STATUS_COLORS[p.status] ?? 'default'}>{p.status || '—'}</Tag>
                </div>
              </div>
              {(detail.program_name || p.parent_id) ? (
                <div>
                  <Text type="secondary" style={{ fontSize: 12 }}>所属项目集</Text>
                  <div>
                    <Text style={{ color: 'var(--zm-text-primary)' }}>
                      {detail.program_name || '—'}
                      {p.parent_id ? `（ID ${p.parent_id}）` : ''}
                    </Text>
                  </div>
                </div>
              ) : null}
              <div style={{ display: 'flex', gap: 24, flexWrap: 'wrap' }}>
                <div>
                  <Text type="secondary" style={{ fontSize: 12 }}>计划开始</Text>
                  <div>
                    <Text style={{ color: 'var(--zm-text-primary)' }}>
                      {p.begin_date ? dayjs(p.begin_date).format('YYYY-MM-DD') : '—'}
                    </Text>
                  </div>
                </div>
                <div>
                  <Text type="secondary" style={{ fontSize: 12 }}>计划结束</Text>
                  <div>
                    <Text style={{ color: 'var(--zm-text-primary)' }}>
                      {p.end_date ? dayjs(p.end_date).format('YYYY-MM-DD') : '—'}
                    </Text>
                  </div>
                </div>
              </div>
              {p.path ? (
                <div>
                  <Text type="secondary" style={{ fontSize: 12 }}>路径</Text>
                  <div>
                    <Text style={{ color: 'var(--zm-text-secondary)', wordBreak: 'break-all' }}>{p.path}</Text>
                  </div>
                </div>
              ) : null}
              <div>
                <Text style={{ color: 'var(--zm-text-primary)', fontWeight: 600, marginBottom: 8, display: 'block' }}>
                  下属迭代（最多 100 条）
                </Text>
                <Table
                  size="small"
                  rowKey="id"
                  dataSource={detail.executions ?? []}
                  pagination={false}
                  scroll={{ x: 560 }}
                  columns={[
                    { title: 'ID', dataIndex: 'id', width: 72 },
                    { title: '迭代名', dataIndex: 'name', render: (v: string) => <Text style={{ color: 'var(--zm-text-primary)' }}>{v || '—'}</Text> },
                    {
                      title: '状态',
                      dataIndex: 'status',
                      width: 96,
                      render: (v: string) => (
                        <Tag color={EXECUTION_STATUS_TAG_COLOR[v] ?? STATUS_COLORS[v] ?? 'default'}>
                          {executionStatusLabel(v)}
                        </Tag>
                      ),
                    },
                    { title: '开始', dataIndex: 'begin_date', width: 110, render: (v: string) => (v ? dayjs(v).format('YYYY-MM-DD') : '-') },
                    { title: '结束', dataIndex: 'end_date', width: 110, render: (v: string) => (v ? dayjs(v).format('YYYY-MM-DD') : '-') },
                  ]}
                />
              </div>
            </Space>
          </div>
        ) : null}
      </Modal>
    </>
  )
}

// ---- Main Workbench Page ----
const WorkbenchPage: React.FC = () => {
  const [scope, setScope] = useState<WorkbenchScope>('all')
  const [groupId, setGroupId] = useState<number | undefined>()
  const [groupName, setGroupName] = useState('')
  const [structureKey, setStructureKey] = useState<string | undefined>()
  const [structureMeta, setStructureMeta] = useState<{ type: string; id: number } | undefined>()
  const me = useAuthStore((s) => s.me)
  const dataScope = (me?.user?.data_scope ?? '').toUpperCase()
  const defaultGroupId = me?.user?.default_group_id ?? undefined
  const scopeLocked = dataScope === 'GROUP'
  const forceGroup = dataScope === 'GROUP'
  const ctxValue = useMemo<WorkbenchGroupCtx>(() => ({
    scope,
    groupId,
    groupName,
    structureKey,
    structureMeta,
    setGroup: (id, name) => {
      setGroupId(id)
      setGroupName(name)
    },
    setScope,
    setStructure: (key, meta) => {
      setStructureKey(key)
      setStructureMeta(meta)
    },
  }), [scope, groupId, groupName, structureKey, structureMeta])

  useEffect(() => {
    if (scope === 'all') {
      setGroupId(undefined)
      setGroupName('')
    }
  }, [scope])

  useEffect(() => {
    if (forceGroup) {
      setScope('group')
      if (defaultGroupId) setGroupId(defaultGroupId)
    }
  }, [defaultGroupId, forceGroup])

  return (
    <WorkbenchGroupContext.Provider value={ctxValue}>
      <div>
        <div style={{ marginBottom: 20 }}>
          <Text style={{ color: 'var(--zm-text-primary)', fontSize: 18, fontWeight: 600 }}>数据明细</Text>
          <div style={{ marginTop: 12 }}>
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 12,
                flexWrap: 'wrap',
                padding: '10px 12px',
                borderRadius: 12,
                border: '1px solid var(--zm-border-subtle)',
                background: 'rgba(255,255,255,0.03)',
              }}
            >
              <Text style={{ color: 'var(--zm-text-muted)', fontSize: 12, fontWeight: 600 }}>
                统计范围
              </Text>
              <Segmented
                size="large"
                block
                value={scope}
                onChange={(v) => ctxValue.setScope(v as WorkbenchScope)}
                disabled={scopeLocked}
                options={[
                  {
                    label: <span style={{ fontWeight: 600 }}>按全部</span>,
                    value: 'all',
                  },
                  {
                    label: <span style={{ fontWeight: 600 }}>按小组</span>,
                    value: 'group',
                  },
                ]}
                style={{
                  minWidth: 240,
                  background: 'var(--zm-bg-surface)',
                  border: '1px solid var(--zm-border-subtle)',
                  borderRadius: 12,
                }}
              />
              <Space wrap>
                {scope === 'group' ? (
                  <>
                    <GroupSelect
                      value={groupId}
                      onChange={(id, name) => ctxValue.setGroup(id, name)}
                      disabled={scopeLocked}
                      allowedGroupIds={scopeLocked && defaultGroupId ? [defaultGroupId] : undefined}
                    />
                    {groupId ? <Tag color="purple">{groupName}</Tag> : <Tag>请选择小组</Tag>}
                  </>
                ) : (
                  <Tag color="blue">全部</Tag>
                )}
              </Space>

              <div style={{ flex: 1 }} />
              <WorkbenchStructureSelect
                value={structureKey}
                onChange={(key, meta) => ctxValue.setStructure(key, meta)}
              />
            </div>
          </div>
        </div>

        <div style={{
          background: 'var(--zm-bg-surface)',
          border: '1px solid var(--zm-border-subtle)',
          borderRadius: 12,
          padding: '16px 20px',
        }}>
          <Tabs
            defaultActiveKey="tasks"
            items={[
              { key: 'tasks', label: '📋 任务', children: <TaskTab /> },
              { key: 'projects', label: '📁 项目', children: <ProjectTab /> },
              { key: 'efforts', label: '⏱ 报工', children: <EffortTab /> },
              { key: 'stories', label: '📖 需求', children: <StoryTab /> },
              { key: 'bugs', label: '🐛 缺陷', children: <BugTab /> },
              { key: 'executions', label: '🚀 迭代', children: <ExecutionTab /> },
            ]}
          />
        </div>
      </div>
    </WorkbenchGroupContext.Provider>
  )
}

export default WorkbenchPage
