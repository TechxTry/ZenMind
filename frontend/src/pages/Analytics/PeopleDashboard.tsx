import React, { useEffect, useMemo, useState } from 'react'
import { Alert, Card, Col, DatePicker, Drawer, Progress, Row, Select, Space, Statistic, Table, Tag, Typography, message } from 'antd'
import dayjs, { Dayjs } from 'dayjs'
import ReactECharts from 'echarts-for-react'
import { getPeopleOverview, getPeopleWipTrend, getPeopleThroughput, getPeopleBottleneck, listTasks } from '../../api'
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

const PeopleDashboardPage: React.FC = () => {
  const [groupId, setGroupId] = useState<number | undefined>()
  const [groupName, setGroupName] = useState('')
  const me = useAuthStore((s) => s.me)
  const dataScope = (me?.user?.data_scope ?? '').toUpperCase()
  const defaultGroupId = me?.user?.default_group_id ?? undefined

  const [range, setRange] = useState<[Dayjs, Dayjs]>(() => [dayjs().add(-29, 'day'), dayjs()])
  const dateFrom = useMemo(() => range[0].format('YYYY-MM-DD'), [range])
  const dateTo = useMemo(() => range[1].format('YYYY-MM-DD'), [range])

  const [overviewLoading, setOverviewLoading] = useState(false)
  const [overview, setOverview] = useState<any | null>(null)

  const [wipLoading, setWipLoading] = useState(false)
  const [wip, setWip] = useState<any | null>(null)

  const [tpLoading, setTpLoading] = useState(false)
  const [tp, setTp] = useState<any | null>(null)

  const [bnLoading, setBnLoading] = useState(false)
  const [bn, setBn] = useState<any | null>(null)

  const [account, setAccount] = useState<string | undefined>()

  const [taskDrawer, setTaskDrawer] = useState<{ open: boolean; title: string; params: any }>({ open: false, title: '', params: null })
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
    const params = { group_id: groupId, date_from: dateFrom, date_to: dateTo }

    setOverviewLoading(true)
    getPeopleOverview(params)
      .then((d) => !cancelled && setOverview(d))
      .catch((e) => !cancelled && message.error(e.response?.data?.error ?? '人员概览获取失败'))
      .finally(() => !cancelled && setOverviewLoading(false))

    setWipLoading(true)
    getPeopleWipTrend(params)
      .then((d) => !cancelled && setWip(d))
      .catch((e) => !cancelled && message.error(e.response?.data?.error ?? 'WIP趋势获取失败'))
      .finally(() => !cancelled && setWipLoading(false))

    setTpLoading(true)
    getPeopleThroughput(params)
      .then((d) => !cancelled && setTp(d))
      .catch((e) => !cancelled && message.error(e.response?.data?.error ?? '吞吐获取失败'))
      .finally(() => !cancelled && setTpLoading(false))

    setBnLoading(true)
    getPeopleBottleneck(params)
      .then((d) => !cancelled && setBn(d))
      .catch((e) => !cancelled && message.error(e.response?.data?.error ?? '瓶颈获取失败'))
      .finally(() => !cancelled && setBnLoading(false))

    return () => {
      cancelled = true
    }
  }, [dataScope, groupId, dateFrom, dateTo])

  useEffect(() => {
    if (!taskDrawer.open || !taskDrawer.params) return
    setTaskLoading(true)
    listTasks(taskDrawer.params)
      .then((res) => setTaskRows(res.data ?? []))
      .catch((e) => message.error(e.response?.data?.error ?? '任务明细获取失败'))
      .finally(() => setTaskLoading(false))
  }, [taskDrawer])

  const memberOptions = useMemo(() => {
    const rows = overview?.rows ?? []
    return rows.map((r: any) => ({
      value: r.account,
      label: r.realname?.trim() ? `${r.realname}（${r.account}）` : r.account,
    }))
  }, [overview])

  const visibleRows = useMemo(
    () => (overview?.rows ?? []).filter((r: any) => (account ? r.account === account : true)),
    [account, overview],
  )

  const totals = useMemo(() => {
    return visibleRows.reduce(
      (acc: any, r: any) => {
        acc.openTaskCount += Number(r.open_task_count ?? 0)
        acc.wipCount += Number(r.wip_count ?? 0)
        acc.openEstimate += Number(r.open_estimate ?? 0)
        acc.doneCount += Number(r.done_count_range ?? 0)
        acc.effortHours += Number(r.effort_hours ?? 0)
        acc.bugHours += Number(r.bug_hours ?? 0)
        return acc
      },
      { openTaskCount: 0, wipCount: 0, openEstimate: 0, doneCount: 0, effortHours: 0, bugHours: 0 },
    )
  }, [visibleRows])

  const bugPercent = safeRate(totals.bugHours, totals.effortHours)
  const avgOpenEstimate = safeRate(totals.openEstimate, visibleRows.length)
  const topLoadRow = useMemo(() => [...visibleRows].sort((a, b) => Number(b.open_estimate ?? 0) - Number(a.open_estimate ?? 0))[0], [visibleRows])
  const concentration = safeRate(Number(topLoadRow?.open_estimate ?? 0), totals.openEstimate)
  const loadRisk = totals.openTaskCount > 0 && (concentration > 0.35 || bugPercent > 0.3 || totals.wipCount > visibleRows.length * 3)

  const wipOption = useMemo(() => {
    const s = wip?.series ?? []
    return {
      tooltip: { trigger: 'axis' },
      grid: { left: 48, right: 16, top: 16, bottom: 32 },
      xAxis: { type: 'category', data: s.map((x: any) => x.day), axisLabel: { rotate: 45 } },
      yAxis: { type: 'value' },
      series: [{ type: 'line', smooth: true, data: s.map((x: any) => x.wip), name: 'WIP' }],
    }
  }, [wip])

  const throughputOption = useMemo(() => {
    const rows = (tp?.series ?? []).filter((r: any) => (account ? r.account === account : true))
    const days = Array.from(new Set(rows.map((r: any) => r.day))).sort()
    const accounts = Array.from(new Set(rows.map((r: any) => r.account))).sort()
    const series = accounts.map((acc) => ({
      name: acc,
      type: 'bar',
      stack: 'done',
      data: days.map((d) => {
        const hit = rows.find((x: any) => x.day === d && x.account === acc)
        return hit?.done ?? 0
      }),
    }))
    return {
      tooltip: { trigger: 'axis' },
      legend: { top: 0, textStyle: { color: 'var(--zm-text-secondary)' } },
      grid: { left: 48, right: 16, top: 32, bottom: 32 },
      xAxis: { type: 'category', data: days, axisLabel: { rotate: 45 } },
      yAxis: { type: 'value' },
      series,
    }
  }, [account, tp])

  return (
    <div>
      <div style={{ marginBottom: 16 }}>
        <Text style={{ color: 'var(--zm-text-primary)', fontSize: 18, fontWeight: 600 }}>员工看板</Text>
        <Space style={{ marginLeft: 12 }} wrap>
          <GroupSelect
            value={groupId}
            onChange={(id, name) => {
              setGroupId(id)
              setGroupName(name)
              setAccount(undefined)
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
          <Select
            allowClear
            showSearch
            optionFilterProp="label"
            placeholder="聚焦到个人（可选）"
            style={{ width: 260 }}
            value={account}
            options={memberOptions}
            onChange={(v) => setAccount(v as string | undefined)}
          />
        </Space>
      </div>

      <Row gutter={[16, 16]}>
        <Col span={24}>
          <Card title="人员管理摘要" loading={overviewLoading} style={CARD_STYLE}>
            {visibleRows.length > 0 ? (
              <>
                <Row gutter={[16, 12]} align="middle" style={{ marginBottom: 12 }}>
                  <Col xs={24} md={4}>
                    <Progress
                      type="dashboard"
                      percent={Math.max(0, Math.min(100, Math.round((1 - bugPercent) * 100)))}
                      size={108}
                      format={(v) => `${v}%`}
                    />
                  </Col>
                  <Col xs={12} md={3}>
                    <Statistic title="人数" value={visibleRows.length} />
                  </Col>
                  <Col xs={12} md={3}>
                    <Statistic title="未完成任务" value={totals.openTaskCount} />
                  </Col>
                  <Col xs={12} md={3}>
                    <Statistic title="WIP" value={totals.wipCount} />
                  </Col>
                  <Col xs={12} md={3}>
                    <Statistic title="周期完成" value={totals.doneCount} />
                  </Col>
                  <Col xs={12} md={3}>
                    <Statistic title="报工(h)" value={totals.effortHours.toFixed(1)} />
                  </Col>
                  <Col xs={12} md={4}>
                    <Statistic title="Bug工时占比" value={(bugPercent * 100).toFixed(1)} suffix="%" />
                  </Col>
                  <Col xs={12} md={4}>
                    <Statistic title="人均剩余预估(h)" value={avgOpenEstimate.toFixed(1)} />
                  </Col>
                </Row>
                {loadRisk ? (
                  <Alert
                    showIcon
                    type="warning"
                    style={{ marginBottom: 12 }}
                    message="人员侧存在管理风险信号：剩余工作集中、WIP偏高或Bug工时占比偏高，请结合下方明细做负载调整。"
                  />
                ) : (
                  <Alert showIcon type="success" style={{ marginBottom: 12 }} message="人员负载、完成吞吐和质量投入处于可观察状态。" />
                )}
                {topLoadRow ? (
                  <Text style={{ color: 'var(--zm-text-muted)', display: 'block', marginBottom: 12 }}>
                    最高剩余预估：{topLoadRow.realname?.trim() ? `${topLoadRow.realname}（${topLoadRow.account}）` : topLoadRow.account}
                    ，{Number(topLoadRow.open_estimate ?? 0).toFixed(1)}h，占当前筛选口径 {(concentration * 100).toFixed(1)}%。
                  </Text>
                ) : null}
              </>
            ) : (
              <Alert showIcon type="info" message="当前时间范围内暂无人员数据。" />
            )}

            <Table
              rowKey="account"
              size="small"
              pagination={{ pageSize: 10, showSizeChanger: true }}
              dataSource={visibleRows}
              scroll={{ x: 980 }}
              columns={[
                {
                  title: '人员',
                  dataIndex: 'account',
                  render: (_: any, r: any) => (
                    <Text style={{ color: 'var(--zm-text-primary)' }}>
                      {r.realname?.trim() ? `${r.realname}（${r.account}）` : r.account}
                    </Text>
                  ),
                },
                { title: '未完成数', dataIndex: 'open_task_count', width: 100, sorter: (a: any, b: any) => Number(a.open_task_count ?? 0) - Number(b.open_task_count ?? 0) },
                { title: 'WIP', dataIndex: 'wip_count', width: 80, sorter: (a: any, b: any) => Number(a.wip_count ?? 0) - Number(b.wip_count ?? 0) },
                { title: '未完成预估(h)', dataIndex: 'open_estimate', width: 140, sorter: (a: any, b: any) => Number(a.open_estimate ?? 0) - Number(b.open_estimate ?? 0) },
                { title: '周期完成数', dataIndex: 'done_count_range', width: 120, sorter: (a: any, b: any) => Number(a.done_count_range ?? 0) - Number(b.done_count_range ?? 0) },
                { title: '报工(h)', dataIndex: 'effort_hours', width: 100, sorter: (a: any, b: any) => Number(a.effort_hours ?? 0) - Number(b.effort_hours ?? 0) },
                { title: 'Bug占比%', dataIndex: 'bug_percent', width: 110, sorter: (a: any, b: any) => Number(a.bug_percent ?? 0) - Number(b.bug_percent ?? 0) },
                {
                  title: '',
                  width: 80,
                  render: (_: any, r: any) => (
                    <a
                      onClick={() =>
                        setTaskDrawer({
                          open: true,
                          title: `${r.account} 的任务（未完成）`,
                          params: { group_id: groupId, assigned_to: r.account, page: 1, page_size: 200 },
                        })
                      }
                      style={{ color: 'var(--zm-primary-text)' }}
                    >
                      下钻
                    </a>
                  ),
                },
              ]}
              style={{ background: 'transparent' }}
            />
          </Card>
        </Col>

        <Col span={12}>
          <Card title="团队WIP趋势" loading={wipLoading} style={CARD_STYLE}>
            <ReactECharts option={wipOption} style={{ height: 320 }} />
          </Card>
        </Col>
        <Col span={12}>
          <Card title={account ? '个人吞吐（完成任务数）' : '个人吞吐对比（完成任务数）'} loading={tpLoading} style={CARD_STYLE}>
            <ReactECharts option={throughputOption} style={{ height: 320 }} />
          </Card>
        </Col>

        <Col span={24}>
          <Card title="瓶颈候选：在制时间过长的任务" loading={bnLoading} style={CARD_STYLE}>
            {bn?.note ? <div style={{ marginBottom: 8, color: 'var(--zm-text-muted)', fontSize: 12 }}>{bn.note}</div> : null}
            <Table
              rowKey="id"
              size="small"
              dataSource={(bn?.items ?? []).filter((r: any) => (account ? r.assigned_to === account : true))}
              pagination={{ pageSize: 10, showSizeChanger: true }}
              scroll={{ x: 980 }}
              columns={[
                { title: 'ID', dataIndex: 'id', width: 90 },
                { title: '任务名', dataIndex: 'name' },
                { title: '指派给', dataIndex: 'assigned_to', width: 140 },
                { title: '状态', dataIndex: 'status', width: 100, render: (v: string) => <Tag>{v}</Tag> },
                { title: '在制时长(h)', dataIndex: 'age_hours', width: 120, sorter: (a: any, b: any) => Number(a.age_hours ?? 0) - Number(b.age_hours ?? 0) },
                {
                  title: '',
                  width: 80,
                  render: (_: any, r: any) => (
                    <a
                      onClick={() =>
                        setTaskDrawer({
                          open: true,
                          title: `任务 #${r.id}`,
                          params: { group_id: groupId, id: r.id, page: 1, page_size: 200 },
                        })
                      }
                      style={{ color: 'var(--zm-primary-text)' }}
                    >
                      查看
                    </a>
                  ),
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
            { title: '状态', dataIndex: 'status', width: 100, render: (v: string) => <Tag>{v}</Tag> },
            { title: '预估(h)', dataIndex: 'estimate', width: 90 },
            { title: '消耗(h)', dataIndex: 'consumed', width: 90 },
            { title: '迭代ID', dataIndex: 'execution_id', width: 90 },
            { title: '截止', dataIndex: 'deadline', width: 120, render: (v: string) => v || '-' },
          ]}
          style={{ background: 'transparent' }}
        />
      </Drawer>
    </div>
  )
}

export default PeopleDashboardPage
