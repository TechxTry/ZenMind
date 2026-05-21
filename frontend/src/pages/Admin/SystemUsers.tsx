import React, { useEffect, useMemo, useState } from 'react'
import { Button, Card, Form, Input, Modal, Select, Space, Switch, Table, Tag, Typography, message, Tabs, Transfer } from 'antd'
import {
  adminCreateSystemUser,
  adminBatchCreateSystemUsers,
  adminDeleteZentaoBinding,
  adminGetZentaoBinding,
  adminListSystemUsers,
  adminResetSystemUserPassword,
  adminSetZentaoBinding,
  adminUpdateSystemUser,
  listGroups,
  listUsers,
} from '../../api'
import { useNavigate } from 'react-router-dom'

const { Text } = Typography

type GroupOption = { id: number; name: string }

const ROLE_OPTIONS = [
  { value: 'user', label: 'user' },
  { value: 'admin', label: 'admin' },
  { value: 'super_admin', label: 'super_admin' },
]
const SCOPE_OPTIONS = [
  { value: 'SELF', label: 'SELF' },
  { value: 'GROUP', label: 'GROUP' },
  { value: 'ALL', label: 'ALL' },
]

const BATCH_CREATE_RESULT_SESSION_KEY = 'zenmind_admin_system_users_batch_create_result'

