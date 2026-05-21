import React, { useEffect, useState } from 'react'
import { Card, Form, Input, Button, Row, Col, message, Space, Typography,
  Divider, Badge, Statistic, Spin, InputNumber } from 'antd'
import { CheckCircleOutlined, SyncOutlined } from '@ant-design/icons'
import { getDatasource, putDatasource, testDatasource, triggerSync, triggerEffortReconcile,
  getSyncStatus, getLocalStats, getSyncSettings, putSyncSettings } from '../../api'
import dayjs from 'dayjs'

const { Title, Text } = Typography

interface SyncInfo {
  watermark: string
  last_count: number
  updated_at: string
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

  useEffect(() => {
    getDatasource().then((d: any) => form.setFieldsValue(d)).catch(() => {})
    fetchStatus()
  }, [])

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
    ])
      .then(([tables, stats, sync]) => {
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
      })
      .catch(() => {})
      .finally(() => setStatusLoading(false))
  }

  const handleTest = async () => {
    const values = form.getFieldsValue()
    setTesting(true)
    try {
      const r = await testDatasource({
        host: values.host, port: values.port,
        user: values.user, password: values.password, db_name: values.db_name,
      })
      r.ok ? message.success('连接成功 ✓') : message.error('连接失败: ' + r.error)
    } catch {
      message.error('请求失败')
    } finally {
      setTesting(false)
    }
  }

  const handleSave = async () => {
    const values = form.getFieldsValue()
    setSaving(true)
    try {
      await putDatasource({
        host: values.host, port: values.port,
        user: values.user, password: values.password, db_name: values.db_name,
      })
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
      message.success('同步任务已触发，请稍后刷新状态')
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
      message.success('报工日对账已触发，请稍后刷新状态')
      setTimeout(fetchStatus, 5000)
    } catch (e: any) {
      const err = e.response?.data?.error
      const running = e.response?.data?.running
      if (e.response?.status === 409 && running) {
        message.warning(`当前有同步任务进行中（${running}），请稍后再试`)
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
              <Form.Item name="password" label={<Text style={{ color: 'var(--zm-text-secondary)' }}>密码</Text>}>
                <Input.Password placeholder="••••••••" />
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
            >
              报工日对账
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
            <Text style={{ color: 'var(--zm-text-secondary)', fontSize: 13 }}>报工日对账</Text>
            <Text style={{ color: 'var(--zm-text-muted)', fontSize: 11 }}>
              {effortReconcileEnabled
                ? `每天 ${String(effortReconcileHour).padStart(2, '0')}:00 自动执行；手动触发将回刷近 ${effortReconcileDays} 天报工（与增量同步互斥）`
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
    </div>
  )
}

export default ConfigPage
