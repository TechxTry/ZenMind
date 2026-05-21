import React, { useEffect, useState } from 'react'
import { Card, DatePicker, Input, Space, Table, Tag, Typography, message } from 'antd'
import dayjs, { Dayjs } from 'dayjs'
import JsonView from '@uiw/react-json-view'
import { adminListAuditLogs } from '../../api'

const { Text } = Typography
const { RangePicker } = DatePicker

export default function AuditLogsPage() {
  const [loading, setLoading] = useState(false)
  const [rows, setRows] = useState<any[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [action, setAction] = useState<string | undefined>()
  const [actor, setActor] = useState<string | undefined>()
  const [range, setRange] = useState<[Dayjs, Dayjs]>(() => [dayjs().add(-6, 'day'), dayjs()])

  const fetch = async (p = page) => {
    setLoading(true)
    try {
      const res = await adminListAuditLogs({
        action: action?.trim() ? action.trim() : undefined,
        actor: actor?.trim() ? actor.trim() : undefined,
        from: range?.[0]?.format('YYYY-MM-DD'),
        to: range?.[1]?.format('YYYY-MM-DD'),
        page: p,
        page_size: 20,
      })
      setRows(res.data ?? [])
      setTotal(res.total ?? 0)
      setPage(res.page ?? p)
    } catch (e: any) {
      message.error(e.response?.data?.error ?? '加载失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void fetch(1)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return (
    <div style={{ maxWidth: 1200 }}>
      <Card
        title={<Text style={{ color: 'var(--zm-text-primary)' }}>审计日志</Text>}
        style={{ background: 'var(--zm-bg-surface)', border: '1px solid var(--zm-border-subtle)', borderRadius: 12 }}
        extra={
          <Space wrap>
            <RangePicker value={range} onChange={(v) => v && v[0] && v[1] && setRange([v[0], v[1]])} />
            <Input
              allowClear
              placeholder="action 精确匹配"
              style={{ width: 220 }}
              value={action}
              onChange={(e) => setAction(e.target.value)}
              onPressEnter={() => void fetch(1)}
            />
            <Input
              allowClear
              placeholder="操作者（模糊）"
              style={{ width: 220 }}
              value={actor}
              onChange={(e) => setActor(e.target.value)}
              onPressEnter={() => void fetch(1)}
            />
            <a onClick={() => void fetch(1)} style={{ color: 'var(--zm-primary-text)' }}>
              查询
            </a>
          </Space>
        }
      >
        <Table
          rowKey="id"
          size="small"
          loading={loading}
          dataSource={rows}
          pagination={{
            current: page,
            total,
            pageSize: 20,
            showSizeChanger: false,
            onChange: (p) => void fetch(p),
          }}
          columns={[
            { title: '时间', dataIndex: 'created_at', width: 180, render: (v: string) => (v ? dayjs(v).format('YYYY-MM-DD HH:mm:ss') : '-') },
            { title: '操作者', dataIndex: 'actor_username', width: 140 },
            { title: '动作', dataIndex: 'action', width: 220, render: (v: string) => <Tag>{v}</Tag> },
            { title: '目标类型', dataIndex: 'target_type', width: 120 },
            { title: '目标ID', dataIndex: 'target_id', width: 140 },
            { title: 'IP', dataIndex: 'ip', width: 130 },
            {
              title: 'metadata',
              dataIndex: 'metadata',
              render: (v: any) =>
                v ? <JsonView value={v} collapsed={2} style={{ background: 'transparent', fontSize: 12, fontFamily: 'monospace' }} /> : <Text type="secondary">-</Text>,
            },
          ]}
          style={{ background: 'transparent' }}
        />
      </Card>
    </div>
  )
}

