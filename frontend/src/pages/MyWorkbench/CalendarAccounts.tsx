import React, { useCallback, useEffect, useState } from 'react'
import {
  Alert,
  Button,
  Card,
  Form,
  Input,
  List,
  Modal,
  Popconfirm,
  Radio,
  Space,
  Tag,
  Typography,
  message,
} from 'antd'
import {
  createCalendarAccount,
  createCalendarFeed,
  deleteCalendarAccount,
  deleteCalendarFeed,
  listCalendarAccounts,
  listCalendarFeeds,
  testCalendarAccount,
} from '../../api'
import type { CalendarAccount, CalendarAccountType, CalendarFeed } from '../../api'

const { Text } = Typography

const typeLabel = (t: string) => {
  if (t === 'exchange') return 'Exchange'
  if (t === 'caldav') return 'CalDAV'
  return t
}

const CalendarAccountsPage: React.FC = () => {
  const [accounts, setAccounts] = useState<CalendarAccount[]>([])
  const [accountsLoading, setAccountsLoading] = useState(false)
  const [accountOpen, setAccountOpen] = useState(false)
  const [accountSubmitting, setAccountSubmitting] = useState(false)
  const [accountTesting, setAccountTesting] = useState(false)
  const [feeds, setFeeds] = useState<CalendarFeed[]>([])
  const [feedsLoading, setFeedsLoading] = useState(false)
  const [feedOpen, setFeedOpen] = useState(false)
  const [feedSubmitting, setFeedSubmitting] = useState(false)
  const [form] = Form.useForm()
  const [feedForm] = Form.useForm()

  const selectedType = Form.useWatch('type', form) as CalendarAccountType | undefined

  const refreshAccounts = useCallback(async () => {
    setAccountsLoading(true)
    try {
      const r = await listCalendarAccounts()
      setAccounts(r?.data ?? [])
    } catch (e: any) {
      message.error(e.response?.data?.error ?? '加载失败')
      setAccounts([])
    } finally {
      setAccountsLoading(false)
    }
  }, [])

  const refreshFeeds = useCallback(async () => {
    setFeedsLoading(true)
    try {
      const r = await listCalendarFeeds()
      setFeeds(r?.data ?? [])
    } catch (e: any) {
      message.error(e.response?.data?.error ?? '加载失败')
      setFeeds([])
    } finally {
      setFeedsLoading(false)
    }
  }, [])

  useEffect(() => {
    void refreshAccounts()
    void refreshFeeds()
  }, [refreshAccounts, refreshFeeds])

  useEffect(() => {
    if (!accountOpen) return
    form.resetFields()
    form.setFieldsValue({ type: 'exchange', description: '' })
  }, [accountOpen, form])

  useEffect(() => {
    if (!feedOpen) return
    feedForm.resetFields()
    feedForm.setFieldsValue({ color: '#6366F1' })
  }, [feedOpen, feedForm])

  const onSubmit = async () => {
    const v = await form.validateFields()
    setAccountSubmitting(true)
    try {
      await createCalendarAccount({
        type: v.type,
        server: v.server,
        username: v.username,
        password: v.password,
        description: v.description,
      })
      message.success('已添加日历账户')
      setAccountOpen(false)
      form.resetFields()
      void refreshAccounts()
    } catch (e: any) {
      message.error(e.response?.data?.error ?? '添加失败')
    } finally {
      setAccountSubmitting(false)
    }
  }

  const onTestAccount = async () => {
    const v = await form.validateFields()
    setAccountTesting(true)
    try {
      const r = await testCalendarAccount({
        type: v.type,
        server: v.server,
        username: v.username,
        password: v.password,
        description: v.description,
      })
      message.success(`连接成功，未来 30 天读取到 ${r.events ?? 0} 条日程`)
    } catch (e: any) {
      message.error(e.response?.data?.error ?? '连接失败')
    } finally {
      setAccountTesting(false)
    }
  }

  const onSubmitFeed = async () => {
    const v = await feedForm.validateFields()
    setFeedSubmitting(true)
    try {
      await createCalendarFeed({
        name: String(v.name ?? '').trim(),
        ical_url: String(v.ical_url ?? '').trim(),
        color: String(v.color ?? '#6366F1'),
      })
      message.success('已添加订阅')
      setFeedOpen(false)
      feedForm.resetFields()
      void refreshFeeds()
    } catch (e: any) {
      message.error(e.response?.data?.error ?? '添加失败')
    } finally {
      setFeedSubmitting(false)
    }
  }

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}>
        <div>
          <Text style={{ color: 'var(--zm-text-primary)', fontSize: 18, fontWeight: 600 }}>日历账户</Text>
          <div style={{ marginTop: 6 }}>
            <Text style={{ color: 'var(--zm-text-muted)', fontSize: 12 }}>
              在这里统一管理日历账户与 ICS 订阅；敏感信息会加密保存，仅用于后续拉取日程。
            </Text>
          </div>
        </div>
      </div>

      <div style={{ height: 12 }} />

      <Card
        title={<Text style={{ color: 'var(--zm-text-primary)' }}>账户列表</Text>}
        extra={(
          <Space>
            <Button onClick={() => void refreshAccounts()} loading={accountsLoading}>
              刷新
            </Button>
            <Button type="primary" onClick={() => setAccountOpen(true)}>
              添加账户
            </Button>
          </Space>
        )}
        styles={{ header: { borderBottom: '1px solid var(--zm-border-subtle)' } }}
        style={{ background: 'var(--zm-bg-surface)', border: '1px solid var(--zm-border-subtle)', borderRadius: 12 }}
      >
        <List
          loading={accountsLoading}
          dataSource={accounts}
          locale={{ emptyText: '暂无日历账户' }}
          renderItem={(it) => (
            <List.Item
              actions={[
                <Button
                  key="del"
                  danger
                  type="link"
                  onClick={() => {
                    Modal.confirm({
                      title: '删除此账户？',
                      content: `将删除 ${typeLabel(it.type)} · ${it.username}`,
                      okText: '删除',
                      cancelText: '取消',
                      okButtonProps: { danger: true },
                      onOk: async () => {
                        try {
                          await deleteCalendarAccount(it.id)
                          message.success('已删除')
                          void refreshAccounts()
                        } catch (e: any) {
                          message.error(e.response?.data?.error ?? '删除失败')
                        }
                      },
                    })
                  }}
                >
                  删除
                </Button>,
              ]}
            >
              <List.Item.Meta
                title={
                  <Space wrap>
                    <Tag color={it.type === 'exchange' ? 'geekblue' : 'purple'}>{typeLabel(it.type)}</Tag>
                    <Text strong>{it.username}</Text>
                    {it.description ? <Text type="secondary">· {it.description}</Text> : null}
                  </Space>
                }
                description={
                  <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap', color: 'var(--zm-text-muted)', fontSize: 12 }}>
                    <span>服务器：{it.server || '—'}</span>
                    <span>创建：{it.created_at ? new Date(it.created_at).toLocaleString() : '—'}</span>
                  </div>
                }
              />
            </List.Item>
          )}
        />
      </Card>

      <div style={{ height: 12 }} />

      <Card
        title={<Text style={{ color: 'var(--zm-text-primary)' }}>日历订阅</Text>}
        extra={(
          <Space>
            <Button onClick={() => void refreshFeeds()} loading={feedsLoading}>
              刷新
            </Button>
            <Button type="primary" onClick={() => setFeedOpen(true)}>
              添加订阅
            </Button>
          </Space>
        )}
        styles={{ header: { borderBottom: '1px solid var(--zm-border-subtle)' } }}
        style={{ background: 'var(--zm-bg-surface)', border: '1px solid var(--zm-border-subtle)', borderRadius: 12 }}
      >
        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 12 }}
          message="通过 iCal / ICS 地址订阅"
          description="在 Google 日历、Outlook、Apple 日历等中复制 ICAL 订阅链接，粘贴到这里即可。订阅地址会加密保存在服务端，仅您本人可拉取。"
        />
        <List
          loading={feedsLoading}
          dataSource={feeds}
          locale={{ emptyText: '尚未添加外部日历' }}
          renderItem={(f) => (
            <List.Item
              actions={[
                <Popconfirm
                  key="del"
                  title="删除此订阅？"
                  okText="删除"
                  cancelText="取消"
                  onConfirm={async () => {
                    try {
                      await deleteCalendarFeed(f.id)
                      message.success('已删除')
                      void refreshFeeds()
                    } catch (e: any) {
                      message.error(e.response?.data?.error ?? '删除失败')
                    }
                  }}
                >
                  <Button size="small" type="link" danger>
                    删除
                  </Button>
                </Popconfirm>,
              ]}
            >
              <List.Item.Meta
                title={
                  <Space align="center">
                    <span
                      style={{
                        width: 10,
                        height: 10,
                        background: f.color,
                        borderRadius: 2,
                        display: 'inline-block',
                        flexShrink: 0,
                      }}
                    />
                    <span>{f.name}</span>
                  </Space>
                }
                description={f.feed_host || '—'}
              />
            </List.Item>
          )}
        />
      </Card>

      <Modal
        title="添加日历账户"
        open={accountOpen}
        onCancel={() => setAccountOpen(false)}
        onOk={onSubmit}
        okText="确定"
        cancelText="取消"
        confirmLoading={accountSubmitting}
        width={640}
      >
        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 12 }}
          message="支持 Exchange / CalDAV"
          description="账户信息会加密保存；保存前可先测试连接，确认服务器地址与密码可用。"
        />
        <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: 12 }}>
          <Button onClick={onTestAccount} loading={accountTesting}>
            测试连接
          </Button>
        </div>
        <Form form={form} layout="vertical" initialValues={{ type: 'exchange' }}>
          <Form.Item name="username" label="邮箱" rules={[{ required: true, message: '请输入邮箱' }]}>
            <Input placeholder="例如：gaojy@digiwin.com" maxLength={200} />
          </Form.Item>

          <Form.Item name="type" label="类型" rules={[{ required: true, message: '请选择类型' }]}>
            <Radio.Group>
              <Radio value="exchange">Exchange</Radio>
              <Radio value="caldav">CalDAV</Radio>
            </Radio.Group>
          </Form.Item>

          {selectedType === 'caldav' ? (
            <Form.Item name="server" label="服务器" rules={[{ required: true, message: '请输入服务器' }]}>
              <Input placeholder="cal.example.com 或 https://cal.example.com/dav" maxLength={1024} />
            </Form.Item>
          ) : selectedType === 'exchange' ? (
            <Form.Item name="server" label="EWS 地址">
              <Input placeholder="可选：mail.example.com 或 https://mail.example.com/EWS/Exchange.asmx" maxLength={1024} />
            </Form.Item>
          ) : null}

          <Form.Item name="password" label="密码" rules={[{ required: true, message: '请输入密码' }]}>
            <Input.Password placeholder="必填" maxLength={512} />
          </Form.Item>

          {selectedType === 'caldav' ? (
            <Form.Item name="description" label="描述">
              <Input placeholder="我的 CalDAV 账户" maxLength={200} />
            </Form.Item>
          ) : (
            <Form.Item name="description" label="描述">
              <Input placeholder="可选" maxLength={200} />
            </Form.Item>
          )}
        </Form>
      </Modal>

      <Modal
        title="添加日历订阅"
        open={feedOpen}
        onCancel={() => setFeedOpen(false)}
        onOk={onSubmitFeed}
        okText="确定"
        cancelText="取消"
        confirmLoading={feedSubmitting}
        width={680}
      >
        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 12 }}
          message="通过 iCal / ICS 地址订阅"
          description="在 Google 日历、Outlook、Apple 日历等中复制 ICAL 订阅链接，粘贴到下方即可。"
        />
        <Form form={feedForm} layout="vertical" initialValues={{ color: '#6366F1' }}>
          <Form.Item name="name" label="显示名称" rules={[{ required: true, message: '请输入名称' }]}>
            <Input placeholder="例如：团队值班表" maxLength={120} />
          </Form.Item>
          <Form.Item name="ical_url" label="ICS 订阅地址" rules={[{ required: true, message: '请输入地址' }]}>
            <Input.TextArea rows={3} placeholder="https://calendar.google.com/calendar/ical/..." />
          </Form.Item>
          <Form.Item name="color" label="标记色">
            <Input type="color" style={{ width: 120, height: 36, padding: 2, borderRadius: 6 }} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default CalendarAccountsPage
