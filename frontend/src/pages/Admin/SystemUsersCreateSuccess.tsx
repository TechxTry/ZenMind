import React, { useEffect, useMemo, useState } from 'react'
import { Button, Card, Space, Table, Typography, message } from 'antd'
import { CopyOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { AdminBatchCreateSystemUserItem } from '../../api'

const BATCH_CREATE_RESULT_SESSION_KEY = 'zenmind_admin_system_users_batch_create_result'

const { Text } = Typography

export default function SystemUsersCreateSuccessPage() {
  const navigate = useNavigate()
  const [rows, setRows] = useState<AdminBatchCreateSystemUserItem[]>([])

  useEffect(() => {
    const raw = sessionStorage.getItem(BATCH_CREATE_RESULT_SESSION_KEY)
    if (!raw) {
      message.warning('未找到本次批量创建结果')
      navigate('/admin/system-users', { replace: true })
      return
    }
    try {
      const parsed = JSON.parse(raw) as AdminBatchCreateSystemUserItem[]
      setRows(Array.isArray(parsed) ? parsed : [])
    } catch {
      setRows([])
    }
  }, [navigate])

  const columns = useMemo(
    () => [
      { title: '账号', dataIndex: 'username', width: 180, render: (v: string) => <Text strong>{v}</Text> },
      { title: '显示名', dataIndex: 'display_name', width: 220 },
      {
        title: '初始密码（已生成）',
        dataIndex: 'password',
        width: 320,
        render: (v: string) => (
          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <pre
              style={{
                margin: 0,
                padding: '8px 10px',
                background: 'rgba(255,255,255,0.04)',
                borderRadius: 8,
                fontFamily: 'monospace',
                whiteSpace: 'pre-wrap',
              }}
            >
              {v}
            </pre>
          </div>
        ),
      },
      {
        title: '',
        dataIndex: 'password',
        width: 80,
        render: (_: any, r: AdminBatchCreateSystemUserItem) => (
          <Button
            icon={<CopyOutlined />}
            size="small"
            onClick={async () => {
              try {
                await navigator.clipboard.writeText(r.password)
                message.success('已复制')
              } catch {
                message.error('复制失败，请手动复制')
              }
            }}
          >
            复制
          </Button>
        ),
      },
    ],
    [],
  )

  const cleanupAndBack = () => {
    sessionStorage.removeItem(BATCH_CREATE_RESULT_SESSION_KEY)
    navigate('/admin/system-users')
  }

  return (
    <div style={{ maxWidth: 1200 }}>
      <Card
        title={<Text style={{ color: 'var(--zm-text-primary)' }}>批量创建成功</Text>}
        style={{ background: 'var(--zm-bg-surface)', border: '1px solid var(--zm-border-subtle)', borderRadius: 12 }}
        extra={
          <Space>
            <Button
              onClick={async () => {
                const text = rows.map((r) => `${r.username}\t${r.password}`).join('\n')
                try {
                  await navigator.clipboard.writeText(text)
                  message.success('已复制全部账号/密码')
                } catch {
                  message.error('复制失败，请手动复制')
                }
              }}
              disabled={rows.length === 0}
            >
              复制全部
            </Button>
            <Button type="primary" onClick={cleanupAndBack}>
              返回账号管理
            </Button>
          </Space>
        }
      >
        <div style={{ marginBottom: 12, color: 'var(--zm-text-muted)', fontSize: 12 }}>
          密码仅展示在此页面。请尽快复制并妥善保存。
        </div>
        <Table rowKey="username" size="small" columns={columns as any} dataSource={rows} pagination={false} />
      </Card>
    </div>
  )
}