export default function SystemUsersPage() {
  const navigate = useNavigate()

  const [q, setQ] = useState('')
  const [loading, setLoading] = useState(false)
  const [rows, setRows] = useState<any[]>([])
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)

  const [groups, setGroups] = useState<GroupOption[]>([])
  const groupNameOf = useMemo(() => new Map(groups.map((g) => [g.id, g.name])), [groups])

  const [createOpen, setCreateOpen] = useState(false)
  const [createForm] = Form.useForm()
  const [batchForm] = Form.useForm()
  const [createMode, setCreateMode] = useState<'single' | 'batch'>('batch')
  const [batchPersonnel, setBatchPersonnel] = useState<Array<{ key: string; title: string }>>([])
  const [batchTargetKeys, setBatchTargetKeys] = useState<string[]>([])

  const [editOpen, setEditOpen] = useState(false)
  const [editForm] = Form.useForm()
  const [editing, setEditing] = useState<any | null>(null)

  const [bindOpen, setBindOpen] = useState(false)
  const [bindingUser, setBindingUser] = useState<any | null>(null)
  const [zentaoOptions, setZentaoOptions] = useState<{ value: string; label: string }[]>([])
  const [zentaoAccount, setZentaoAccount] = useState<string | undefined>()

  const fetchAllLocalUsers = async () => {
    const pageSize = 5000
    let p = 1
    let total = 0
    const all: any[] = []
    do {
      const res = await listUsers({ q: '', page: p, page_size: pageSize })
      const rows = res.data ?? []
      total = Number(res.total ?? 0)
      all.push(...rows)
      p += 1
      if (rows.length === 0) break
    } while (all.length < total)
    return all
  }

  const fetchGroups = async () => {
    try {
      const d = await listGroups()
      setGroups(d.data ?? [])
    } catch {
      setGroups([])
    }
  }

  const fetch = async (p = page) => {
    setLoading(true)
    try {
      const res = await adminListSystemUsers({ q, page: p, page_size: 20 })
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
    void fetchGroups()
    void fetch(1)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const fetchBatchPersonnel = async () => {
    try {
      const us = await fetchAllLocalUsers()
      setBatchPersonnel(
        us.map((x: any) => ({
          key: x.account,
          title: x.realname?.trim() ? `${x.realname}（${x.account}）` : x.account,
        })),
      )
    } catch {
      setBatchPersonnel([])
    }
  }

  useEffect(() => {
    if (!createOpen || createMode !== 'batch' || batchPersonnel.length > 0) return
    void fetchBatchPersonnel()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [createOpen, createMode, batchPersonnel.length])

  const openCreate = () => {
    setCreateMode('batch')
    setBatchTargetKeys([])
    setCreateOpen(true)
    createForm.resetFields()
    batchForm.resetFields()
  }

  const openEdit = (u: any) => {
    setEditing(u)
    editForm.setFieldsValue({
      display_name: u.display_name ?? '',
      role: u.role ?? 'user',
      data_scope: u.data_scope ?? 'SELF',
      default_group_id: u.default_group_id ?? undefined,
      disabled: !!u.disabled,
    })
    setEditOpen(true)
  }

  const openBind = async (u: any) => {
    setBindingUser(u)
    setZentaoAccount(undefined)
    setBindOpen(true)
    try {
      const r = await adminGetZentaoBinding(u.id)
      if (r.bound && r.binding?.zentao_account) setZentaoAccount(r.binding.zentao_account)
    } catch {
      // ignore
    }
    try {
      const us = await fetchAllLocalUsers()
      setZentaoOptions(
        us.map((x: any) => ({
          value: x.account,
          label: x.realname?.trim() ? `${x.realname}（${x.account}）` : x.account,
        })),
      )
    } catch {
      setZentaoOptions([])
    }
  }

  const columns = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    { title: '用户名', dataIndex: 'username', width: 160, render: (v: string) => <Text strong>{v}</Text> },
    { title: '显示名', dataIndex: 'display_name', width: 160 },
    {
      title: '角色',
      dataIndex: 'role',
      width: 120,
      render: (v: string) => <Tag color={v === 'super_admin' ? 'gold' : v === 'admin' ? 'blue' : 'default'}>{v}</Tag>,
    },
    { title: '范围', dataIndex: 'data_scope', width: 90, render: (v: string) => <Tag>{v}</Tag> },
    {
      title: '默认组',
      dataIndex: 'default_group_id',
      width: 180,
      render: (v: any) => {
        const id = Number(v)
        if (!id) return <Text type="secondary">-</Text>
        return <Text>{groupNameOf.get(id) ?? `#${id}`}</Text>
      },
    },
    { title: '禁用', dataIndex: 'disabled', width: 80, render: (v: boolean) => (v ? <Tag color="red">是</Tag> : <Tag color="green">否</Tag>) },
    {
      title: '操作',
      key: 'actions',
      width: 280,
      render: (_: any, r: any) => (
        <Space>
          <Button size="small" onClick={() => openEdit(r)}>
            编辑
          </Button>
          <Button size="small" onClick={() => openBind(r)}>
            绑定禅道
          </Button>
          <Button
            size="small"
            onClick={async () => {
              try {
                const d = await adminResetSystemUserPassword(r.id)
                Modal.info({
                  title: '密码已重置',
                  content: (
                    <div>
                      <div>新密码（仅展示一次）：</div>
                      <pre style={{ padding: 12, background: 'rgba(255,255,255,0.04)', borderRadius: 8 }}>{d.new_password}</pre>
                    </div>
                  ),
                })
              } catch (e: any) {
                message.error(e.response?.data?.error ?? '重置失败')
              }
            }}
          >
            重置密码
          </Button>
        </Space>
      ),
    },
  ]

  return (
    <div style={{ maxWidth: 1200 }}>
      <Card
        title={<Text style={{ color: 'var(--zm-text-primary)' }}>系统用户管理</Text>}
        style={{ background: 'var(--zm-bg-surface)', border: '1px solid var(--zm-border-subtle)', borderRadius: 12 }}
        extra={
          <Space>
            <Input.Search
              allowClear
              placeholder="搜索用户名/显示名"
              style={{ width: 260 }}
              onSearch={() => void fetch(1)}
              value={q}
              onChange={(e) => setQ(e.target.value)}
            />
            <Button type="primary" onClick={openCreate} style={{ background: 'var(--zm-brand-gradient)', border: 'none' }}>
              新建用户
            </Button>
          </Space>
        }
      >
        <Table
          rowKey="id"
          size="small"
          loading={loading}
          dataSource={rows}
          columns={columns as any}
          pagination={{
            current: page,
            total,
            pageSize: 20,
            showSizeChanger: false,
            onChange: (p) => void fetch(p),
          }}
          style={{ background: 'transparent' }}
        />
      </Card>

      <Modal
        open={createOpen}
        onCancel={() => {
          setCreateOpen(false)
          setCreateMode('batch')
          setBatchTargetKeys([])
          createForm.resetFields()
          batchForm.resetFields()
        }}
        title="新建系统用户"
        okText="创建"
        onOk={async () => {
          if (createMode === 'single') {
            const v = await createForm.validateFields()
            try {
              await adminCreateSystemUser(v)
              message.success('创建成功')
              setCreateOpen(false)
              createForm.resetFields()
              void fetch(1)
            } catch (e: any) {
              message.error(e.response?.data?.error ?? '创建失败')
            }
            return
          }

          if (batchTargetKeys.length === 0) {
            message.error('请先从禅道人员列表选择要创建的用户')
            return
          }

          const v = await batchForm.validateFields()
          if (v.data_scope === 'GROUP' && !v.default_group_id) {
            message.error('GROUP 用户必须选择默认小组')
            return
          }

          try {
            const r = await adminBatchCreateSystemUsers({
              accounts: batchTargetKeys,
              role: v.role,
              data_scope: v.data_scope,
              default_group_id: v.default_group_id ?? null,
            })

            sessionStorage.setItem(BATCH_CREATE_RESULT_SESSION_KEY, JSON.stringify(r.created ?? []))
            message.success('创建成功')
            setCreateOpen(false)
            setBatchTargetKeys([])
            void fetch(1)
            navigate('/admin/system-users/create-success')
          } catch (e: any) {
            message.error(e.response?.data?.error ?? '批量创建失败')
          }
        }}
      >
        <Tabs
          activeKey={createMode}
          onChange={(k) => setCreateMode(k as any)}
          items={[
            {
              key: 'single',
              label: '单个创建',
              children: (
                <Form form={createForm} layout="vertical" initialValues={{ role: 'user', data_scope: 'SELF' }}>
                  <Form.Item name="username" label="用户名" rules={[{ required: true }]}>
                    <Input placeholder="例如: a.li" />
                  </Form.Item>
                  <Form.Item name="display_name" label="显示名">
                    <Input placeholder="例如: 李A" />
                  </Form.Item>
                  <Form.Item name="password" label="初始密码" rules={[{ required: true }]}>
                    <Input.Password />
                  </Form.Item>
                  <Space style={{ width: '100%' }} align="start">
                    <Form.Item name="role" label="角色" style={{ flex: 1 }}>
                      <Select options={ROLE_OPTIONS} />
                    </Form.Item>
                    <Form.Item name="data_scope" label="数据范围" style={{ flex: 1 }}>
                      <Select options={SCOPE_OPTIONS} />
                    </Form.Item>
                  </Space>
                  <Form.Item name="default_group_id" label="默认小组（GROUP 用户必填）">
                    <Select
                      allowClear
                      showSearch
                      optionFilterProp="label"
                      options={groups.map((g) => ({ value: g.id, label: g.name }))}
                    />
                  </Form.Item>
                </Form>
              ),
            },
            {
              key: 'batch',
              label: '禅道人员批量创建',
              children: (
                <>
                  <div style={{ marginBottom: 10, color: 'var(--zm-text-muted)', fontSize: 12 }}>
                    勾选禅道人员后，将为每位用户自动生成随机密码，并在“创建成功页”一次性展示账号与密码。
                  </div>
                  <Transfer
                    dataSource={batchPersonnel}
                    titles={['禅道人员（local_users）', '将创建的系统用户']}
                    targetKeys={batchTargetKeys}
                    onChange={(nextKeys) => setBatchTargetKeys(nextKeys as string[])}
                    showSearch
                    listStyle={{ width: '100%', height: 280 }}
                    render={(item) => <span>{item.title}</span>}
                    filterOption={(input, item) => {
                      const title = (item?.title ?? '') as string
                      return title.includes(input)
                    }}
                  />

                  <Form
                    form={batchForm}
                    layout="vertical"
                    initialValues={{ role: 'user', data_scope: 'SELF' }}
                    style={{ marginTop: 16 }}
                  >
                    <Space style={{ width: '100%' }} align="start">
                      <Form.Item name="role" label="角色" style={{ flex: 1 }}>
                        <Select options={ROLE_OPTIONS} />
                      </Form.Item>
                      <Form.Item name="data_scope" label="数据范围" style={{ flex: 1 }}>
                        <Select options={SCOPE_OPTIONS} />
                      </Form.Item>
                    </Space>
                    <Form.Item name="default_group_id" label="默认小组（data_scope=GROUP 时必填）">
                      <Select
                        allowClear
                        showSearch
                        optionFilterProp="label"
                        options={groups.map((g) => ({ value: g.id, label: g.name }))}
                      />
                    </Form.Item>
                  </Form>
                </>
              ),
            },
          ]}
        />
      </Modal>

      <Modal
        open={editOpen}
        onCancel={() => setEditOpen(false)}
        title={`编辑用户：${editing?.username ?? ''}`}
        okText="保存"
        onOk={async () => {
          const v = await editForm.validateFields()
          try {
            await adminUpdateSystemUser(editing.id, {
              display_name: v.display_name,
              role: v.role,
              data_scope: v.data_scope,
              default_group_id: v.default_group_id ?? null,
              disabled: !!v.disabled,
            })
            message.success('已保存')
            setEditOpen(false)
            void fetch(page)
          } catch (e: any) {
            message.error(e.response?.data?.error ?? '保存失败')
          }
        }}
      >
        <Form form={editForm} layout="vertical">
          <Form.Item name="display_name" label="显示名">
            <Input />
          </Form.Item>
          <Space style={{ width: '100%' }} align="start">
            <Form.Item name="role" label="角色" style={{ flex: 1 }}>
              <Select options={ROLE_OPTIONS} />
            </Form.Item>
            <Form.Item name="data_scope" label="数据范围" style={{ flex: 1 }}>
              <Select options={SCOPE_OPTIONS} />
            </Form.Item>
          </Space>
          <Form.Item name="default_group_id" label="默认小组（GROUP 用户必填）">
            <Select
              allowClear
              showSearch
              optionFilterProp="label"
              options={groups.map((g) => ({ value: g.id, label: g.name }))}
            />
          </Form.Item>
          <Form.Item name="disabled" label="禁用" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        open={bindOpen}
        onCancel={() => setBindOpen(false)}
        title={`当前操作用户：${bindingUser?.username ?? ''}`}
        okText="保存绑定"
        onOk={async () => {
          if (!bindingUser) return
          if (!zentaoAccount) {
            message.error('请选择禅道账号')
            return
          }
          try {
            await adminSetZentaoBinding(bindingUser.id, zentaoAccount)
            message.success('绑定成功')
            setBindOpen(false)
          } catch (e: any) {
            message.error(e.response?.data?.error ?? '绑定失败（可能已被其他用户绑定）')
          }
        }}
        footer={(_, { OkBtn, CancelBtn }) => (
          <Space style={{ width: '100%', justifyContent: 'space-between' }}>
            <Button
              danger
              onClick={async () => {
                if (!bindingUser) return
                try {
                  await adminDeleteZentaoBinding(bindingUser.id)
                  message.success('已解绑')
                  setBindOpen(false)
                } catch (e: any) {
                  message.error(e.response?.data?.error ?? '解绑失败')
                }
              }}
            >
              解绑
            </Button>
            <Space>
              <CancelBtn />
              <OkBtn />
            </Space>
          </Space>
        )}
      >
        <div style={{ marginBottom: 10, color: 'var(--zm-text-muted)', fontSize: 12 }}>
          绑定用于“SELF 个人口径”筛选与权限裁剪；同一禅道账号禁止绑定多个系统用户。
        </div>
        <Select
          showSearch
          optionFilterProp="label"
          placeholder="选择禅道账号（来自本地同步的 local_users）"
          style={{ width: '100%' }}
          options={zentaoOptions}
          value={zentaoAccount}
          onChange={(v) => setZentaoAccount(v as string)}
        />
      </Modal>
    </div>
  )
}

