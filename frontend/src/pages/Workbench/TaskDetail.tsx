import React, { useCallback, useEffect, useState } from 'react'
import { Link, useLocation, useParams } from 'react-router-dom'
import {
  Typography, Button, Table, Space, DatePicker, Tag, Tooltip, Modal, message, Descriptions, Collapse, Tabs,
} from 'antd'
import { ArrowLeftOutlined, EyeOutlined, LinkOutlined, SearchOutlined } from '@ant-design/icons'
import JsonView from '@uiw/react-json-view'
import dayjs, { Dayjs } from 'dayjs'
import { getTask, getZentaoAPIConfig, listEfforts } from '../../api'
import { useAuthStore } from '../../store/auth'
import { buildZentaoTaskViewUrl } from '../../utils/zentaoUrls'
import { taskTypeLabel, taskStatusLabel, useMemberPersonDisplay } from './workbenchDisplay'

const { RangePicker } = DatePicker
const { Text } = Typography

const STATUS_COLORS: Record<string, string> = {
  done: 'green', closed: 'default', active: 'blue',
  wait: 'orange', doing: 'blue', resolved: 'cyan', rejected: 'red',
  pause: 'default', cancel: 'red',
}

const panelStyle: React.CSSProperties = {
  background: 'var(--zm-bg-surface)',
  border: '1px solid var(--zm-border-subtle)',
  borderRadius: 12,
  padding: '16px 20px',
}

function formatDetailValue(v: unknown): string {
  if (v == null || v === '') return '-'
  if (typeof v === 'boolean') return v ? '是' : '否'
  if (typeof v === 'number') return Number.isFinite(v) ? String(v) : '-'
  if (typeof v === 'string') {
    const d = dayjs(v)
    if (d.isValid() && /^\d{4}-\d{2}-\d{2}/.test(v)) {
      return d.format(v.includes('T') || v.includes(':') ? 'YYYY-MM-DD HH:mm:ss' : 'YYYY-MM-DD')
    }
    return v
  }
  return String(v)
}

function taskRawData(task: Record<string, unknown>): object | null {
  const raw = task.raw_data
  if (raw != null && typeof raw === 'object' && !Array.isArray(raw)) {
    return raw as object
  }
  return null
}

