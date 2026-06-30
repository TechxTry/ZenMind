import React, { useEffect, useState } from 'react'
import { Card, Form, Input, Button, Row, Col, message, Space, Typography,
  Divider, Badge, Statistic, Spin, InputNumber, Table, Tag, Alert } from 'antd'
import { CheckCircleOutlined, SyncOutlined } from '@ant-design/icons'
import { getDatasource, putDatasource, testDatasource, triggerSync, triggerEffortReconcile,
  getSyncStatus, getSyncActive, getSyncLogs, getLocalStats, getSyncSettings, putSyncSettings } from '../../api'
import dayjs from 'dayjs'

const { Title, Text } = Typography

interface SyncInfo {
  watermark: string
  last_count: number
  updated_at: string
}

interface SyncLogRow {
  id: number
  display_name: string
  status_label: string
  status: string
  message?: string
  actor_username?: string
  metadata?: { days?: number; upserted?: number }
  started_at: string
  finished_at?: string
  duration_ms?: number
}

const TABLE_LABELS: Record<string, string> = {
  local_users: '人员 (zt_user)',
  local_tasks: '任务 (zt_task)',
  local_stories: '需求 (zt_story)',
  local_bugs: '缺陷 (zt_bug)',
  local_efforts: '报工 (zt_effort)',
  local_executions: '迭代 (zt_project)',
}

