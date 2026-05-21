import React, { useEffect, useState } from 'react'
import { Alert, Button, Card, Divider, InputNumber, Space, Tag, Tooltip, Typography, message } from 'antd'
import { useNavigate } from 'react-router-dom'
import { getZentaoAuthStatus, probeZentaoAuth, type ZentaoProbeEndpoint, type ZentaoProbeResult } from '../api'

const { Text } = Typography

type ZentaoAuthStatus = {
  bound?: boolean
  credential_saved?: boolean
  redis_unavailable?: boolean
  account?: string
}

const ZentaoEffortProbeCard: React.FC = () => {
  const navigate = useNavigate()
  const [checkingStatus, setCheckingStatus] = useState(false)
  const [probing, setProbing] = useState(false)
  const [probeTaskId, setProbeTaskId] = useState<number | undefined>()
  const [probeResult, setProbeResult] = useState<ZentaoProbeResult | null>(null)
  const [authStatus, setAuthStatus] = useState<ZentaoAuthStatus | null>(null)

  const refreshStatus = async () => {
    setCheckingStatus(true)
    try {
      const r = await getZentaoAuthStatus()
      setAuthStatus(r ?? null)
    } catch (e: any) {
      message.warning('刷新授权状态失败：' + (e?.response?.data?.error ?? e?.message ?? '未知错误'))
    } finally {
      setCheckingStatus(false)
    }
  }

  useEffect(() => {
    void refreshStatus()
  }, [])

  const handleProbe = async () => {
    if (!authStatus?.bound) {
      message.warning('请先到「禅道授权」完成保存授权，诊断需要使用已绑定的会话 Cookie')
      return
    }
    setProbing(true)
    setProbeResult(null)
    try {
      const r = await probeZentaoAuth(
        probeTaskId && probeTaskId > 0 ? { task_id: probeTaskId } : undefined,
      )
      if (r.ok && r.result) {
        setProbeResult(r.result)
        if (r.result.recommended_url) {
          message.success('诊断完成：已找到可用报工 URL')
        } else {
          message.warning('诊断完成：未在候选 URL 里找到可用报工表单，请查看下方详情')
        }
      } else {
        message.error(r.error ?? '诊断失败')
      }
    } catch (e: any) {
      message.error(e?.response?.data?.error ?? '诊断失败')
    } finally {
      setProbing(false)
    }
  }

  return (
    <Card
      title={<Text style={{ color: 'var(--zm-text-primary)' }}>禅道报工接口诊断</Text>}
      style={{
        background: 'var(--zm-bg-surface)',
        border: '1px solid var(--zm-border-subtle)',
        borderRadius: 12,
      }}
      styles={{ header: { borderBottom: '1px solid var(--zm-border-subtle)' } }}
      extra={
        <Space wrap>
          <Button onClick={() => void refreshStatus()} loading={checkingStatus}>
            刷新授权状态
          </Button>
          <Tooltip title="用真实任务 ID 诊断可得到更准确的表单字段；留空会用 1 做占位探测">
            <InputNumber
              min={1}
              style={{ width: 160 }}
              placeholder="任务 ID（可选）"
              value={probeTaskId}
              onChange={(v) => setProbeTaskId(v ?? undefined)}
            />
          </Tooltip>
          <Button
            type="primary"
            onClick={() => void handleProbe()}
            loading={probing}
            disabled={!authStatus?.bound}
            style={{ background: 'var(--zm-brand-gradient)', border: 'none' }}
          >
            开始诊断
          </Button>
        </Space>
      }
    >
      <Text style={{ color: 'var(--zm-text-muted)', fontSize: 12 }}>
        使用当前绑定的会话 Cookie，对禅道常见报工 URL 做<strong>只读 GET 探测</strong>，检测真实 URL、表单字段名与
        CSRF 名称。结果用于后端对齐写入逻辑，不会产生任何数据副作用。
      </Text>

      <div style={{ marginTop: 12 }}>
        <Space wrap>
          <Tag color={authStatus?.bound ? 'green' : 'default'}>
            {authStatus?.bound ? '会话已绑定' : '会话未绑定'}
          </Tag>
          {authStatus?.credential_saved ? <Tag color="blue">凭证已保存</Tag> : null}
          {authStatus?.account ? <Tag>{authStatus.account}</Tag> : null}
          {authStatus?.redis_unavailable ? <Tag color="red">Redis 不可用</Tag> : null}
        </Space>
      </div>

      {!authStatus?.bound ? (
        <Alert
          style={{ marginTop: 12 }}
          type="warning"
          showIcon
          message="当前没有可用的禅道授权会话"
          description="请先到“禅道授权”页完成保存授权，再回到这里执行报工诊断。"
          action={
            <Button size="small" onClick={() => navigate('/zentao-auth')}>
              去禅道授权
            </Button>
          }
        />
      ) : null}

      {probeResult ? <ProbeResultView data={probeResult} /> : null}
    </Card>
  )
}

