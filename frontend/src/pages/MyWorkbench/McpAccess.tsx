import { useEffect, useMemo, useState } from 'react'
import { Button, Card, Input, InputNumber, Modal, Space, Table, Tag, Typography, message } from 'antd'
import { createMyMCPToken, deleteMyMCPToken, listMyMCPTokens, type MCPAccessToken } from '../../api'

const { Paragraph, Text } = Typography

async function copyToClipboard(txt: string): Promise<boolean> {
  if (!txt) return false
  try {
    await navigator.clipboard.writeText(txt)
    return true
  } catch {
    try {
      const ta = document.createElement('textarea')
      ta.value = txt
      ta.style.position = 'fixed'
      ta.style.left = '-9999px'
      document.body.appendChild(ta)
      ta.focus()
      ta.select()
      const ok = document.execCommand('copy')
      document.body.removeChild(ta)
      return ok
    } catch {
      return false
    }
  }
}

const McpAccessPage: React.FC = () => {
  const [rows, setRows] = useState<MCPAccessToken[]>([])
  const [loading, setLoading] = useState(false)
  const [createOpen, setCreateOpen] = useState(false)
  const [tokenOpen, setTokenOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [name, setName] = useState('我的 Cursor')
  const [expireDays, setExpireDays] = useState<number>(90)
  const [newToken, setNewToken] = useState<string>('')

  const load = async () => {
    setLoading(true)
    try {
      const r = await listMyMCPTokens()
      setRows(r.data || [])
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  const mcpConfig = useMemo(() => {
    if (!newToken) return ''
    const mcpUrl = `${window.location.origin}/api/mcp`
    return JSON.stringify(
      {
        mcpServers: {
          zenmind: {
            url: mcpUrl,
            headers: {
              Authorization: `Bearer ${newToken}`,
            },
          },
        },
      },
      null,
      2,
    )
  }, [newToken])

  const onCreate = async () => {
    setCreating(true)
    try {
      const r = await createMyMCPToken({ name, expire_days: expireDays })
      setNewToken(r.token)
      setCreateOpen(false)
      setTokenOpen(true)
      message.success('MCP 密钥创建成功，请立即复制保存')
      await load()
    } catch (e: any) {
      message.error(e?.response?.data?.error || '创建失败')
      throw e
    } finally {
      setCreating(false)
    }
  }

  const onDelete = async (id: number) => {
    await deleteMyMCPToken(id)
    message.success('已撤销')
    await load()
  }

  const copyText = async (txt: string, okMsg: string) => {
    const ok = await copyToClipboard(txt)
    if (ok) {
      message.success(okMsg)
    } else {
      message.error('复制失败，请手动选中复制')
    }
  }

  const openCreateModal = () => {
    setName('我的 Cursor')
    setExpireDays(90)
    setCreateOpen(true)
  }

  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <Card title="MCP 访问密钥" extra={<Button type="primary" onClick={openCreateModal}>生成新密钥</Button>}>
        <Paragraph type="secondary">
          普通用户可在这里创建专用于 MCP 的个人访问密钥（无需 DevTools）。密钥只会在创建成功时展示一次。
        </Paragraph>
        <Paragraph type="secondary" style={{ marginBottom: 0 }}>
          在 Cursor 中：设置 → Features → Model Context Protocol → Open JSON，将下方「MCP 配置」整段粘贴到
          <Text code>mcpServers</Text> 中（或写入项目 <Text code>.cursor/mcp.json</Text>）。
          修改配置后请重启 Cursor 或重新加载 MCP。
        </Paragraph>
        <Table<MCPAccessToken>
          rowKey="id"
          loading={loading}
          dataSource={rows}
          pagination={false}
          columns={[
            { title: '名称', dataIndex: 'token_name' },
            { title: '前缀', dataIndex: 'token_prefix', render: (v) => <Text code>{v}...</Text> },
            { title: '过期时间', dataIndex: 'expires_at', render: (v) => v ? new Date(v).toLocaleString() : '永不过期' },
            { title: '最近使用', dataIndex: 'last_used_at', render: (v) => v ? new Date(v).toLocaleString() : <Tag>未使用</Tag> },
            {
              title: '操作',
              key: 'op',
              render: (_, row) => (
                <Button danger size="small" onClick={() => onDelete(row.id)}>
                  撤销
                </Button>
              ),
            },
          ]}
        />
      </Card>

      <Modal
        title="创建 MCP 密钥"
        open={createOpen}
        onCancel={() => setCreateOpen(false)}
        onOk={() => onCreate()}
        okText="创建"
        confirmLoading={creating}
        destroyOnClose
      >
        <Space direction="vertical" style={{ width: '100%' }}>
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="密钥名称" />
          <InputNumber min={1} max={365} value={expireDays} onChange={(v) => setExpireDays(Number(v || 90))} addonAfter="天" />
        </Space>
      </Modal>

      <Modal
        title="新密钥（仅展示一次）"
        open={tokenOpen}
        onCancel={() => setTokenOpen(false)}
        footer={
          <Button type="primary" onClick={() => setTokenOpen(false)}>
            我已保存
          </Button>
        }
        width={640}
        destroyOnClose={false}
      >
        <Space direction="vertical" style={{ width: '100%' }}>
          <Input.TextArea value={newToken} autoSize={{ minRows: 2, maxRows: 4 }} readOnly />
          <Space>
            <Button type="primary" onClick={() => void copyText(newToken, '已复制密钥')}>
              复制密钥
            </Button>
            <Button onClick={() => void copyText(mcpConfig, '已复制 MCP 配置')}>
              复制MCP配置
            </Button>
          </Space>
          <Input.TextArea value={mcpConfig} autoSize={{ minRows: 6, maxRows: 12 }} readOnly />
        </Space>
      </Modal>

      {newToken && !tokenOpen ? (
        <Card title="新密钥（仅展示一次）">
          <Space direction="vertical" style={{ width: '100%' }}>
            <Input.TextArea value={newToken} autoSize={{ minRows: 2, maxRows: 4 }} readOnly />
            <Space>
              <Button type="primary" onClick={() => void copyText(newToken, '已复制密钥')}>
                复制密钥
              </Button>
              <Button onClick={() => void copyText(mcpConfig, '已复制 MCP 配置')}>
                复制MCP配置
              </Button>
            </Space>
            <Input.TextArea value={mcpConfig} autoSize={{ minRows: 6, maxRows: 12 }} readOnly />
          </Space>
        </Card>
      ) : null}
    </Space>
  )
}

export default McpAccessPage
