import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Button,
  Card,
  Col,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Row,
  Select,
  Space,
  Statistic,
  Table,
  Tag,
  Typography,
  message,
} from 'antd'
import dayjs from 'dayjs'
import { PlusOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import {
  createZentaoBug,
  deleteZentaoBug,
  listBugs,
  type WorkbenchParams,
  updateZentaoBug,
} from '../../api'
import { WorkbenchStructureSelect } from '../../components/WorkbenchStructureSelect'
import {
  BUG_RESOLUTION_LABEL,
  BUG_STATUS_TAG_COLOR,
  bugResolutionLabel,
  bugStatusLabel,
} from '../Workbench/workbenchDisplay'

const { Text } = Typography

type BugRow = {
  id: number
  title: string
  severity: number
  status: string
  assigned_to: string
  resolved_by: string
  resolution: string
  execution_id: number
  story_id: number
  task_id: number
  opened_date?: string
  resolved_date?: string
  closed_date?: string
  last_edited_date?: string
}

type BugFormValues = {
  title: string
  steps?: string
  execution_id?: number
  assigned_to?: string
  severity?: number
  status?: 'active' | 'resolved' | 'closed' | 'wait' | 'activating'
  resolution?: string
  story_id?: number
  task_id?: number
}

const BUG_STATUS_OPTIONS = [
  { value: 'active', label: '激活' },
  { value: 'resolved', label: '已解决' },
  { value: 'closed', label: '已关闭' },
  { value: 'wait', label: '待确认' },
  { value: 'activating', label: '激活中' },
]

const BUG_SEVERITY_OPTIONS = [
  { value: 1, label: 'P1' },
  { value: 2, label: 'P2' },
  { value: 3, label: 'P3' },
  { value: 4, label: 'P4' },
]

const BUG_RESOLUTION_OPTIONS = Object.entries(BUG_RESOLUTION_LABEL).map(([value, label]) => ({ value, label }))

const severityTagColor = (v: number) => {
  if (v <= 1) return 'red'
  if (v === 2) return 'orange'
  if (v === 3) return 'gold'
  return 'default'
}

function normalizeFormValues(values: BugFormValues): BugFormValues {
  return {
    title: String(values.title ?? '').trim(),
    steps: String(values.steps ?? '').trim() || undefined,
    execution_id: values.execution_id && values.execution_id > 0 ? Number(values.execution_id) : undefined,
    assigned_to: String(values.assigned_to ?? '').trim() || undefined,
    severity: values.severity && values.severity > 0 ? Number(values.severity) : undefined,
    status: values.status ? values.status : undefined,
    resolution: String(values.resolution ?? '').trim() || undefined,
    story_id: values.story_id && values.story_id > 0 ? Number(values.story_id) : undefined,
    task_id: values.task_id && values.task_id > 0 ? Number(values.task_id) : undefined,
  }
}

const MyBugsPage: React.FC = () => {
  const [params, setParams] = useState<WorkbenchParams>({ my_binding: 1 })
  const paramsRef = useRef(params)
  paramsRef.current = params
  const summaryReqID = useRef(0)

  const [rows, setRows] = useState<BugRow[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)
  const [structureKey, setStructureKey] = useState<string | undefined>()
  const [summaryLoading, setSummaryLoading] = useState(false)
  const [summary, setSummary] = useState({
    total: 0,
    active: 0,
    wait: 0,
    p1p2: 0,
  })

  const [createOpen, setCreateOpen] = useState(false)
  const [editingRow, setEditingRow] = useState<BugRow | null>(null)
  const [saving, setSaving] = useState(false)
  const [createForm] = Form.useForm<BugFormValues>()
  const [editForm] = Form.useForm<BugFormValues>()

  const fetchRows = useCallback(async (p: WorkbenchParams, pg: number) => {
    setLoading(true)
    try {
      const res = await listBugs({
        ...p,
        my_binding: 1,
        page: pg,
        page_size: 20,
      })
      setRows((res?.data ?? []) as BugRow[])
      setTotal(Number(res?.total ?? 0))
    } catch (e: any) {
      setRows([])
      setTotal(0)
      message.error(e?.response?.data?.error ?? '加载我的Bug失败')
    } finally {
      setLoading(false)
    }
  }, [])

  const buildSummaryBaseParams = useCallback((p: WorkbenchParams): WorkbenchParams => ({
    my_binding: 1,
    id: p.id,
    execution_id: p.execution_id,
    project_id: p.project_id,
    program_id: p.program_id,
    product_id: p.product_id,
  }), [])

  const fetchSummary = useCallback(async (p: WorkbenchParams) => {
    const reqID = summaryReqID.current + 1
    summaryReqID.current = reqID
    setSummaryLoading(true)
    const base = buildSummaryBaseParams(p)
    try {
      const [allRes, activeRes, waitRes, p1Res, p2Res] = await Promise.all([
        listBugs({ ...base, page: 1, page_size: 1 }),
        listBugs({ ...base, status: 'active', page: 1, page_size: 1 }),
        listBugs({ ...base, status: 'wait', page: 1, page_size: 1 }),
        listBugs({ ...base, severity: '1', page: 1, page_size: 1 }),
        listBugs({ ...base, severity: '2', page: 1, page_size: 1 }),
      ])
      if (summaryReqID.current !== reqID) return
      setSummary({
        total: Number(allRes?.total ?? 0),
        active: Number(activeRes?.total ?? 0),
        wait: Number(waitRes?.total ?? 0),
        p1p2: Number(p1Res?.total ?? 0) + Number(p2Res?.total ?? 0),
      })
    } catch {
      if (summaryReqID.current !== reqID) return
      setSummary({ total: 0, active: 0, wait: 0, p1p2: 0 })
    } finally {
      if (summaryReqID.current === reqID) {
        setSummaryLoading(false)
      }
    }
  }, [buildSummaryBaseParams])

  useEffect(() => {
    void fetchRows(paramsRef.current, page)
  }, [fetchRows, page])

  useEffect(() => {
    void fetchSummary(paramsRef.current)
  }, [fetchSummary])

  const refreshCurrent = useCallback(async () => {
    await Promise.all([
      fetchRows(paramsRef.current, page),
      fetchSummary(paramsRef.current),
    ])
  }, [fetchRows, fetchSummary, page])

  const onSearch = () => {
    void fetchSummary(paramsRef.current)
    if (page !== 1) {
      setPage(1)
      return
    }
    void fetchRows(paramsRef.current, 1)
  }

  const onReset = () => {
    setStructureKey(undefined)
    const next: WorkbenchParams = { my_binding: 1 }
    setParams(next)
    void fetchSummary(next)
    if (page !== 1) {
      setPage(1)
      return
    }
    void fetchRows(next, 1)
  }

  const openCreate = () => {
    createForm.setFieldsValue({
      title: '',
      steps: '',
      execution_id: undefined,
      assigned_to: '',
      severity: 3,
      story_id: undefined,
      task_id: undefined,
    })
    setCreateOpen(true)
  }

  const openEdit = (row: BugRow) => {
    setEditingRow(row)
    editForm.setFieldsValue({
      title: row.title,
      execution_id: row.execution_id > 0 ? row.execution_id : undefined,
      assigned_to: row.assigned_to || '',
      severity: row.severity > 0 ? row.severity : undefined,
      status: (row.status as BugFormValues['status']) || undefined,
      resolution: row.resolution || undefined,
      story_id: row.story_id > 0 ? row.story_id : undefined,
      task_id: row.task_id > 0 ? row.task_id : undefined,
      steps: '',
    })
  }

  const handleCreate = async () => {
    try {
      const values = normalizeFormValues(await createForm.validateFields())
      if (!values.title) {
        message.error('请填写缺陷标题')
        return
      }
      setSaving(true)
      await createZentaoBug(values)
      message.success('缺陷创建成功')
      setCreateOpen(false)
      await refreshCurrent()
    } catch (e: any) {
      if (e?.errorFields) return
      message.error(e?.response?.data?.error ?? '创建缺陷失败')
    } finally {
      setSaving(false)
    }
  }

  const handleUpdate = async () => {
    if (!editingRow) return
    try {
      const values = normalizeFormValues(await editForm.validateFields())
      const payload: Record<string, any> = {}
      if (values.title) payload.title = values.title
      if (values.steps) payload.steps = values.steps
      if (values.execution_id) payload.execution_id = values.execution_id
      if (values.assigned_to) payload.assigned_to = values.assigned_to
      if (values.severity) payload.severity = values.severity
      if (values.status) payload.status = values.status
      if (values.resolution) payload.resolution = values.resolution
      if (values.story_id) payload.story_id = values.story_id
      if (values.task_id) payload.task_id = values.task_id
      if (Object.keys(payload).length === 0) {
        message.info('没有可更新的内容')
        return
      }
      setSaving(true)
      await updateZentaoBug(editingRow.id, payload)
      message.success('缺陷更新成功')
      setEditingRow(null)
      await refreshCurrent()
    } catch (e: any) {
      if (e?.errorFields) return
      message.error(e?.response?.data?.error ?? '更新缺陷失败')
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (bugID: number) => {
    try {
      await deleteZentaoBug(bugID)
      message.success('缺陷已删除')
      await refreshCurrent()
    } catch (e: any) {
      message.error(e?.response?.data?.error ?? '删除缺陷失败')
    }
  }

  const columns = useMemo(
    () => [
      { title: 'ID', dataIndex: 'id', width: 88 },
      {
        title: '缺陷标题',
        dataIndex: 'title',
        render: (v: string) => <Text style={{ color: 'var(--zm-text-primary)' }}>{v || '-'}</Text>,
      },
      {
        title: '严重级别',
        dataIndex: 'severity',
        width: 96,
        render: (v: number) => <Tag color={severityTagColor(Number(v ?? 0))}>P{Number(v ?? 0)}</Tag>,
      },
      {
        title: '状态',
        dataIndex: 'status',
        width: 110,
        render: (v: string) => <Tag color={BUG_STATUS_TAG_COLOR[v] ?? 'default'}>{bugStatusLabel(v)}</Tag>,
      },
      {
        title: '解决方案',
        dataIndex: 'resolution',
        width: 140,
        render: (v: string) => <Text style={{ color: 'var(--zm-text-secondary)' }}>{bugResolutionLabel(v)}</Text>,
      },
      {
        title: '迭代ID',
        dataIndex: 'execution_id',
        width: 96,
        render: (v: number) => (v > 0 ? v : '-'),
      },
      {
        title: '任务ID',
        dataIndex: 'task_id',
        width: 90,
        render: (v: number) => (v > 0 ? v : '-'),
      },
      {
        title: '需求ID',
        dataIndex: 'story_id',
        width: 90,
        render: (v: number) => (v > 0 ? v : '-'),
      },
      {
        title: '最后编辑',
        dataIndex: 'last_edited_date',
        width: 160,
        render: (v: string) => (v ? dayjs(v).format('YYYY-MM-DD HH:mm') : '-'),
      },
      {
        title: '操作',
        key: 'actions',
        width: 136,
        fixed: 'right' as const,
        render: (_: any, row: BugRow) => (
          <Space size={4}>
            <Button size="small" onClick={() => openEdit(row)}>编辑</Button>
            <Popconfirm
              title="确认删除该缺陷吗？"
              description="若禅道实例不支持物理删除，会回退为关闭该缺陷。"
              okText="删除"
              cancelText="取消"
              onConfirm={() => handleDelete(row.id)}
            >
              <Button size="small" danger>删除</Button>
            </Popconfirm>
          </Space>
        ),
      },
    ],
    [],
  )

  return (
    <div>
      <div style={{ marginBottom: 16 }}>
        <Text style={{ color: 'var(--zm-text-primary)', fontSize: 18, fontWeight: 600 }}>我的Bug</Text>
        <div>
          <Text style={{ color: 'var(--zm-text-muted)', fontSize: 12 }}>
            仅展示当前绑定禅道账号下“归属给我”的缺陷数据（个人口径）。
          </Text>
        </div>
      </div>

      <div
        style={{
          background: 'var(--zm-bg-surface)',
          border: '1px solid var(--zm-border-subtle)',
          borderRadius: 12,
          padding: '16px 20px',
        }}
      >
        <Row gutter={12} style={{ marginBottom: 16 }}>
          <Col xs={24} sm={12} md={6}>
            <Card size="small" styles={{ body: { padding: '12px 14px' } }}>
              <Statistic title="缺陷总数" value={summary.total} loading={summaryLoading} />
            </Card>
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Card size="small" styles={{ body: { padding: '12px 14px' } }}>
              <Statistic title="激活" value={summary.active} loading={summaryLoading} />
            </Card>
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Card size="small" styles={{ body: { padding: '12px 14px' } }}>
              <Statistic title="待确认" value={summary.wait} loading={summaryLoading} />
            </Card>
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Card size="small" styles={{ body: { padding: '12px 14px' } }}>
              <Statistic title="高优先(P1/P2)" value={summary.p1p2} loading={summaryLoading} />
            </Card>
          </Col>
        </Row>

        <Space wrap style={{ marginBottom: 16 }}>
          <Input
            allowClear
            placeholder="缺陷ID"
            style={{ width: 140 }}
            value={params.id?.toString() ?? ''}
            onChange={(e) => {
              const raw = e.target.value.trim()
              if (!raw) {
                setParams((p) => ({ ...p, id: undefined }))
                return
              }
              const n = Number(raw)
              if (!Number.isFinite(n) || n <= 0) {
                setParams((p) => ({ ...p, id: undefined }))
                return
              }
              setParams((p) => ({ ...p, id: Math.trunc(n) }))
            }}
          />
          <Select
            allowClear
            placeholder="状态"
            style={{ width: 140 }}
            value={params.status}
            options={BUG_STATUS_OPTIONS}
            onChange={(v) => setParams((p) => ({ ...p, status: v }))}
          />
          <Select
            allowClear
            placeholder="严重级别"
            style={{ width: 140 }}
            value={params.severity}
            options={BUG_SEVERITY_OPTIONS.map((x) => ({ value: String(x.value), label: x.label }))}
            onChange={(v) => setParams((p) => ({ ...p, severity: v }))}
          />
          <WorkbenchStructureSelect
            value={structureKey}
            onChange={(key, meta) => {
              setStructureKey(key)
              setParams((p) => {
                const next: WorkbenchParams = {
                  ...p,
                  execution_id: undefined,
                  project_id: undefined,
                  program_id: undefined,
                  product_id: undefined,
                }
                if (meta?.type === 'execution') next.execution_id = meta.id
                if (meta?.type === 'project') next.project_id = meta.id
                if (meta?.type === 'program') next.program_id = meta.id
                if (meta?.type === 'product') next.product_id = meta.id
                return next
              })
            }}
          />
          <Button
            type="primary"
            icon={<SearchOutlined />}
            onClick={onSearch}
            style={{ background: 'var(--zm-brand-gradient)', border: 'none' }}
          >
            查询
          </Button>
          <Button icon={<ReloadOutlined />} onClick={onReset}>
            重置
          </Button>
          <Button type="dashed" icon={<PlusOutlined />} onClick={openCreate}>
            新建缺陷
          </Button>
        </Space>

        <Table<BugRow>
          rowKey="id"
          size="small"
          loading={loading}
          dataSource={rows}
          columns={columns as any}
          scroll={{ x: 1360 }}
          pagination={{
            current: page,
            pageSize: 20,
            total,
            showSizeChanger: false,
            showTotal: (t) => `共 ${t} 条`,
            onChange: setPage,
          }}
        />
      </div>

      <Modal
        title="新建缺陷"
        open={createOpen}
        onCancel={() => setCreateOpen(false)}
        onOk={() => void handleCreate()}
        confirmLoading={saving}
        destroyOnHidden
      >
        <Form<BugFormValues> form={createForm} layout="vertical">
          <Form.Item label="缺陷标题" name="title" rules={[{ required: true, message: '请填写缺陷标题' }]}>
            <Input placeholder="请输入缺陷标题" />
          </Form.Item>
          <Form.Item label="复现步骤/描述" name="steps">
            <Input.TextArea rows={3} placeholder="可选" />
          </Form.Item>
          <Row gutter={12}>
            <Col span={12}>
              <Form.Item label="严重级别" name="severity">
                <Select allowClear options={BUG_SEVERITY_OPTIONS} placeholder="可选" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item label="指派给" name="assigned_to">
                <Input placeholder="禅道账号，可选" />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={12}>
            <Col span={8}>
              <Form.Item label="迭代ID" name="execution_id">
                <InputNumber min={1} style={{ width: '100%' }} placeholder="可选" />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item label="需求ID" name="story_id">
                <InputNumber min={1} style={{ width: '100%' }} placeholder="可选" />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item label="任务ID" name="task_id">
                <InputNumber min={1} style={{ width: '100%' }} placeholder="可选" />
              </Form.Item>
            </Col>
          </Row>
        </Form>
      </Modal>

      <Modal
        title={editingRow ? `编辑缺陷 #${editingRow.id}` : '编辑缺陷'}
        open={!!editingRow}
        onCancel={() => setEditingRow(null)}
        onOk={() => void handleUpdate()}
        confirmLoading={saving}
        destroyOnHidden
      >
        <Form<BugFormValues> form={editForm} layout="vertical">
          <Form.Item label="缺陷标题" name="title" rules={[{ required: true, message: '请填写缺陷标题' }]}>
            <Input placeholder="请输入缺陷标题" />
          </Form.Item>
          <Form.Item label="复现步骤/描述" name="steps">
            <Input.TextArea rows={3} placeholder="可选" />
          </Form.Item>
          <Row gutter={12}>
            <Col span={12}>
              <Form.Item label="状态" name="status">
                <Select allowClear options={BUG_STATUS_OPTIONS} placeholder="可选" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item label="解决方案" name="resolution">
                <Select allowClear options={BUG_RESOLUTION_OPTIONS} placeholder="可选" />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={12}>
            <Col span={12}>
              <Form.Item label="严重级别" name="severity">
                <Select allowClear options={BUG_SEVERITY_OPTIONS} placeholder="可选" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item label="指派给" name="assigned_to">
                <Input placeholder="禅道账号，可选" />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={12}>
            <Col span={8}>
              <Form.Item label="迭代ID" name="execution_id">
                <InputNumber min={1} style={{ width: '100%' }} placeholder="可选" />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item label="需求ID" name="story_id">
                <InputNumber min={1} style={{ width: '100%' }} placeholder="可选" />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item label="任务ID" name="task_id">
                <InputNumber min={1} style={{ width: '100%' }} placeholder="可选" />
              </Form.Item>
            </Col>
          </Row>
        </Form>
      </Modal>
    </div>
  )
}

export default MyBugsPage