const ConfigPage: React.FC = () => {
  const [form] = Form.useForm()
  const [testing, setTesting] = useState(false)
  const [saving, setSaving] = useState(false)
  const [syncing, setSyncing] = useState(false)
  const [syncStatus, setSyncStatus] = useState<Record<string, SyncInfo>>({})
  const [localCounts, setLocalCounts] = useState<Record<string, number>>({})
  const [localTotal, setLocalTotal] = useState(0)
  const [statusLoading, setStatusLoading] = useState(false)
  const [syncIntervalMinutes, setSyncIntervalMinutes] = useState(15)
  const [savingInterval, setSavingInterval] = useState(false)
  const [reconciling, setReconciling] = useState(false)
  const [effortReconcileEnabled, setEffortReconcileEnabled] = useState(true)
  const [effortReconcileHour, setEffortReconcileHour] = useState(3)
  const [effortReconcileDays, setEffortReconcileDays] = useState(180)
  const [passwordConfigured, setPasswordConfigured] = useState(false)
  const [syncLogs, setSyncLogs] = useState<SyncLogRow[]>([])
  const [syncLogsTotal, setSyncLogsTotal] = useState(0)
  const [syncLogsPage, setSyncLogsPage] = useState(1)
  const [activeRuns, setActiveRuns] = useState<{ kind: string; label: string; started_at: string }[]>([])

  useEffect(() => {
    getDatasource()
      .then((d: { password_configured?: boolean; host?: string; port?: string; user?: string; db_name?: string }) => {
        setPasswordConfigured(!!d.password_configured)
        form.setFieldsValue({
          host: d.host,
          port: d.port,
          user: d.user,
          db_name: d.db_name,
          password: '',
        })
      })
      .catch(() => {})
    fetchStatus()
  }, [])

  const buildDatasourcePayload = (values: {
    host?: string
    port?: string
    user?: string
    password?: string
    db_name?: string
  }) => {
    const payload: Record<string, string> = {
      host: values.host ?? '',
      port: values.port ?? '',
      user: values.user ?? '',
      db_name: values.db_name ?? '',
    }
    const pwd = (values.password ?? '').trim()
    if (pwd) {
      payload.password = pwd
    } else if (!passwordConfigured) {
      return null
    }
    return payload
  }

  const fetchStatus = () => {
    setStatusLoading(true)
    Promise.all([
      getSyncStatus().then((d: { tables: Record<string, SyncInfo> }) => d.tables ?? {}),
      getLocalStats().then((d: { tables: Record<string, number>; total: number }) => d),
      getSyncSettings().then((d: {
        interval_minutes: number
        effort_reconcile_enabled?: boolean
        effort_reconcile_hour?: number
        effort_reconcile_days?: number
      }) => d),
      getSyncLogs({ page: syncLogsPage, page_size: 15 }).then((d: { data: SyncLogRow[]; total: number }) => d),
      getSyncActive().catch(() => ({ running: [], busy: false })),
    ])
      .then(([tables, stats, sync, logs, active]) => {
        setSyncStatus(tables)
        setLocalCounts(stats.tables ?? {})
        setLocalTotal(stats.total ?? 0)
        if (typeof sync?.interval_minutes === 'number') {
          setSyncIntervalMinutes(sync.interval_minutes)
        }
        if (typeof sync?.effort_reconcile_enabled === 'boolean') {
          setEffortReconcileEnabled(sync.effort_reconcile_enabled)
        }
        if (typeof sync?.effort_reconcile_hour === 'number') {
          setEffortReconcileHour(sync.effort_reconcile_hour)
        }
        if (typeof sync?.effort_reconcile_days === 'number') {
          setEffortReconcileDays(sync.effort_reconcile_days)
        }
        setSyncLogs(logs?.data ?? [])
        setSyncLogsTotal(logs?.total ?? 0)
        setActiveRuns(active?.running ?? [])
      })
      .catch(() => {})
      .finally(() => setStatusLoading(false))
  }

  const handleTest = async () => {
    const values = form.getFieldsValue()
    const payload = buildDatasourcePayload(values)
    if (!payload) {
      message.warning('请填写 MySQL 密码')
      return
    }
    setTesting(true)
    try {
      const r = await testDatasource(payload)
      r.ok ? message.success('连接成功 ✓') : message.error('连接失败: ' + r.error)
    } catch {
      message.error('请求失败')
    } finally {
      setTesting(false)
    }
  }

  const handleSave = async () => {
    const values = form.getFieldsValue()
    const payload = buildDatasourcePayload(values)
    if (!payload) {
      message.warning('请填写 MySQL 密码')
      return
    }
    setSaving(true)
    try {
      await putDatasource(payload)
      setPasswordConfigured(true)
      form.setFieldsValue({ password: '' })
      message.success('数据源已保存并连接')
    } catch (e: any) {
      message.error(e.response?.data?.error ?? '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const handleSync = async () => {
    setSyncing(true)
    try {
      await triggerSync()
      message.success('增量同步已触发，请稍后刷新状态与日志')
      setTimeout(fetchStatus, 3000)
    } catch (e: any) {
      message.error(e.response?.data?.error ?? '触发失败')
    } finally {
      setSyncing(false)
    }
  }

  const handleEffortReconcile = async () => {
    setReconciling(true)
    try {
      await triggerEffortReconcile()
      message.success('报工回刷已触发，请稍后刷新状态与日志')
      setTimeout(fetchStatus, 5000)
    } catch (e: any) {
      const err = e.response?.data?.error
      const running = e.response?.data?.running
      if (e.response?.status === 409 && running) {
        message.warning('回刷报工正在执行中，请等其完成后再试')
      } else {
        message.error(err ?? '触发失败')
      }
    } finally {
      setReconciling(false)
    }
  }

  const handleSaveInterval = async () => {
    setSavingInterval(true)
    try {
      await putSyncSettings({ interval_minutes: syncIntervalMinutes })
      message.success('自动同步周期已保存')
    } catch (e: any) {
      message.error(e.response?.data?.error ?? '保存失败')
    } finally {
      setSavingInterval(false)
    }
  }

  const cardStyle = {
    background: 'var(--zm-bg-surface)',
    border: '1px solid var(--zm-border-subtle)',
    borderRadius: 12,
  }

  return (
    <div style={{ maxWidth: 1000 }}>
      <Title level={4} style={{ color: 'var(--zm-text-primary)', marginBottom: 24 }}>数据同步</Title>

      {/* Datasource Config */}
      <Card title={<Text style={{ color: 'var(--zm-text-primary)' }}>禅道 MySQL 数据源</Text>} style={cardStyle}
        styles={{ header: { borderBottom: '1px solid var(--zm-border-subtle)' } }}>
        <Form form={form} layout="vertical">
          <Row gutter={16}>
            <Col span={14}>
              <Form.Item name="host" label={<Text style={{ color: 'var(--zm-text-secondary)' }}>Host</Text>}
                rules={[{ required: true }]}>
                <Input placeholder="192.168.1.100" />
              </Form.Item>
            </Col>
            <Col span={10}>
              <Form.Item name="port" label={<Text style={{ color: 'var(--zm-text-secondary)' }}>Port</Text>}
                rules={[{ required: true }]} initialValue="3306">
                <Input placeholder="3306" />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item name="user" label={<Text style={{ color: 'var(--zm-text-secondary)' }}>用户名</Text>}
                rules={[{ required: true }]}>
                <Input placeholder="root" />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item
                name="password"
                label={<Text style={{ color: 'var(--zm-text-secondary)' }}>密码</Text>}
                extra={
                  passwordConfigured ? (
                    <Text style={{ color: 'var(--zm-text-muted)', fontSize: 11 }}>
                      已保存过密码，留空将沿用数据库中的配置（重启后仍有效）
                    </Text>
                  ) : undefined
                }
              >
                <Input.Password
                  placeholder={passwordConfigured ? '已配置（留空不修改）' : '••••••••'}
                  autoComplete="new-password"
                />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item name="db_name" label={<Text style={{ color: 'var(--zm-text-secondary)' }}>数据库名</Text>}
                rules={[{ required: true }]} initialValue="zentao">
                <Input placeholder="zentao" />
              </Form.Item>
            </Col>
          </Row>
          <Space>
            <Button onClick={handleTest} loading={testing} icon={<CheckCircleOutlined />}>
              测试连接
            </Button>
            <Button type="primary" onClick={handleSave} loading={saving}>
              保存 & 连接
            </Button>
          </Space>
        </Form>
      </Card>

      <Divider style={{ borderColor: 'var(--zm-border-subtle)' }} />

      {/* Local DB row counts */}
      <Card
        title={<Text style={{ color: 'var(--zm-text-primary)' }}>本地数据库数据量</Text>}
        style={cardStyle}
        styles={{ header: { borderBottom: '1px solid var(--zm-border-subtle)' } }}
        extra={
          <Text style={{ color: 'var(--zm-text-muted)', fontSize: 12 }}>
            PostgreSQL 已落库行数（与小组筛选无关）
          </Text>
        }
      >
        <Spin spinning={statusLoading}>
          <div style={{ marginBottom: 16 }}>
            <Text style={{ color: 'var(--zm-text-muted)', fontSize: 12 }}>合计 </Text>
            <Text style={{ color: 'var(--zm-primary-text)', fontSize: 18, fontWeight: 600 }}>{localTotal.toLocaleString()}</Text>
            <Text style={{ color: 'var(--zm-text-muted)', fontSize: 12 }}> 行</Text>
          </div>
          <Row gutter={[16, 16]}>
            {Object.entries(TABLE_LABELS).map(([key, label]) => {
              const n = localCounts[key]
              const has = typeof n === 'number'
              return (
                <Col span={8} key={`lc-${key}`}>
                  <div style={{
                    padding: 16, borderRadius: 10,
                    background: 'var(--zm-bg-surface-muted)',
                    border: '1px solid var(--zm-border-subtle)',
                  }}>
                    <Text style={{ color: 'var(--zm-text-secondary)', fontSize: 12, display: 'block', marginBottom: 8 }}>{label}</Text>
                    {has ? (
                      <Statistic
                        value={n}
                        suffix="行"
                        valueStyle={{ color: 'var(--zm-primary-text)', fontSize: 20 }}
                      />
                    ) : (
                      <Text style={{ color: 'var(--zm-text-disabled)', fontSize: 12 }}>—</Text>
                    )}
                  </div>
                </Col>
              )
            })}
          </Row>
        </Spin>
      </Card>

      <Divider style={{ borderColor: 'var(--zm-border-subtle)' }} />

      {activeRuns.length > 0 && (
        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
          message="当前有同步任务在执行"
          description={
            <Space direction="vertical" size={4}>
              {activeRuns.map((r) => (
                <Text key={r.kind} style={{ fontSize: 12 }}>
                  {r.label}：自 {dayjs(r.started_at).format('MM-DD HH:mm:ss')} 起
                  {r.kind === 'incremental' && (
                    <Text type="secondary">（可与「回刷报工」同时进行）</Text>
                  )}
                </Text>
              ))}
            </Space>
          }
        />
      )}

      {/* Sync Status */}
      <Card
        title={<Text style={{ color: 'var(--zm-text-primary)' }}>同步状态</Text>}
        style={cardStyle}
        styles={{ header: { borderBottom: '1px solid var(--zm-border-subtle)' } }}
        extra={
          <Space>
            <Button size="small" onClick={fetchStatus} icon={<SyncOutlined />}>刷新</Button>
            <Button type="primary" size="small" onClick={handleSync} loading={syncing}
              style={{ background: 'var(--zm-brand-gradient)', border: 'none' }}>
              立即同步
            </Button>
            <Button
              size="small"
              onClick={handleEffortReconcile}
              loading={reconciling}
              disabled={!effortReconcileEnabled}
              title="从禅道 MySQL 回刷近期报工，用于对齐在禅道里改过的工时"
            >
              回刷报工
            </Button>
          </Space>
        }
      >
        <Spin spinning={statusLoading}>
          <div style={{
            marginBottom: 20, display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 12,
          }}>
            <Text style={{ color: 'var(--zm-text-secondary)', fontSize: 13 }}>自动同步周期（分钟）</Text>
            <InputNumber
              min={1}
              max={1440}
              value={syncIntervalMinutes}
              onChange={(v) => { if (v != null) setSyncIntervalMinutes(v) }}
              style={{ width: 100 }}
            />
            <Button size="small" onClick={handleSaveInterval} loading={savingInterval}>
              保存周期
            </Button>
            <Text style={{ color: 'var(--zm-text-muted)', fontSize: 11 }}>
              范围 1～1440；保存后立即按新周期间隔重新计时（若此时正在跑 ETL，需等其结束后再进入等待）
            </Text>
          </div>
          <div style={{
            marginBottom: 20, display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 12,
          }}>
            <Text style={{ color: 'var(--zm-text-secondary)', fontSize: 13 }}>报工回刷</Text>
            <Text style={{ color: 'var(--zm-text-muted)', fontSize: 11 }}>
              {effortReconcileEnabled
                ? `每天 ${String(effortReconcileHour).padStart(2, '0')}:00 自动从禅道对齐近 ${effortReconcileDays} 天报工；手动「回刷报工」可与增量同步同时进行`
                : '已在环境变量中关闭（EFFORT_RECONCILE_ENABLED=false）'}
            </Text>
          </div>
          <Row gutter={[16, 16]}>
            {Object.entries(TABLE_LABELS).map(([key, label]) => {
              const info = syncStatus[key]
              const isSync = !!info
              return (
                <Col span={8} key={key}>
                  <div style={{
                    padding: 16, borderRadius: 10,
                    background: 'var(--zm-bg-surface-muted)',
                    border: '1px solid var(--zm-border-subtle)',
                  }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
                      <Text style={{ color: 'var(--zm-text-secondary)', fontSize: 12 }}>{label}</Text>
                      <Badge status={isSync ? 'success' : 'default'} text={
                        <Text style={{ color: 'var(--zm-text-muted)', fontSize: 11 }}>
                          {isSync ? '已同步' : '未同步'}
                        </Text>
                      } />
                    </div>
                    {info && (
                      <>
                        <Statistic
                          value={info.last_count}
                          suffix="条"
                          valueStyle={{ color: 'var(--zm-primary-text)', fontSize: 20 }}
                        />
                        <Text style={{ color: 'var(--zm-text-muted)', fontSize: 11, display: 'block', marginTop: 2 }}>
                          上轮增量（非库内总量）
                        </Text>
                        <Text style={{ color: 'var(--zm-text-muted)', fontSize: 11, display: 'block', marginTop: 4 }}>
                          {dayjs(info.updated_at).format('MM-DD HH:mm')}
                        </Text>
                      </>
                    )}
                    {!info && <Text style={{ color: 'var(--zm-text-disabled)', fontSize: 12 }}>暂无数据</Text>}
                  </div>
                </Col>
              )
            })}
          </Row>
        </Spin>
      </Card>

      <Divider style={{ borderColor: 'var(--zm-border-subtle)' }} />

      <Card
        title={<Text style={{ color: 'var(--zm-text-primary)' }}>同步日志</Text>}
        style={cardStyle}
        styles={{ header: { borderBottom: '1px solid var(--zm-border-subtle)' } }}
        extra={
          <Text style={{ color: 'var(--zm-text-muted)', fontSize: 12 }}>
            自动增量同步、手动同步、报工回刷的执行记录
          </Text>
        }
      >
        <Table<SyncLogRow>
          size="small"
          rowKey="id"
          loading={statusLoading}
          dataSource={syncLogs}
          pagination={{
            current: syncLogsPage,
            pageSize: 15,
            total: syncLogsTotal,
            showSizeChanger: false,
            onChange: (p) => {
              setSyncLogsPage(p)
              setStatusLoading(true)
              getSyncLogs({ page: p, page_size: 15 })
                .then((d: { data: SyncLogRow[]; total: number }) => {
                  setSyncLogs(d?.data ?? [])
                  setSyncLogsTotal(d?.total ?? 0)
                })
                .catch(() => {})
                .finally(() => setStatusLoading(false))
            },
          }}
          columns={[
            {
              title: '任务',
              dataIndex: 'display_name',
              width: 140,
            },
            {
              title: '状态',
              dataIndex: 'status_label',
              width: 88,
              render: (label: string, row) => {
                const color =
                  row.status === 'success' ? 'success'
                    : row.status === 'running' ? 'processing'
                      : row.status === 'skipped' ? 'warning'
                        : row.status === 'failed' ? 'error'
                          : 'default'
                return <Tag color={color}>{label}</Tag>
              },
            },
            {
              title: '开始时间',
              dataIndex: 'started_at',
              width: 130,
              render: (v: string) => dayjs(v).format('YYYY-MM-DD HH:mm:ss'),
            },
            {
              title: '耗时',
              dataIndex: 'duration_ms',
              width: 72,
              render: (ms: number | undefined, row: SyncLogRow) => {
                if (row.status === 'running') return '—'
                if (ms == null) return '—'
                if (ms < 1000) return `${ms} ms`
                return `${(ms / 1000).toFixed(1)} s`
              },
            },
            {
              title: '操作人',
              dataIndex: 'actor_username',
              width: 100,
              render: (v?: string) => v || '—',
            },
            {
              title: '说明',
              dataIndex: 'message',
              ellipsis: true,
              render: (msg: string | undefined, row) => {
                const parts: string[] = []
                if (msg) parts.push(msg)
                if (row.metadata?.upserted != null) {
                  parts.push(`写入 ${row.metadata.upserted} 条`)
                }
                if (row.metadata?.days != null) {
                  parts.push(`窗口 ${row.metadata.days} 天`)
                }
                return parts.join(' · ') || '—'
              },
            },
          ]}
        />
      </Card>
    </div>
  )
}

export default ConfigPage
