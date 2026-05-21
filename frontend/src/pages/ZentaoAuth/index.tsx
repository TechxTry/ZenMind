import React, { useEffect, useMemo, useState } from 'react'
import { Card, Form, Input, Button, Space, Typography, message, Alert } from 'antd'
import {
  getZentaoAPIConfig,
  testZentaoAuth,
  testZentaoAuthSaved,
  bindZentaoAuth,
  bindZentaoAuthSaved,
  getZentaoAuthStatus,
  clearZentaoAuth,
} from '../../api'
import { useAuthStore } from '../../store/auth'

const { Title, Text } = Typography

const ZentaoAuthPage: React.FC = () => {
  const [form] = Form.useForm()
  const [testing, setTesting] = useState(false)
  const [binding, setBinding] = useState(false)
  const [bound, setBound] = useState<boolean | null>(null)
  const [credentialSaved, setCredentialSaved] = useState(false)
  const [redisUnavailable, setRedisUnavailable] = useState(false)
  const [loginURL, setLoginURL] = useState<string>('')
  const [savedAccount, setSavedAccount] = useState<string>('')
  const me = useAuthStore((s) => s.me)
  const requiredAccount = (me?.zentao_binding?.zentao_account ?? '').trim()

  const canUseSavedCredential = useMemo(() => {
    if (!credentialSaved || !savedAccount || !requiredAccount) return false
    return savedAccount === requiredAccount
  }, [credentialSaved, requiredAccount, savedAccount])

  const openZentaoLogin = () => {
    const url = (loginURL ?? '').trim()
    if (!url) {
      message.warning('尚未配置禅道登录 URL')
      return
    }
    const w = window.open(url, '_blank', 'noopener,noreferrer')
    // 浏览器可能阻止聚焦；这里尽力尝试
    try {
      w?.focus()
    } catch (_) {
      // ignore
    }
  }

  const refreshStatus = async () => {
    try {
      const r = await getZentaoAuthStatus()
      setBound(!!r?.bound)
      setCredentialSaved(!!r?.credential_saved)
      setRedisUnavailable(!!r?.redis_unavailable)
      const acct = r?.account ?? ''
      setSavedAccount(acct)
    } catch (e: any) {
      setBound(null)
      setCredentialSaved(false)
      setRedisUnavailable(false)
      message.warning('刷新状态失败：' + (e?.response?.data?.error ?? e?.message ?? '未知错误'))
    }
  }

  useEffect(() => {
    getZentaoAPIConfig()
      .then((d: any) => setLoginURL(d?.login_url ?? ''))
      .catch(() => setLoginURL(''))
    refreshStatus()
  }, [])

  const handleTest = async () => {
    const v = form.getFieldsValue()
    const pwd = (v.password ?? '').trim()
    setTesting(true)
    try {
      let r: { ok?: boolean; error?: string }
      if (pwd) {
        await form.validateFields(['password'])
        if (!requiredAccount) {
          message.warning('请先在账号管理中绑定禅道账号')
          return
        }
        r = await testZentaoAuth({ password: pwd })
      } else if (canUseSavedCredential) {
        r = await testZentaoAuthSaved()
      } else {
        message.warning('请输入禅道密码后再测试，或在绑定未变更且已保存凭证时留空以复用已保存密码')
        return
      }
      if (r.ok) {
        message.success('登录成功（测试通过）')
      } else {
        message.error('登录失败：' + (r.error ?? '未知错误'))
      }
    } catch (e: any) {
      if (e?.errorFields) return
      message.error(e.response?.data?.error ?? '请求失败')
    } finally {
      setTesting(false)
    }
  }

  const handleBind = async () => {
    setBinding(true)
    try {
      const v = form.getFieldsValue()
      const pwd = (v.password ?? '').trim()

      let r: { ok?: boolean; error?: string }
      if (pwd) {
        await form.validateFields(['password'])
        if (!requiredAccount) {
          message.warning('请先在账号管理中绑定禅道账号')
          return
        }
        r = await bindZentaoAuth({ password: pwd })
      } else if (canUseSavedCredential) {
        r = await bindZentaoAuthSaved()
      } else {
        message.warning('请输入禅道密码后再保存，或在绑定与已保存账号一致时留空以复用已保存密码')
        return
      }
      if (r.ok) {
        message.success('授权已保存（会话已绑定）')
        // 不在页面上保留明文密码
        form.setFieldsValue({ password: '' })
        refreshStatus()
      } else {
        message.error('授权失败：' + (r.error ?? '未知错误'))
      }
    } catch (e: any) {
      message.error(e.response?.data?.error ?? '请求失败')
    } finally {
      setBinding(false)
    }
  }

  const handleClear = async () => {
    try {
      await clearZentaoAuth()
      message.success('已清除保存的禅道凭证与会话')
      refreshStatus()
    } catch (e: any) {
      message.error(e.response?.data?.error ?? '清除失败')
    }
  }

  const cardStyle = {
    background: 'var(--zm-bg-surface)',
    border: '1px solid var(--zm-border-subtle)',
    borderRadius: 12,
    maxWidth: 720,
  }

  return (
    <div>
      <Title level={4} style={{ color: 'var(--zm-text-primary)', marginBottom: 12 }}>禅道授权（登录换会话）</Title>
      <Text style={{ color: 'var(--zm-text-muted)' }}>
        用于后续“报工写入”等操作，以当前禅道账号身份调用接口。密码在库中加密保存，并用于刷新 Redis 中的短期登录会话。
      </Text>

      <div style={{ height: 16 }} />

      {loginURL ? (
        <Alert
          type="info"
          showIcon
          message="当前登录 URL"
          description={
            <div>
              <Text style={{ color: 'var(--zm-text-muted)' }}>{loginURL}</Text>
              <div style={{ height: 8 }} />
              <Space>
                <Button type="primary" onClick={openZentaoLogin}
                  style={{ background: 'var(--zm-brand-gradient)', border: 'none' }}>
                  打开禅道登录页
                </Button>
              </Space>
            </div>
          }
          style={{ marginBottom: 16, maxWidth: 720 }}
        />
      ) : (
        <Alert
          type="warning"
          showIcon
          message="尚未配置登录 URL"
          description={<Text style={{ color: 'var(--zm-text-muted)' }}>请先到「业务配置」里配置禅道登录 URL。</Text>}
          style={{ marginBottom: 16, maxWidth: 720 }}
        />
      )}

      <Card
        title={<Text style={{ color: 'var(--zm-text-primary)' }}>禅道密码与会话</Text>}
        style={cardStyle}
        styles={{ header: { borderBottom: '1px solid var(--zm-border-subtle)' } }}
        extra={
          <Space>
            <Button onClick={refreshStatus}>刷新状态</Button>
            <Button danger onClick={handleClear}>清除绑定</Button>
            <Button onClick={handleTest} loading={testing} disabled={!requiredAccount}>测试连通性</Button>
            <Button type="primary" onClick={handleBind} loading={binding} disabled={!requiredAccount}
              style={{ background: 'var(--zm-brand-gradient)', border: 'none' }}>
              保存授权
            </Button>
          </Space>
        }
      >
        <div style={{ marginBottom: 12 }}>
          {!requiredAccount ? (
            <Alert
              type="warning"
              showIcon
              style={{ marginBottom: 12 }}
              message="当前用户未绑定禅道账号"
              description="禅道登录账号由「账号管理」中的绑定决定，任何角色均不能在本页修改。请先在账号管理中完成绑定后再输入密码授权。"
            />
          ) : null}
          <Text style={{ color: 'var(--zm-text-secondary)' }}>
            当前状态：
            {bound == null
              ? '未知'
              : bound
                ? '已绑定（Redis 会话有效）'
                : credentialSaved
                  ? '凭证已保存，会话未建立或已失效（可点「保存授权」重建会话）'
                  : '未保存凭证'}
            {redisUnavailable ? '；Redis 不可用，无法维护会话' : ''}
          </Text>
          {savedAccount ? (
            <Text style={{ color: 'var(--zm-text-muted)', marginLeft: 12 }}>
              已保存账号：{savedAccount}
            </Text>
          ) : null}
          {requiredAccount ? (
            <Text style={{ color: 'var(--zm-text-muted)', marginLeft: 12 }}>
              绑定账号：{requiredAccount}（仅在账号管理中修改）
            </Text>
          ) : null}
        </div>
        <Form form={form} layout="vertical">
          <div style={{ marginBottom: 16 }}>
            <Text style={{ color: 'var(--zm-text-secondary)', display: 'block', marginBottom: 8 }}>禅道账号</Text>
            <Input
              value={requiredAccount || ''}
              placeholder={requiredAccount ? undefined : '未绑定'}
              readOnly
              disabled
              autoComplete="off"
            />
          </div>
          <Form.Item
            name="password"
            label={<Text style={{ color: 'var(--zm-text-secondary)' }}>禅道密码</Text>}
            extra={
              canUseSavedCredential
                ? <Text style={{ color: 'var(--zm-text-muted)' }}>已保存密码：无需重复输入；如需覆盖为新密码，直接输入后点「保存授权」。</Text>
                : <Text style={{ color: 'var(--zm-text-muted)' }}>密码将加密保存，用于后续自动换取 Token 与重建会话。登录所用账号始终为上方绑定账号。</Text>
            }
            rules={[
              {
                validator: async (_, value) => {
                  const pwd = (value ?? '').trim()
                  if (!pwd && canUseSavedCredential) return
                  if (!pwd) throw new Error('请输入禅道密码')
                },
              },
            ]}
          >
            <Input.Password
              placeholder={canUseSavedCredential ? '已保存（可留空）' : '••••••••'}
              autoComplete="new-password"
            />
          </Form.Item>
        </Form>
      </Card>
    </div>
  )
}

export default ZentaoAuthPage

