import React, { useEffect, useMemo, useState } from 'react'
import { Alert, Button, Card, Col, DatePicker, Drawer, Progress, Row, Select, Space, Statistic, Table, Tag, Typography, message } from 'antd'
import dayjs, { Dayjs } from 'dayjs'
import ReactECharts from 'echarts-for-react'
import { getIterationOverview, getIterationBurndown, getIterationCFD, getIterationCycleTime, getIterationScopeChange, listTasks, listExecutions } from '../../api'
import { GroupSelect } from '../../components/GroupSelect'
import { useAuthStore } from '../../store/auth'

const { RangePicker } = DatePicker
const { Text } = Typography

const CARD_STYLE: React.CSSProperties = {
  background: 'var(--zm-bg-surface)',
  border: '1px solid var(--zm-border-subtle)',
  borderRadius: 8,
}

function safeRate(n: number, d: number) {
  if (!Number.isFinite(n) || !Number.isFinite(d) || d <= 0) return 0
  return n / d
}

const IterationDashboardPage: React.FC = () => {
  const [groupId, setGroupId] = useState<number | undefined>()
  const [groupName, setGroupName] = useState('')
  const me = useAuthStore((s) => s.me)
  const dataScope = (me?.user?.data_scope ?? '').toUpperCase()
  const defaultGroupId = me?.user?.default_group_id ?? undefined

  const [executionId, setExecutionId] = useState<number | undefined>()
  const [executionOptions, setExecutionOptions] = useState<{ value: number; label: string }[]>([])

  const [range, setRange] = useState<[Dayjs, Dayjs]>(() => [dayjs().add(-29, 'day'), dayjs()])
  const dateFrom = useMemo(() => range[0].format('YYYY-MM-DD'), [range])
  const dateTo = useMemo(() => range[1].format('YYYY-MM-DD'), [range])

  const [overviewLoading, setOverviewLoading] = useState(false)
  const [overview, setOverview] = useState<any | null>(null)

  const [burndownLoading, setBurndownLoading] = useState(false)
  const [burndown, setBurndown] = useState<any | null>(null)

  const [cfdLoading, setCfdLoading] = useState(false)
  const [cfd, setCfd] = useState<any | null>(null)

  const [cycleLoading, setCycleLoading] = useState(false)
  const [cycle, setCycle] = useState<any | null>(null)

  const [scopeLoading, setScopeLoading] = useState(false)
  const [scope, setScope] = useState<any | null>(null)

  const [taskDrawer, setTaskDrawer] = useState<{ open: boolean; title: string; params: any }>({
    open: false,
    title: '',
    params: null,
  })
  const [taskRows, setTaskRows] = useState<any[]>([])
  const [taskLoading, setTaskLoading] = useState(false)

  useEffect(() => {
    if (dataScope === 'GROUP' && defaultGroupId && !groupId) {
      setGroupId(defaultGroupId)
    }
  }, [dataScope, defaultGroupId, groupId])

  useEffect(() => {
    if (dataScope === 'GROUP' && !groupId) return
    let cancelled = false
    setExecutionId(undefined)
    listExecutions({ group_id: groupId, page: 1, page_size: 200 })
      .then((res) => {
        if (cancelled) return
        const rows = res.data ?? []
        const opts = rows.map((e: any) => ({ value: e.id, label: e.name ? `${e.id} · ${e.name}` : String(e.id) }))
        setExecutionOptions(opts)
        if (opts.length > 0) setExecutionId(opts[0].value)
      })
      .catch(() => {
        if (!cancelled) setExecutionOptions([])
      })
    return () => {
      cancelled = true
    }
  }, [dataScope, groupId])

  useEffect(() => {
    if (!executionId) {
      setOverview(null)
      setBurndown(null)
      setCfd(null)
      setCycle(null)
      setScope(null)
      return
    }
    let cancelled = false
    const params = { group_id: groupId, execution_id: executionId, date_from: dateFrom, date_to: dateTo }

    setOverviewLoading(true)
    getIterationOverview(params)
      .then((d) => !cancelled && setOverview(d))
      .catch((e) => !cancelled && message.error(e.response?.data?.error ?? '迭代概览获取失败'))
      .finally(() => !cancelled && setOverviewLoading(false))

    setBurndownLoading(true)
    getIterationBurndown(params)
      .then((d) => !cancelled && setBurndown(d))
      .catch((e) => !cancelled && message.error(e.response?.data?.error ?? '燃尽获取失败'))
      .finally(() => !cancelled && setBurndownLoading(false))

    setCfdLoading(true)
    getIterationCFD(params)
      .then((d) => !cancelled && setCfd(d))
      .catch((e) => !cancelled && message.error(e.response?.data?.error ?? 'CFD获取失败'))
      .finally(() => !cancelled && setCfdLoading(false))

    setCycleLoading(true)
    getIterationCycleTime(params)
      .then((d) => !cancelled && setCycle(d))
      .catch((e) => !cancelled && message.error(e.response?.data?.error ?? '周期时间获取失败'))
      .finally(() => !cancelled && setCycleLoading(false))

    setScopeLoading(true)
    getIterationScopeChange(params)
      .then((d) => !cancelled && setScope(d))
      .catch((e) => !cancelled && message.error(e.response?.data?.error ?? '范围变更获取失败'))
      .finally(() => !cancelled && setScopeLoading(false))

    return () => {
      cancelled = true
    }
  }, [groupId, executionId, dateFrom, dateTo])

  useEffect(() => {
    if (!taskDrawer.open || !taskDrawer.params) return
    setTaskLoading(true)
    listTasks(taskDrawer.params)
      .then((res) => setTaskRows(res.data ?? []))
      .catch((e) => message.error(e.response?.data?.error ?? '任务明细获取失败'))
      .finally(() => setTaskLoading(false))
  }, [taskDrawer])

  const burndownOption = useMemo(() => {
    const s = burndown?.series ?? []
    return {
      tooltip: { trigger: 'axis' },
      legend: { top: 0, textStyle: { color: 'var(--zm-text-secondary)' } },
      grid: { left: 48, right: 16, top: 32, bottom: 32 },
      xAxis: { type: 'category', data: s.map((x: any) => x.day), axisLabel: { rotate: 45 } },
      yAxis: { type: 'value' },
      series: [
        { name: '剩余预估(h)', type: 'line', smooth: true, data: s.map((x: any) => x.open_estimate) },
        { name: '累计完成数', type: 'line', smooth: true, data: s.map((x: any) => x.done_count) },
      ],
    }
  }, [burndown])

  const cfdOption = useMemo(() => {
    const s = cfd?.series ?? []
    return {
      tooltip: { trigger: 'axis' },
      legend: { top: 0, textStyle: { color: 'var(--zm-text-secondary)' } },
      grid: { left: 48, right: 16, top: 32, bottom: 32 },
      xAxis: { type: 'category', data: s.map((x: any) => x.day), axisLabel: { rotate: 45 } },
      yAxis: { type: 'value' },
      series: [
        { name: 'Todo', type: 'line', stack: 's', areaStyle: {}, data: s.map((x: any) => x.todo) },
        { name: 'Doing', type: 'line', stack: 's', areaStyle: {}, data: s.map((x: any) => x.doing) },
        { name: 'Done', type: 'line', stack: 's', areaStyle: {}, data: s.map((x: any) => x.done) },
      ],
    }
  }, [cfd])

  const healthScore = Number(overview?.health?.score ?? 0)
  const execMeta = overview?.execution
  const tasks = overview?.tasks
  const bugs = overview?.bugs
  const efforts = overview?.efforts
  const completionRate = safeRate(Number(tasks?.done ?? 0), Number(tasks?.total ?? 0))
  const remainingEstimateRate = safeRate(Number(tasks?.estimate_sum_open ?? 0), Number(tasks?.estimate_sum ?? 0))
  const openBugRate = safeRate(Number(bugs?.open ?? 0), Number(bugs?.total ?? 0))
  const scopeChangeCount = scope?.items?.length ?? 0
  const cycleP85 = Number(cycle?.cycle_time_hours?.p85 ?? 0)
  const leadP85 = Number(cycle?.lead_time_hours?.p85 ?? 0)
  const hasDeliveryRisk = healthScore > 0 && (healthScore < 65 || remainingEstimateRate > 0.45 || scopeChangeCount >= 5 || Number(bugs?.open ?? 0) > 0)

  const openTaskDrawer = (title: string, extraParams: any = {}) => {
    setTaskDrawer({
      open: true,
      title,
      params: { group_id: groupId, execution_id: executionId, page: 1, page_size: 200, ...extraParams },
    })
  }

  return (
    <div>
      <div style={{ marginBottom: 16 }}>
        <Text style={{ color: 'var(--zm-text-primary)', fontSize: 18, fontWeight: 600 }}>迭代看板</Text>
        <Space style={{ marginLeft: 12 }} wrap>
          <GroupSelect
            value={groupId}
            onChange={(id, name) => {
              setGroupId(id)
              setGroupName(name)
            }}
            disabled={dataScope === 'GROUP'}
            allowedGroupIds={dataScope === 'GROUP' && defaultGroupId ? [defaultGroupId] : undefined}
          />
          {groupId ? <Tag color="purple">{groupName || `小组 ${groupId}`}</Tag> : <Tag>全部可见数据</Tag>}
          <RangePicker
            value={range}
            onChange={(v) => {
              if (!v || !v[0] || !v[1]) return
              setRange([v[0], v[1]])
            }}
            disabledDate={(current) => Math.abs(current.diff(range[0], 'day')) > 180}
          />
        </Space>
      </div>

      <Card style={{ ...CARD_STYLE, marginBottom: 16 }}>
        <Space wrap>
          <Text style={{ color: 'var(--zm-text-muted)' }}>迭代</Text>
          <Select
            showSearch
            optionFilterProp="label"
            placeholder="请选择迭代"
            style={{ width: 360 }}
            value={executionId}
            options={executionOptions}
            onChange={(v) => setExecutionId(v)}
          />
          {execMeta?.name ? <Tag color="blue">{execMeta.name}</Tag> : null}
          {execMeta?.status ? <Tag>{execMeta.status}</Tag> : null}
          {execMeta?.begin ? <Tag>开始 {execMeta.begin}</Tag> : null}
          {execMeta?.end ? <Tag>结束 {execMeta.end}</Tag> : null}
        </Space>
      </Card>

      <Row gutter={[16, 16]}>
        <Col span={24}>
          <Card
            loading={overviewLoading}
            style={CARD_STYLE}
            title="管理摘要"
            extra={
              <Button size="small" onClick={() => openTaskDrawer('迭代任务明细')}>
                任务明细
              </Button>
            }
          >
            <Row gutter={[16, 12]} align="middle">
              <Col xs={24} md={5}>
                <Progress type="dashboard" percent={Math.round(healthScore)} size={108} />
              </Col>
              <Col xs={12} md={3}>
                <Statistic title="任务总数" value={tasks?.total ?? 0} />
              </Col>
              <Col xs={12} md={3}>
                <Statistic title="未完成" value={tasks?.open ?? 0} />
              </Col>
              <Col xs={12} md={3}>
                <Statistic title="完成率" value={(completionRate * 100).toFixed(1)} suffix="%" />
              </Col>
              <Col xs={12} md={3}>
                <Statistic title="剩余预估(h)" value={tasks?.estimate_sum_open ?? 0} />
              </Col>
              <Col xs={12} md={3}>
                <Statistic title="报工总计(h)" value={efforts?.total_hours ?? 0} />
              </Col>
              <Col xs={12} md={3}>
                <Statistic title="未关闭Bug" value={bugs?.open ?? 0} />
              </Col>
              <Col xs={12} md={4}>
                <Statistic title="范围变更" value={scopeChangeCount} />
              </Col>
            </Row>
            <Row gutter={[16, 8]} style={{ marginTop: 12 }}>
              <Col xs={24} md={8}>
                <Text style={{ color: 'var(--zm-text-muted)' }}>剩余预估占比：{(remainingEstimateRate * 100).toFixed(1)}%</Text>
              </Col>
              <Col xs={24} md={8}>
                <Text style={{ color: 'var(--zm-text-muted)' }}>Bug未关闭占比：{(openBugRate * 100).toFixed(1)}%</Text>
              </Col>
              <Col xs={24} md={8}>
                <Text style={{ color: 'var(--zm-text-muted)' }}>P85周期/前置：{cycleP85}h / {leadP85}h</Text>
              </Col>
            </Row>
            {hasDeliveryRisk ? (
              <Alert
                style={{ marginTop: 12 }}
                type="warning"
                showIcon
                message="当前迭代存在交付风险信号：请优先核对剩余预估、未关闭Bug和范围变更是否仍可控。"
              />
            ) : (
              <Alert
                style={{ marginTop: 12 }}
                type="success"
                showIcon
                message="当前迭代的完成率、缺陷和范围变更处于可跟踪状态。"
              />
            )}
            {cycle?.note ? (
              <Alert style={{ marginTop: 12 }} type="info" showIcon message={cycle.note} />
            ) : null}
          </Card>
        </Col>

        <Col span={12}>
          <Card title="燃尽与完成趋势" loading={burndownLoading} style={CARD_STYLE}>
            <ReactECharts option={burndownOption} style={{ height: 320 }} />
          </Card>
        </Col>
        <Col span={12}>
          <Card title="累计流图 CFD" loading={cfdLoading} style={CARD_STYLE}>
            <ReactECharts option={cfdOption} style={{ height: 320 }} />
            {cfd?.note ? <div style={{ marginTop: 8, color: 'var(--zm-text-muted)', fontSize: 12 }}>{cfd.note}</div> : null}
          </Card>
        </Col>

        <Col span={24}>
          <Card
            title="范围变更审计"
            loading={scopeLoading}
            style={CARD_STYLE}
          >
            {scope?.note ? <div style={{ marginBottom: 8, color: 'var(--zm-text-muted)', fontSize: 12 }}>{scope.note}</div> : null}
            <Table
              rowKey={(r: any) => `${r.time}-${r.object_type}-${r.object_id}-${r.field}`}
              size="small"
              dataSource={scope?.items ?? []}
              pagination={{ pageSize: 10, showSizeChanger: true }}
              scroll={{ x: 980 }}
              columns={[
                { title: '时间', dataIndex: 'time', width: 180, render: (v: string) => (v ? dayjs(v).format('YYYY-MM-DD HH:mm') : '-') },
                { title: '类型', dataIndex: 'object_type', width: 90 },
                { title: 'ID', dataIndex: 'object_id', width: 90 },
                { title: '字段', dataIndex: 'field', width: 120 },
                { title: '旧值', dataIndex: 'old' },
                { title: '新值', dataIndex: 'new' },
                { title: '操作者', dataIndex: 'actor', width: 120 },
                {
                  title: '',
                  width: 90,
                  render: (_: any, r: any) =>
                    r.object_type === 'task' ? (
                      <a onClick={() => openTaskDrawer(`任务 #${r.object_id}`, { id: r.object_id })} style={{ color: 'var(--zm-primary-text)' }}>
                        查看
                      </a>
                    ) : null,
                },
              ]}
              style={{ background: 'transparent' }}
            />
          </Card>
        </Col>
      </Row>

      <Drawer
        open={taskDrawer.open}
        width={960}
        onClose={() => setTaskDrawer({ open: false, title: '', params: null })}
        title={<Text style={{ color: 'var(--zm-text-primary)' }}>{taskDrawer.title}</Text>}
        styles={{ body: { background: 'var(--zm-bg-canvas)' }, header: { background: 'var(--zm-bg-canvas)' } }}
      >
        <Table
          rowKey="id"
          size="small"
          loading={taskLoading}
          dataSource={taskRows}
          pagination={{ pageSize: 20, showSizeChanger: true }}
          scroll={{ x: 980 }}
          columns={[
            { title: 'ID', dataIndex: 'id', width: 80 },
            { title: '任务名', dataIndex: 'name' },
            { title: '指派给', dataIndex: 'assigned_to', width: 120 },
            { title: '状态', dataIndex: 'status', width: 100, render: (v: string) => <Tag>{v}</Tag> },
            { title: '预估(h)', dataIndex: 'estimate', width: 90 },
            { title: '消耗(h)', dataIndex: 'consumed', width: 90 },
            { title: '截止', dataIndex: 'deadline', width: 120, render: (v: string) => v || '-' },
          ]}
          style={{ background: 'transparent' }}
        />
      </Drawer>
    </div>
  )
}

export default IterationDashboardPage