const ProbeResultView: React.FC<{ data: ZentaoProbeResult }> = ({ data }) => {
  return (
    <div style={{ marginTop: 16 }}>
      <Space wrap>
        <Tag color={data.session_valid ? 'green' : 'red'}>
          会话 {data.session_valid ? '有效' : '无效/已过期'}
        </Tag>
        <Tag>base: {data.base_url}</Tag>
        <Tag>task_id: {data.used_task_id}</Tag>
      </Space>

      {data.recommended_url ? (
        <Alert
          style={{ marginTop: 12 }}
          type="success"
          showIcon
          message={`推荐提交方式：${data.recommended_mode === 'api_v1' ? 'REST API v1（禅道 15+ Biz/Pro/Max）' : 'Webform 表单提交'}`}
          description={
            <div style={{ fontFamily: 'monospace', fontSize: 12 }}>
              <div>URL: {data.recommended_url}</div>
              {data.recommended_csrf ? <div>CSRF 字段名: {data.recommended_csrf}</div> : null}
              {data.recommended_fields && data.recommended_fields.length > 0 ? (
                <div>表单字段: {data.recommended_fields.join(', ')}</div>
              ) : null}
            </div>
          }
        />
      ) : (
        <Alert
          style={{ marginTop: 12 }}
          type="warning"
          showIcon
          message="未找到可用的报工表单"
          description="候选 URL 都返回 200 但 HTML 没有真实表单（可能被 JS 渲染），API 也不可用；请手动抓一次报工 POST 请求贴给后端。"
        />
      )}

      {data.notes && data.notes.length > 0 ? (
        <ul style={{ marginTop: 8, color: 'var(--zm-text-muted)', fontSize: 12 }}>
          {data.notes.map((n, i) => (
            <li key={i}>{n}</li>
          ))}
        </ul>
      ) : null}

      <Divider style={{ margin: '16px 0 8px' }} />
      <Text style={{ color: 'var(--zm-text-secondary)' }}>会话探测</Text>
      <ProbeEndpointTable rows={data.session_check} />

      <Divider style={{ margin: '16px 0 8px' }} />
      <Text style={{ color: 'var(--zm-text-secondary)' }}>报工 URL 候选（webform）</Text>
      <ProbeEndpointTable rows={data.effort_endpoints} showFields showSnippet />

      <Divider style={{ margin: '16px 0 8px' }} />
      <Text style={{ color: 'var(--zm-text-secondary)' }}>REST API v1 探测（禅道 15+ Biz/Pro/Max）</Text>
      <ProbeEndpointTable rows={data.api_endpoints} showFields showSnippet />

      {data.api_login ? (
        <>
          <Divider style={{ margin: '16px 0 8px' }} />
          <Text style={{ color: 'var(--zm-text-secondary)' }}>API 登录实测（POST /api.php/v1/tokens）</Text>
          <div style={{ marginTop: 8 }}>
            {data.api_login.ok ? (
              <Alert
                type="success"
                showIcon
                message="换取 Token 成功"
                description={
                  <div style={{ fontFamily: 'monospace', fontSize: 12 }}>
                    <div>Token 长度: {data.api_login.token_length}</div>
                    {data.api_login.token_preview ? <div>Token 预览: {data.api_login.token_preview}</div> : null}
                    {data.api_login.expire_seconds ? <div>到期秒数: {data.api_login.expire_seconds}</div> : null}
                    {data.api_login.account ? <div>账号: {data.api_login.account}</div> : null}
                  </div>
                }
              />
            ) : (
              <Alert
                type="error"
                showIcon
                message="换取 Token 失败"
                description={<div style={{ fontFamily: 'monospace', fontSize: 12 }}>{data.api_login.error ?? '未知错误'}</div>}
              />
            )}
          </div>
        </>
      ) : null}
    </div>
  )
}