const TaskFullDetailPanel: React.FC<{
  task: Record<string, unknown>
  personOf: (account: string | undefined | null) => string
  onViewJson: () => void
}> = ({ task, personOf, onViewJson }) => {
  const raw = taskRawData(task)
  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'flex-end', gap: 12, flexWrap: 'wrap', marginBottom: 12 }}>
        <Button size="small" icon={<EyeOutlined />} onClick={onViewJson}>
          弹窗查看 JSON
        </Button>
      </div>
      <Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 12 }}>
        以下为本地库同步字段；底部为禅道源表 raw_data（ETL 全量快照）
      </Text>
      <Descriptions
        bordered
        size="small"
        column={{ xs: 1, sm: 1, md: 2, lg: 2, xl: 2, xxl: 2 }}
        styles={{ label: { width: 120, color: 'var(--zm-text-muted)' } }}
      >
        <Descriptions.Item label="任务 ID">{formatDetailValue(task.id)}</Descriptions.Item>
        <Descriptions.Item label="任务名称">{formatDetailValue(task.name)}</Descriptions.Item>
        <Descriptions.Item label="类型">
          {taskTypeLabel(String(task.type ?? ''))}
          {task.type ? <Text type="secondary"> ({String(task.type)})</Text> : null}
        </Descriptions.Item>
        <Descriptions.Item label="状态">
          <Tag color={STATUS_COLORS[String(task.status)] ?? 'default'} style={{ margin: 0 }}>
            {taskStatusLabel(String(task.status ?? ''))}
          </Tag>
          {task.status ? <Text type="secondary"> ({String(task.status)})</Text> : null}
        </Descriptions.Item>
        <Descriptions.Item label="指派给">{personOf(String(task.assigned_to ?? ''))}</Descriptions.Item>
        <Descriptions.Item label="完成者">{personOf(String(task.finished_by ?? ''))}</Descriptions.Item>
        <Descriptions.Item label="预估(h)">{formatDetailValue(task.estimate)}</Descriptions.Item>
        <Descriptions.Item label="消耗(h)">{formatDetailValue(task.consumed)}</Descriptions.Item>
        <Descriptions.Item label="执行 ID">{formatDetailValue(task.execution_id)}</Descriptions.Item>
        <Descriptions.Item label="需求 ID">{formatDetailValue(task.story_id)}</Descriptions.Item>
        <Descriptions.Item label="创建时间">{formatDetailValue(task.opened_date)}</Descriptions.Item>
        <Descriptions.Item label="开始时间">{formatDetailValue(task.started_date)}</Descriptions.Item>
        <Descriptions.Item label="指派时间">{formatDetailValue(task.assigned_date)}</Descriptions.Item>
        <Descriptions.Item label="截止时间">{formatDetailValue(task.deadline_date)}</Descriptions.Item>
        <Descriptions.Item label="完成时间">{formatDetailValue(task.finished_date)}</Descriptions.Item>
        <Descriptions.Item label="关闭时间">{formatDetailValue(task.closed_date)}</Descriptions.Item>
        <Descriptions.Item label="最后编辑">{formatDetailValue(task.last_edited_date)}</Descriptions.Item>
        <Descriptions.Item label="已删除">{formatDetailValue(task.deleted)}</Descriptions.Item>
        <Descriptions.Item label="同步时间">{formatDetailValue(task.synced_at)}</Descriptions.Item>
      </Descriptions>
      <Collapse
        bordered={false}
        defaultActiveKey={[]}
        style={{ marginTop: 16, background: 'transparent' }}
        items={[
          {
            key: 'raw',
            label: (
              <Text style={{ color: 'var(--zm-text-primary)', fontWeight: 600 }}>
                禅道原始数据 (raw_data)
              </Text>
            ),
            children: raw ? (
              <div
                style={{
                  maxHeight: 480,
                  overflow: 'auto',
                  padding: 12,
                  borderRadius: 8,
                  border: '1px solid var(--zm-border-subtle)',
                  background: 'rgba(0,0,0,0.15)',
                }}
              >
                <JsonView
                  value={raw}
                  collapsed={2}
                  style={{ background: 'transparent', fontSize: 13, fontFamily: 'monospace' }}
                />
              </div>
            ) : (
              <Text type="secondary" style={{ fontSize: 12 }}>暂无 raw_data</Text>
            ),
          },
        ]}
      />
    </div>
  )
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

