import React, { useEffect, useMemo, useState } from 'react'
import { Alert, Card, Col, DatePicker, Drawer, InputNumber, Row, Select, Space, Statistic, Switch, Table, Tag, Typography, message } from 'antd'
import dayjs, { Dayjs } from 'dayjs'
import ReactECharts from 'echarts-for-react'
import { getEffortHeatmap, getUserLoad, getWorkloadDistribution, listEfforts, listTasks } from '../../api'
import { GroupSelect } from '../../components/GroupSelect'
import { useAuthStore } from '../../store/auth'

const { RangePicker } = DatePicker
const { Text } = Typography

type Member = { account: string; realname: string }

function memberLabel(m: Member) {
  const name = m.realname?.trim()
  return name ? `${name}（${m.account}）` : m.account
}

const DEFAULT_THRESHOLDS = {
  targetHours: 8,
  overloadHours: 12,
  overloadStreak: 3,
  bugPercentAlert: 0.3,
}

const TeamHealthPage: React.FC = () => {
  const [groupId, setGroupId] = useState<number | undefined>()
  const [groupName, setGroupName] = useState('')
  const me = useAuthStore((s) => s.me)
  const dataScope = (me?.user?.data_scope ?? '').toUpperCase()
  const defaultGroupId = me?.user?.default_group_id ?? undefined

  const [range, setRange] = useState<[Dayjs, Dayjs]>(() => [dayjs().add(-29, 'day'), dayjs()])
  const [excludeWeekend, setExcludeWeekend] = useState(true)
  const [account, setAccount] = useState<string | undefined>()

  const [thresholds, setThresholds] = useState(DEFAULT_THRESHOLDS)

  const [heatmapLoading, setHeatmapLoading] = useState(false)
  const [heatmap, setHeatmap] = useState<any | null>(null)

  const [userLoadLoading, setUserLoadLoading] = useState(false)
  const [userLoadRows, setUserLoadRows] = useState<any[]>([])

  const [distLoading, setDistLoading] = useState(false)
  const [dist, setDist] = useState<any | null>(null)

  // ---- Drilldown drawers ----
  const [effortDrawer, setEffortDrawer] = useState<{
    open: boolean
    title: string
    params: any
  }>({ open: false, title: '', params: null })
  const [effortRows, setEffortRows] = useState<any[]>([])
  const [effortLoading, setEffortLoading] = useState(false)

  const [taskDrawer, setTaskDrawer] = useState<{
    open: boolean
    title: string
    account?: string
    status?: string
  }>({ open: false, title: '' })
  const [taskRows, setTaskRows] = useState<any[]>([])
  const [taskLoading, setTaskLoading] = useState(false)

  const start = useMemo(() => range[0].format('YYYY-MM-DD'), [range])
  const end = useMemo(() => range[1].format('YYYY-MM-DD'), [range])

  const members: Member[] = useMemo(() => heatmap?.members ?? [], [heatmap])
  const memberOptions = useMemo(
    () => members.map((m) => ({ value: m.account, label: memberLabel(m) })),
    [members],
  )

  useEffect(() => {
    if (dataScope === 'GROUP' && defaultGroupId && !groupId) {
      setGroupId(defaultGroupId)
    }
  }, [dataScope, defaultGroupId, groupId])

  useEffect(() => {
    if (!groupId) {
      setHeatmap(null)
      setUserLoadRows([])
      setDist(null)
      return
    }

    let cancelled = false

    setHeatmapLoading(true)
    getEffortHeatmap({
      group_id: groupId,
      start,
      end,
      exclude_weekend: excludeWeekend,
      target_hours: thresholds.targetHours,
      overload_hours: thresholds.overloadHours,
      overload_streak: thresholds.overloadStreak,
    })
      .then((d) => {
        if (!cancelled) setHeatmap(d)
      })
      .catch((e) => {
        if (!cancelled) message.error(e.response?.data?.error ?? '热力图数据获取失败')
      })
      .finally(() => {
        if (!cancelled) setHeatmapLoading(false)
      })

    setUserLoadLoading(true)
    getUserLoad({ group_id: groupId })
      .then((d) => {
        if (!cancelled) setUserLoadRows(d.rows ?? [])
      })
      .catch((e) => {
        if (!cancelled) message.error(e.response?.data?.error ?? '人员负载数据获取失败')
      })
      .finally(() => {
        if (!cancelled) setUserLoadLoading(false)
      })

    setDistLoading(true)
    getWorkloadDistribution({ group_id: groupId, start, end, account })
      .then((d) => {
        if (!cancelled) setDist(d)
      })
      .catch((e) => {
        if (!cancelled) message.error(e.response?.data?.error ?? '精力投入数据获取失败')
      })
      .finally(() => {
        if (!cancelled) setDistLoading(false)
      })

    return () => {
      cancelled = true
    }
  }, [groupId, start, end, excludeWeekend, thresholds, account])

  const heatmapOption = useMemo(() => {
    const dates: string[] = heatmap?.dates ?? []
    const ms: Member[] = heatmap?.members ?? []
    const matrix: Record<string, Record<string, { consumed_sum: number; entries_count: number }>> = heatmap?.matrix ?? {}

    const yLabels = ms.map((m) => memberLabel(m))
    const points: any[] = []
    for (let yi = 0; yi < ms.length; yi++) {
      const acc = ms[yi].account
      for (let xi = 0; xi < dates.length; xi++) {
        const d = dates[xi]
        const cell = matrix?.[acc]?.[d]
        const v = cell?.consumed_sum ?? 0
        points.push([xi, yi, v, cell?.entries_count ?? 0, acc, d])
      }
    }

    return {
      grid: { left: 140, right: 24, top: 16, bottom: 80 },
      tooltip: {
        trigger: 'item',
        formatter: (p: any) => {
          const v = p?.value
          if (!v) return ''
          const xi = v[0]
          const yi = v[1]
          const hours = v[2]
          const cnt = v[3]
          return `${yLabels[yi]}<br/>${dates[xi]}<br/>工时：${hours}h<br/>条目：${cnt}`
        },
      },
      xAxis: {
        type: 'category',
        data: dates,
        axisLabel: { rotate: 45, color: 'var(--zm-text-secondary)' },
        axisLine: { lineStyle: { color: 'var(--zm-border-subtle)' } },
        splitLine: { show: false },
      },
      yAxis: {
        type: 'category',
        data: yLabels,
        axisLabel: { color: 'var(--zm-text-secondary)' },
        axisLine: { lineStyle: { color: 'var(--zm-border-subtle)' } },
        splitLine: { show: false },
      },
      visualMap: {
        min: 0,
        max: Math.max(12, thresholds.overloadHours),
        calculable: true,
        orient: 'horizontal',
        left: 'center',
        bottom: 10,
        textStyle: { color: 'var(--zm-text-secondary)' },
      },
      series: [
        {
          type: 'heatmap',
          data: points,
          encode: { x: 0, y: 1, value: 2 },
          emphasis: { itemStyle: { borderColor: 'var(--zm-text-primary)', borderWidth: 1 } },
        },
      ],
    }
  }, [heatmap, thresholds.overloadHours])

  const openEffortDrawerForDay = async (acc: string, day: string, titlePrefix: string) => {
    if (!groupId) return
    setEffortDrawer({
      open: true,
      title: `${titlePrefix} · ${day}`,
      params: { group_id: groupId, account: acc, date_from: day, date_to: day, page: 1, page_size: 200 },
    })
  }

  useEffect(() => {
    if (!effortDrawer.open || !effortDrawer.params) return
    setEffortLoading(true)
    listEfforts(effortDrawer.params)
      .then((res) => setEffortRows(res.data ?? []))
      .catch((e) => message.error(e.response?.data?.error ?? '报工明细获取失败'))
      .finally(() => setEffortLoading(false))
  }, [effortDrawer])

  const openEffortDrawerByType = async (ot: string, title: string) => {
    if (!groupId) return
    setEffortDrawer({
      open: true,
      title,
      params: { group_id: groupId, account, date_from: start, date_to: end, object_type: ot, page: 1, page_size: 200 },
    })
  }

  const openTaskDrawer = async (acc: string, status?: string) => {
    setTaskDrawer({
      open: true,
      title: `${acc} 的任务（${status ?? '全部状态'}）`,
      account: acc,
      status,
    })
  }

  useEffect(() => {
    if (!taskDrawer.open || !taskDrawer.account || !groupId) return
    setTaskLoading(true)
    listTasks({
      group_id: groupId,
      assigned_to: taskDrawer.account,
      status: taskDrawer.status,
      page: 1,
      page_size: 200,
    })
      .then((res) => setTaskRows(res.data ?? []))
      .catch((e) => message.error(e.response?.data?.error ?? '任务明细获取失败'))
      .finally(() => setTaskLoading(false))
  }, [taskDrawer, groupId])

  const pieOption = useMemo(() => {
    const items = dist?.items ?? []
    return {
      tooltip: {
        trigger: 'item',
        formatter: (p: any) => `${p.name}<br/>${p.value}h（${(p.percent ?? 0).toFixed(1)}%）`,
      },
      legend: { top: 0, textStyle: { color: 'var(--zm-text-secondary)' } },
      series: [
        {
          type: 'pie',
          radius: ['35%', '70%'],
          top: 20,
          data: items.map((it: any) => ({
            name: `${it.category} / ${it.object_type}`,
            value: Number(it.consumed_sum ?? 0),
            _object_type: it.object_type,
          })),
          label: { color: 'var(--zm-text-secondary)' },
        },
      ],
    }
  }, [dist])

  const summary = heatmap?.summary
  const bugPercent = dist?.alerts?.bug_percent ?? 0
  const bugAlert = bugPercent > thresholds.bugPercentAlert
  const overloadStreakUsers = useMemo(() => {
    const rows = summary?.top_overload_users ?? []
    return rows.filter((u: any) => (u.max_streak_days ?? 0) >= thresholds.overloadStreak)
  }, [summary, thresholds.overloadStreak])
  const complianceRate = summary?.compliance_rate ?? 0
  const complianceAlert = complianceRate > 0 && complianceRate < 0.85

  return (
    <div>
      <div style={{ marginBottom: 16 }}>
        <Text style={{ color: 'var(--zm-text-primary)', fontSize: 18, fontWeight: 600 }}>团队负荷与工时健康度</Text>
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
          {groupId ? <Tag color="purple">{groupName}</Tag> : <Tag>请选择小组</Tag>}
        </Space>
      </div>

      <Card
        style={{
          marginBottom: 16,
          background: 'var(--zm-bg-surface)',
          border: '1px solid var(--zm-border-subtle)',
          borderRadius: 12,
        }}
      >
        <Space wrap>
          <RangePicker
            value={range}
            onChange={(v) => {
              if (!v || !v[0] || !v[1]) return
              setRange([v[0], v[1]])
            }}
            disabledDate={(current) => Math.abs(current.diff(range[0], 'day')) > 180}
          />
          <Switch checked={excludeWeekend} onChange={setExcludeWeekend} />
          <Text style={{ color: 'var(--zm-text-secondary)' }}>过滤周末</Text>
          <Select
            allowClear
            showSearch
            optionFilterProp="label"
            placeholder="聚合到个人（可选）"
            style={{ width: 260 }}
            disabled={!groupId}
            value={account}
            options={memberOptions}
            onChange={(v) => setAccount(v as string | undefined)}
          />
          <Space size={6}>
            <Text style={{ color: 'var(--zm-text-muted)' }}>达标</Text>
            <InputNumber
              min={0}
              step={0.5}
              value={thresholds.targetHours}
              onChange={(v) => setThresholds((s) => ({ ...s, targetHours: Number(v ?? 8) }))}
              style={{ width: 90 }}
            />
            <Text style={{ color: 'var(--zm-text-muted)' }}>过载</Text>
            <InputNumber
              min={0}
              step={0.5}
              value={thresholds.overloadHours}
              onChange={(v) => setThresholds((s) => ({ ...s, overloadHours: Number(v ?? 12) }))}
              style={{ width: 90 }}
            />
            <Text style={{ color: 'var(--zm-text-muted)' }}>Bug阈值%</Text>
            <InputNumber
              min={0}
              max={100}
              step={1}
              value={Math.round(thresholds.bugPercentAlert * 100)}
              onChange={(v) => setThresholds((s) => ({ ...s, bugPercentAlert: Number(v ?? 30) / 100 }))}
              style={{ width: 100 }}
            />
          </Space>
        </Space>
      </Card>

      <Row gutter={[16, 16]}>
        <Col span={24}>
          <Card
            title="工时填报热力图"
            loading={heatmapLoading}
            style={{ background: 'var(--zm-bg-surface)', border: '1px solid var(--zm-border-subtle)', borderRadius: 12 }}
            extra={
              <Space>
                <Text style={{ color: 'var(--zm-text-muted)' }}>达标≥{thresholds.targetHours}h</Text>
                <Text style={{ color: 'var(--zm-text-muted)' }}>过载{'>'}{thresholds.overloadHours}h</Text>
              </Space>
            }
          >
            {groupId && (
              <>
                <Row gutter={12} style={{ marginBottom: 12 }}>
                  <Col>
                    <Statistic title="合规率(>0h)" value={((summary?.compliance_rate ?? 0) * 100).toFixed(1)} suffix="%" />
                  </Col>
                  <Col>
                    <Statistic title={`达标率(≥${thresholds.targetHours}h)`} value={((summary?.target_rate ?? 0) * 100).toFixed(1)} suffix="%" />
                  </Col>
                  <Col>
                    <Statistic title={`过载占比(>${thresholds.overloadHours}h)`} value={((summary?.overload_rate ?? 0) * 100).toFixed(1)} suffix="%" />
                  </Col>
                </Row>
                {complianceAlert && (
                  <Alert
                    type="info"
                    showIcon
                    style={{ marginBottom: 12 }}
                    message={`报工合规率 ${(complianceRate * 100).toFixed(1)}% 偏低，建议关注填报执行情况（未报工不等于未工作，但会影响数据决策质量）。`}
                  />
                )}
                {overloadStreakUsers.length > 0 && (
                  <Alert
                    type="warning"
                    showIcon
                    style={{ marginBottom: 12 }}
                    message={`存在连续过载（≥${thresholds.overloadStreak}天，>${thresholds.overloadHours}h）的成员：${overloadStreakUsers
                      .slice(0, 5)
                      .map((u: any) => (u.realname?.trim() ? `${u.realname}（${u.account}）` : u.account))
                      .join('、')}${overloadStreakUsers.length > 5 ? '…' : ''}`}
                  />
                )}

                <ReactECharts
                  option={heatmapOption}
                  style={{ height: Math.max(320, (members.length || 1) * 22 + 160) }}
                  onEvents={{
                    click: (p: any) => {
                      const v = p?.value
                      if (!v) return
                      const acc = v[4]
                      const day = v[5]
                      const m = members.find((x) => x.account === acc)
                      void openEffortDrawerForDay(acc, day, memberLabel(m ?? { account: acc, realname: '' }))
                    },
                  }}
                />
              </>
            )}
          </Card>
        </Col>

        <Col span={12}>
          <Card
            title="人员资源负载分布"
            loading={userLoadLoading}
            style={{ background: 'var(--zm-bg-surface)', border: '1px solid var(--zm-border-subtle)', borderRadius: 12 }}
          >
            <Table
              size="small"
              rowKey="account"
              dataSource={[...userLoadRows].sort((a, b) => (b.estimate_sum_open ?? 0) - (a.estimate_sum_open ?? 0))}
              pagination={false}
              columns={[
                {
                  title: '人员',
                  dataIndex: 'account',
                  render: (_: any, r: any) => (
                    <Text style={{ color: 'var(--zm-text-primary)' }}>{r.realname?.trim() ? `${r.realname}（${r.account}）` : r.account}</Text>
                  ),
                },
                { title: '未完成数', dataIndex: 'open_task_count', width: 90 },
                { title: '预估(h)', dataIndex: 'estimate_sum_open', width: 90, render: (v: any) => Number(v ?? 0).toFixed(1) },
                {
                  title: '',
                  width: 80,
                  render: (_: any, r: any) => (
                    <a
                      onClick={() => void openTaskDrawer(r.account)}
                      style={{ color: 'var(--zm-primary-text)' }}
                    >查看</a>
                  ),
                },
              ]}
              style={{ background: 'transparent' }}
            />
            <div style={{ marginTop: 10, color: 'var(--zm-text-muted)', fontSize: 12 }}>
              说明：当前按任务状态聚合（wait/doing/active/pause）；点击“查看”可进一步按状态筛选钻取任务列表。
            </div>
          </Card>
        </Col>

        <Col span={12}>
          <Card
            title="精力投入分布（按报工关联类型）"
            loading={distLoading}
            style={{ background: 'var(--zm-bg-surface)', border: '1px solid var(--zm-border-subtle)', borderRadius: 12 }}
            extra={<Text style={{ color: 'var(--zm-text-muted)' }}>Bug占比阈值：{Math.round(thresholds.bugPercentAlert * 100)}%</Text>}
          >
            {bugAlert && (
              <Alert
                type="warning"
                showIcon
                style={{ marginBottom: 12 }}
                message={`修Bug工时占比 ${(bugPercent * 100).toFixed(1)}% 超过阈值，建议关注质量与返工成本。`}
              />
            )}
            <ReactECharts
              option={pieOption}
              style={{ height: 360 }}
              onEvents={{
                click: (p: any) => {
                  const ot = p?.data?._object_type
                  if (!ot) return
                  void openEffortDrawerByType(ot, `报工明细 · object_type=${ot}`)
                },
              }}
            />
          </Card>
        </Col>
      </Row>

      <Drawer
        title={<Text style={{ color: 'var(--zm-text-primary)' }}>{effortDrawer.title}</Text>}
        open={effortDrawer.open}
        width={860}
        onClose={() => setEffortDrawer({ open: false, title: '', params: null })}
        styles={{ body: { background: 'var(--zm-bg-canvas)' }, header: { background: 'var(--zm-bg-canvas)' } }}
      >
        <Table
          rowKey="id"
          size="small"
          loading={effortLoading}
          dataSource={effortRows}
          pagination={false}
          columns={[
            { title: '日期', dataIndex: 'work_date', width: 110, render: (v: string) => v ? dayjs(v).format('YYYY-MM-DD') : '-' },
            { title: '消耗(h)', dataIndex: 'consumed', width: 90 },
            { title: '类型', dataIndex: 'object_type', width: 90 },
            { title: '关联ID', dataIndex: 'object_id', width: 90 },
            { title: '工作内容', dataIndex: 'work', render: (v: string) => <Text style={{ color: 'var(--zm-text-secondary)' }}>{v}</Text> },
          ]}
          style={{ background: 'transparent' }}
        />
      </Drawer>

      <Drawer
        title={<Text style={{ color: 'var(--zm-text-primary)' }}>{taskDrawer.title}</Text>}
        open={taskDrawer.open}
        width={960}
        onClose={() => setTaskDrawer({ open: false, title: '' })}
        styles={{ body: { background: 'var(--zm-bg-canvas)' }, header: { background: 'var(--zm-bg-canvas)' } }}
        extra={
          <Space>
            <Select
              value={taskDrawer.status}
              placeholder="按状态过滤"
              allowClear
              style={{ width: 180 }}
              options={[
                { value: 'wait', label: '未开始(wait)' },
                { value: 'doing', label: '进行中(doing)' },
                { value: 'active', label: '激活(active)' },
                { value: 'pause', label: '暂停(pause)' },
                { value: 'done', label: '已完成(done)' },
                { value: 'closed', label: '已关闭(closed)' },
              ]}
              onChange={(v) => {
                setTaskDrawer((s) => ({ ...s, status: v as string | undefined, title: `${s.account} 的任务（${v ?? '全部状态'}）` }))
              }}
            />
          </Space>
        }
      >
        <Table
          rowKey="id"
          size="small"
          loading={taskLoading}
          dataSource={taskRows}
          pagination={false}
          columns={[
            { title: 'ID', dataIndex: 'id', width: 80 },
            { title: '任务名', dataIndex: 'name', render: (v: string) => <Text style={{ color: 'var(--zm-text-primary)' }}>{v}</Text> },
            { title: '状态', dataIndex: 'status', width: 110, render: (v: string) => <Tag>{v}</Tag> },
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

export default TeamHealthPage