const ProbeEndpointTable: React.FC<{
  rows: ZentaoProbeEndpoint[]
  showFields?: boolean
  showSnippet?: boolean
}> = ({ rows, showFields, showSnippet }) => {
  if (!rows || rows.length === 0) return null
  return (
    <div style={{ marginTop: 8, overflowX: 'auto' }}>
      <table
        style={{
          width: '100%',
          fontSize: 12,
          borderCollapse: 'collapse',
          color: 'var(--zm-text-secondary)',
        }}
      >
        <thead>
          <tr style={{ textAlign: 'left', color: 'var(--zm-text-muted)' }}>
            <th style={{ padding: '6px 8px' }}>Label</th>
            <th style={{ padding: '6px 8px' }}>URL</th>
            <th style={{ padding: '6px 8px' }}>Status</th>
            {showFields ? <th style={{ padding: '6px 8px' }}>Form action / CSRF / 字段</th> : null}
            <th style={{ padding: '6px 8px' }}>备注</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => {
            const ok = r.status >= 200 && r.status < 400 && !r.is_login_page && !r.error
            return (
              <React.Fragment key={i}>
                <tr style={{ borderTop: '1px solid var(--zm-border-subtle)' }}>
                  <td style={{ padding: '6px 8px' }}>
                    {r.method ? <Tag>{r.method}</Tag> : null}
                    {r.label}
                    {r.zin_detected ? <Tag color="purple" style={{ marginLeft: 6 }}>ZIN</Tag> : null}
                  </td>
                  <td style={{ padding: '6px 8px', fontFamily: 'monospace', wordBreak: 'break-all' }}>
                    {r.url}
                  </td>
                  <td style={{ padding: '6px 8px' }}>
                    <Tag color={ok ? 'green' : r.is_login_page ? 'orange' : 'red'}>
                      {r.error ? 'err' : r.status}
                    </Tag>
                    {r.redirected ? <Tag>redir</Tag> : null}
                    {r.is_login_page ? <Tag color="orange">login</Tag> : null}
                    {r.content_type && r.content_type.toLowerCase().includes('json') ? <Tag color="blue">json</Tag> : null}
                  </td>
                  {showFields ? (
                    <td style={{ padding: '6px 8px', fontFamily: 'monospace', maxWidth: 320, wordBreak: 'break-all' }}>
                      {r.form_action ? <div>action: {r.form_action}</div> : null}
                      {r.csrf_name ? <div>csrf: {r.csrf_name}</div> : null}
                      {r.found_fields && r.found_fields.length > 0 ? (
                        <div style={{ color: 'var(--zm-text-muted)' }}>
                          {r.found_fields.join(', ')}
                        </div>
                      ) : null}
                    </td>
                  ) : null}
                  <td style={{ padding: '6px 8px', color: 'var(--zm-text-muted)' }}>
                    {r.error ?? (r.redirected ? `→ ${r.final_url}` : r.content_type ?? '')}
                  </td>
                </tr>
                {showSnippet && r.body_snippet ? (
                  <tr style={{ background: 'rgba(255,255,255,0.02)' }}>
                    <td
                      colSpan={showFields ? 5 : 4}
                      style={{
                        padding: '4px 8px 10px',
                        fontFamily: 'monospace',
                        fontSize: 11,
                        color: 'var(--zm-text-muted)',
                        wordBreak: 'break-all',
                      }}
                    >
                      {r.body_snippet}
                    </td>
                  </tr>
                ) : null}
              </React.Fragment>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

export default ZentaoEffortProbeCard
