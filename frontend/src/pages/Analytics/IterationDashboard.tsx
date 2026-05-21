import React, { useEffect, useMemo, useState } from 'react'
import { Alert, Card, Col, DatePicker, Drawer, Row, Select, Space, Statistic, Table, Tag, Typography, message } from 'antd'
import dayjs, { Dayjs } from 'dayjs'
import ReactECharts from 'echarts-for-react'
import { getIterationOverview, getIterationBurndown, getIterationCFD, getIterationCycleTime, getIterationScopeChange, listTasks, listExecutions } from '../../api'

const { RangePicker } = DatePicker
const { Text } = Typography

const IterationDashboardPage: React.FC = () => {
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
    let cancelled = false
    listExecutions({ page: 1, page_size: 200 })
      .then((res) => {
        if (cancelled) return
        const rows = res.data ?? []
        const opts = rows.map((e: any) => ({ value: e.id, label: e.name ? `${e.id} · ${e.name}` : String(e.id) }))
        setExecutionOptions(opts)
        if (!executionId && opts.length > 0) setExecutionId(opts[0].value)
      })
      .catch(() => {
        if (!cancelled) setExecutionOptions([])
      })
    return () => {
      cancelled = true
    }
  }, [])

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

    setOverviewLoading(true)
    getIterationOverview({ execution_id: executionId, date_from: dateFrom, date_to: dateTo })
      .then((d) => !cancelled && setOverview(d))
      .catch((e) => !cancelled && message.error(e.response?.data?.error ?? '迭代概览获取失败'))
      .finally(() => !cancelled && setOverviewLoading(false))

    setBurndownLoading(true)
    getIterationBurndown({ execution_id: executionId, date_from: dateFrom, date_to: dateTo })
      .then((d) => !cancelled && setBurndown(d))
      .catch((e) => !cancelled && message.error(e.response?.data?.error ?? '燃尽获取失败'))
      .finally(() => !cancelled && setBurndownLoading(false))

    setCfdLoading(true)
    getIterationCFD({ execution_id: executionId, date_from: dateFrom, date_to: dateTo })
      .then((d) => !cancelled && setCfd(d))
      .catch((e) => !cancelled && message.error(e.response?.data?.error ?? 'CFD获取失败'))
      .finally(() => !cancelled && setCfdLoading(false))

    setCycleLoading(true)
    getIterationCycleTime({ execution_id: executionId, date_from: dateFrom, date_to: dateTo })
      .then((d) => !cancelled && setCycle(d))
      .catch((e) => !cancelled && message.error(e.response?.data?.error ?? '周期时间获取失败'))
      .finally(() => !cancelled && setCycleLoading(false))

    setScopeLoading(true)
    getIterationScopeChange({ execution_id: executionId, date_from: dateFrom, date_to: dateTo })
      .then((d) => !cancelled && setScope(d))
      .catch((e) => !cancelled && message.error(e.response?.data?.error ?? '范围变更获取失败'))
      .finally(() => !cancelled && setScopeLoading(false))

    return () => {
      cancelled = true
    }
  }, [executionId, dateFrom, dateTo])

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

  return (
    <div>
      <div style={{ marginBottom: 16 }}>
        <Text style={{ color: 'var(--zm-text-primary)', fontSize: 18, fontWeight: 600 }}>迭代看板</Text>
        <Space style={{ marginLeft: 12 }} wrap>
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

      <Card style={{ marginBottom: 16, background: 'var(--zm-bg-surface)', border: '1px solid var(--zm-border-subtle)', borderRadius: 12 }}>
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
          {execMeta?.begin ? <Tag>开始 {execMeta.begin}</Tag> : null}
          {execMeta?.end ? <Tag>结束 {execMeta.end}</Tag> : null}
        </Space>
      </Card>

      <Row gutter={[16, 16]}>
        <Col span={24}>
          <Card
            loading={overviewLoading}
            style={{ background: 'var(--zm-bg-surface)', border: '1px solid var(--zm-border-subtle)', borderRadius: 12 }}
            title="迭代概览"
          >
            <Row gutter={12}>
              <Col>
                <Statistic title="健康度" value={healthScore} suffix="/100" />
              </Col>
              <Col>
                <Statistic title="任务总数" value={tasks?.total ?? 0} />
              </Col>
              <Col>
                <Statistic title="未完成" value={tasks?.open ?? 0} />
              </Col>
              <Col>
                <Statistic title="预估总计(h)" value={tasks?.estimate_sum ?? 0} />
              </Col>
              <Col>
                <Statistic title="报工总计(h)" value={efforts?.total_hours ?? 0} />
              </Col>
              <Col>
                <Statistic title="Bug总数" value={bugs?.total ?? 0} />
              </Col>
            </Row>
            {cycle?.note ? (
              <Alert style={{ marginTop: 12 }} type="info" showIcon message={`周期时间P85：${cycle?.cycle_time_hours?.p85 ?? 0}h；前置时间P85：${cycle?.lead_time_hours?.p85 ?? 0}h`} />
            ) : null}
          </Card>
        </Col>

        <Col span={12}>
          <Card title="燃尽（近似）" loading={burndownLoading} style={{ background: 'var(--zm-bg-surface)', border: '1px solid var(--zm-border-subtle)', borderRadius: 12 }}>
            <ReactECharts option={burndownOption} style={{ height: 320 }} />
          </Card>
        </Col>
        <Col span={12}>
          <Card title="累计流图 CFD（近似）" loading={cfdLoading} style={{ background: 'var(--zm-bg-surface)', border: '1px solid var(--zm-border-subtle)', borderRadius: 12 }}>
            <ReactECharts option={cfdOption} style={{ height: 320 }} />
            {cfd?.note ? <div style={{ marginTop: 8, color: 'var(--zm-text-muted)', fontSize: 12 }}>{cfd.note}</div> : null}
          </Card>
        </Col>

        <Col span={24}>
          <Card
            title="范围变更（execution字段变更）"
            loading={scopeLoading}
            style={{ background: 'var(--zm-bg-surface)', border: '1px solid var(--zm-border-subtle)', borderRadius: 12 }}
          >
            {scope?.note ? <div style={{ marginBottom: 8, color: 'var(--zm-text-muted)', fontSize: 12 }}>{scope.note}</div> : null}
            <Table
              rowKey={(r: any) => `${r.time}-${r.object_type}-${r.object_id}-${r.field}`}
              size="small"
              dataSource={scope?.items ?? []}
              pagination={false}
              columns={[
                { title: '时间', dataIndex: 'time', width: 200 },
                { title: '类型', dataIndex: 'object_type', width: 90 },
                { title: 'ID', dataIndex: 'object_id', width: 90 },
                { title: '字段', dataIndex: 'field', width: 120 },
                { title: '旧值', dataIndex: 'old' },
                { title: '新值', dataIndex: 'new' },
                { title: '操作者', dataIndex: 'actor', width: 120 },
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
          pagination={false}
          columns={[
            { title: 'ID', dataIndex: 'id', width: 80 },
            { title: '任务名', dataIndex: 'name' },
            { title: '状态', dataIndex: 'status', width: 100, render: (v: string) => <Tag>{v}</Tag> },
            { title: '预估(h)', dataIndex: 'estimate', width: 90 },
            { title: '消耗(h)', dataIndex: 'consumed', width: 90 },
          ]}
          style={{ background: 'transparent' }}
        />
      </Drawer>
    </div>
  )
}

export default IterationDashboardPage