/** 任务详情：展示任务字段 + 该任务下的报工明细 */
const TaskDetailPage: React.FC = () => {
  const { taskId: taskIdParam } = useParams<{ taskId: string }>()
  const taskId = Number(taskIdParam)
  const location = useLocation()
  const me = useAuthStore((s) => s.me)
  const sp = new URLSearchParams(location.search)
  const initialGroupId = (() => {
    const raw = sp.get('group_id')
    const n = raw ? Number(raw) : NaN
    return Number.isFinite(n) && n > 0 ? n : undefined
  })()
  const backTo = sp.get('from') || '/workbench'
  const fromMyWorkbench = backTo === '/my-workbench'
  const backLabel = fromMyWorkbench ? '我的工作台' : '数据明细'
  const [groupId, setGroupId] = useState<number | undefined>(initialGroupId)
  const dataScope = String(me?.user?.data_scope ?? '').toUpperCase()
  const defaultGroupId = me?.user?.default_group_id ?? undefined
  const effectiveGroupId = groupId ?? (dataScope === 'GROUP' ? (defaultGroupId ?? undefined) : undefined)
  const personOf = useMemberPersonDisplay(effectiveGroupId ?? undefined)

  const [task, setTask] = useState<Record<string, unknown> | null>(null)
  const [taskLoading, setTaskLoading] = useState(true)
  const [efforts, setEfforts] = useState<any[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [effortLoading, setEffortLoading] = useState(false)
  const [rawData, setRawData] = useState<object | null>(null)
  const [zentaoBaseUrl, setZentaoBaseUrl] = useState('')
  const [dateRange, setDateRange] = useState<[Dayjs, Dayjs]>(() => [
    dayjs().subtract(89, 'day'),
    dayjs(),
  ])

  useEffect(() => {
    getZentaoAPIConfig()
      .then((d) => setZentaoBaseUrl(String(d?.base_url ?? '').trim()))
      .catch(() => setZentaoBaseUrl(''))
  }, [])

  const loadTask = useCallback(async () => {
    if (!Number.isFinite(taskId) || taskId <= 0) return
    setTaskLoading(true)
    try {
      const row = await getTask(taskId, fromMyWorkbench
        ? { my_binding: 1 }
        : { group_id: effectiveGroupId ?? undefined })
      setTask(row as Record<string, unknown>)
    } catch (e: any) {
      message.error(e.response?.data?.error ?? '加载任务失败')
      setTask(null)
    } finally {
      setTaskLoading(false)
    }
  }, [taskId, effectiveGroupId, fromMyWorkbench])

  const loadEfforts = useCallback(async () => {
    if (!Number.isFinite(taskId) || taskId <= 0) return
    if (!task) {
      setEfforts([])
      setTotal(0)
      return
    }
    const from = dateRange[0].format('YYYY-MM-DD')
    const to = dateRange[1].format('YYYY-MM-DD')
    setEffortLoading(true)
    try {
      const res = await listEfforts(fromMyWorkbench
        ? {
            my_binding: 1,
            task_id: taskId,
            date_from: from,
            date_to: to,
            page,
            page_size: 20,
          }
        : {
            group_id: effectiveGroupId ?? undefined,
            task_id: taskId,
            date_from: from,
            date_to: to,
            page,
            page_size: 20,
          })
      setEfforts(res.data ?? [])
      setTotal(res.total ?? 0)
    } catch (e: any) {
      message.error(e.response?.data?.error ?? '加载报工失败')
    } finally {
      setEffortLoading(false)
    }
  }, [task, taskId, effectiveGroupId, dateRange, page, fromMyWorkbench])

  useEffect(() => {
    loadTask()
  }, [loadTask])

  useEffect(() => {
    loadEfforts()
  }, [loadEfforts])

  const handleSearch = () => {
    void loadEfforts()
  }

  const zentaoTaskUrl = task
    ? buildZentaoTaskViewUrl(zentaoBaseUrl, taskId, task.execution_id as number | undefined)
    : null

  const effortColumns = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    {
      title: '登记人',
      dataIndex: 'account',
      width: 160,
      render: (v: string) => <Text style={{ color: 'var(--zm-text-secondary)' }}>{personOf(v)}</Text>,
    },
    { title: '日期', dataIndex: 'work_date', width: 100, render: (v: string) => (v ? dayjs(v).format('YYYY-MM-DD') : '-') },
    { title: '消耗(h)', dataIndex: 'consumed', width: 80 },
    { title: '工作内容', dataIndex: 'work', render: (v: string) => <Text style={{ color: 'var(--zm-text-secondary)' }}>{v}</Text> },
    {
      title: '',
      key: 'actions',
      width: 60,
      render: (_: unknown, row: any) => (
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

  if (!Number.isFinite(taskId) || taskId <= 0) {
    return (
      <div>
        <Text type="danger">无效的任务 ID</Text>
        <div style={{ marginTop: 16 }}>
          <Link to={backTo}>返回{backLabel}</Link>
        </div>
      </div>
    )
  }

  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <Link to={backTo}>
          <Button type="text" icon={<ArrowLeftOutlined />} style={{ color: 'var(--zm-text-secondary)' }}>
            {backLabel}
          </Button>
        </Link>
      </Space>

      <div style={{ marginBottom: 20 }}>
        <Text style={{ color: 'var(--zm-text-primary)', fontSize: 18, fontWeight: 600 }}>任务详情</Text>
        <Tag color="purple" style={{ marginLeft: 12 }}>#{taskId}</Tag>
        <Space style={{ marginLeft: 12 }} wrap>
          {effectiveGroupId
            ? <Tag color="blue">group_id: {effectiveGroupId}</Tag>
            : <Tag>未指定小组</Tag>}
        </Space>
        <Link to={`/my-workbench?task_id=${taskId}`} style={{ marginLeft: 12 }}>
          <Button type="primary" size="small" style={{ background: 'var(--zm-brand-gradient)', border: 'none' }}>
            报工
          </Button>
        </Link>
      </div>

      {taskLoading ? (
        <Text style={{ color: 'var(--zm-text-muted)' }}>加载中…</Text>
      ) : !task ? (
        <Text type="danger">任务不存在或无权查看</Text>
      ) : (
        <>
          <div style={{ ...panelStyle, marginBottom: 20 }}>
            <Space direction="vertical" size={8} style={{ width: '100%' }}>
              <Space align="center" wrap>
                <Text style={{ color: 'var(--zm-text-primary)', fontSize: 16 }}>{String(task.name ?? '')}</Text>
                {zentaoTaskUrl ? (
                  <Button
                    size="small"
                    icon={<LinkOutlined />}
                    href={zentaoTaskUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    禅道任务
                  </Button>
                ) : null}
              </Space>
              <Space wrap>
                <Tag color={STATUS_COLORS[String(task.status)] ?? 'default'}>
                  {taskStatusLabel(String(task.status ?? ''))}
                </Tag>
                <Text type="secondary">类型 {taskTypeLabel(String(task.type ?? ''))}</Text>
                <Text type="secondary">指派 {personOf(String(task.assigned_to ?? ''))}</Text>
                <Text type="secondary">预估(h) {String(task.estimate ?? '-')}</Text>
                <Text type="secondary">消耗(h) {String(task.consumed ?? '-')}</Text>
              </Space>
            </Space>
          </div>

          <div style={panelStyle}>
            <Tabs
              defaultActiveKey="efforts"
              items={[
                {
                  key: 'efforts',
                  label: '报工明细',
                  children: (
                    <>
                      <div style={{ marginBottom: 8, color: 'var(--zm-text-muted)', fontSize: 12 }}>
                        仅展示关联本任务的报工记录；时间跨度最多 6 个月
                      </div>
                      <Space wrap style={{ marginBottom: 16 }}>
                        <RangePicker
                          value={dateRange}
                          onChange={(dates) => {
                            if (dates?.[0] && dates?.[1]) {
                              setDateRange([dates[0], dates[1]])
                              setPage(1)
                            }
                          }}
                          disabledDate={(current) => {
                            if (!dateRange) return false
                            return Math.abs(current.diff(dateRange[0], 'day')) > 180
                          }}
                          placeholder={['开始日期', '结束日期 (最多半年)']}
                        />
                        <Button
                          type="primary"
                          icon={<SearchOutlined />}
                          onClick={handleSearch}
                          style={{ background: 'var(--zm-brand-gradient)', border: 'none' }}
                        >
                          查询
                        </Button>
                      </Space>
                      <Table
                        dataSource={efforts}
                        columns={effortColumns}
                        rowKey="id"
                        loading={effortLoading}
                        size="small"
                        pagination={{
                          current: page,
                          total,
                          pageSize: 20,
                          showTotal: (t) => `共 ${t} 条`,
                          onChange: setPage,
                          showSizeChanger: false,
                        }}
                        scroll={{ x: 800 }}
                        style={{ background: 'transparent' }}
                      />
                    </>
                  ),
                },
                {
                  key: 'detail',
                  label: '明细数据',
                  children: (
                    <TaskFullDetailPanel
                      task={task}
                      personOf={personOf}
                      onViewJson={() => setRawData(taskRawData(task) ?? task)}
                    />
                  ),
                },
              ]}
            />
          </div>
        </>
      )}
      <RawDataModal data={rawData} onClose={() => setRawData(null)} />
    </div>
  )
}

export default TaskDetailPage
