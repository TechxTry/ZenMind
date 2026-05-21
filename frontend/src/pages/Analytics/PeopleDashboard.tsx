import React, { useEffect, useMemo, useState } from 'react'
import { Alert, Card, Col, DatePicker, Drawer, Row, Select, Space, Statistic, Table, Tag, Typography, message } from 'antd'
import dayjs, { Dayjs } from 'dayjs'
import ReactECharts from 'echarts-for-react'
import { getPeopleOverview, getPeopleWipTrend, getPeopleThroughput, getPeopleBottleneck, listTasks } from '../../api'

const { RangePicker } = DatePicker
const { Text } = Typography

const PeopleDashboardPage: React.FC = () => {
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
    let cancelled = false
    const params = { date_from: dateFrom, date_to: dateTo }

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
  }, [dateFrom, dateTo])

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
    const rows = tp?.series ?? []
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
  }, [tp])

  const topRow = (overview?.rows ?? [])[0]

  return (
    <div>
      <div style={{ marginBottom: 16 }}>
        <Text style={{ color: 'var(--zm-text-primary)', fontSize: 18, fontWeight: 600 }}>员工看板</Text>
        <Space style={{ marginLeft: 12 }} wrap>
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
          <Card
            title="人员概览"
            loading={overviewLoading}
            style={{ background: 'var(--zm-bg-surface)', border: '1px solid var(--zm-border-subtle)', borderRadius: 12 }}
          >
            {topRow ? (
              <Row gutter={12} style={{ marginBottom: 12 }}>
                <Col>
                  <Statistic title="统计周期" value={`${dateFrom} ~ ${dateTo}`} />
                </Col>
                <Col>
                  <Statistic title="人数" value={(overview?.rows ?? []).length} />
                </Col>
              </Row>
            ) : (
              <Alert showIcon type="info" message="当前时间范围内暂无人员数据。" />
            )}

            <Table
              rowKey="account"
              size="small"
              pagination={false}
              dataSource={(overview?.rows ?? []).filter((r: any) => (account ? r.account === account : true))}
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
                { title: '未完成数', dataIndex: 'open_task_count', width: 90 },
                { title: 'WIP', dataIndex: 'wip_count', width: 70 },
                { title: '未完成预估(h)', dataIndex: 'open_estimate', width: 120 },
                { title: '周期完成数', dataIndex: 'done_count_range', width: 100 },
                { title: '报工(h)', dataIndex: 'effort_hours', width: 90 },
                { title: 'Bug占比%', dataIndex: 'bug_percent', width: 90 },
                {
                  title: '',
                  width: 80,
                  render: (_: any, r: any) => (
                    <a
                      onClick={() =>
                        setTaskDrawer({
                          open: true,
                          title: `${r.account} 的任务（未完成）`,
                          params: { assigned_to: r.account, page: 1, page_size: 200 },
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
          <Card title="团队WIP趋势（近似）" loading={wipLoading} style={{ background: 'var(--zm-bg-surface)', border: '1px solid var(--zm-border-subtle)', borderRadius: 12 }}>
            <ReactECharts option={wipOption} style={{ height: 320 }} />
          </Card>
        </Col>
        <Col span={12}>
          <Card title="个人吞吐（完成任务数）" loading={tpLoading} style={{ background: 'var(--zm-bg-surface)', border: '1px solid var(--zm-border-subtle)', borderRadius: 12 }}>
            <ReactECharts option={throughputOption} style={{ height: 320 }} />
          </Card>
        </Col>

        <Col span={24}>
          <Card title="瓶颈候选：在制时间过长的任务" loading={bnLoading} style={{ background: 'var(--zm-bg-surface)', border: '1px solid var(--zm-border-subtle)', borderRadius: 12 }}>
            {bn?.note ? <div style={{ marginBottom: 8, color: 'var(--zm-text-muted)', fontSize: 12 }}>{bn.note}</div> : null}
            <Table
              rowKey="id"
              size="small"
              dataSource={(bn?.items ?? []).filter((r: any) => (account ? r.assigned_to === account : true))}
              pagination={false}
              columns={[
                { title: 'ID', dataIndex: 'id', width: 90 },
                { title: '任务名', dataIndex: 'name' },
                { title: '指派给', dataIndex: 'assigned_to', width: 140 },
                { title: '状态', dataIndex: 'status', width: 100, render: (v: string) => <Tag>{v}</Tag> },
                { title: '在制时长(h)', dataIndex: 'age_hours', width: 110 },
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
            { title: '迭代ID', dataIndex: 'execution_id', width: 90 },
          ]}
          style={{ background: 'transparent' }}
        />
      </Drawer>
    </div>
  )
}

export default PeopleDashboardPage

