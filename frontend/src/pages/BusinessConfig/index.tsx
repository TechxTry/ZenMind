import React, { useEffect, useState } from 'react'
import { Card, Form, InputNumber, Input, Button, Typography, message, Spin, Row, Col, Divider } from 'antd'
import { SaveOutlined } from '@ant-design/icons'
import { getBusinessConfig, putBusinessConfig, getZentaoAPIConfig, putZentaoAPIConfig } from '../../api'
import ZentaoEffortProbeCard from '../../components/ZentaoEffortProbeCard'

const { Title, Text } = Typography

const BusinessConfigPage: React.FC = () => {
  const [form] = Form.useForm()
  const [ztForm] = Form.useForm()
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [savingZtAPI, setSavingZtAPI] = useState(false)

  useEffect(() => {
    setLoading(true)
    getZentaoAPIConfig()
      .then((d: any) => {
        ztForm.setFieldsValue({
          zt_base_url: d?.base_url,
          zt_login_url: d?.login_url,
        })
      })
      .catch(() => {})
    getBusinessConfig()
      .then((d: any) => {
        form.setFieldsValue({
          daily_standard_hours: d?.daily_standard_hours ?? 8,
        })
      })
      .catch(() => {
        message.error('加载业务配置失败')
      })
      .finally(() => setLoading(false))
  }, [form, ztForm])

  const handleSave = async () => {
    const values = await form.validateFields()
    setSaving(true)
    try {
      await putBusinessConfig({ daily_standard_hours: values.daily_standard_hours })
      message.success('业务配置已保存')
    } catch (e: any) {
      message.error(e.response?.data?.error ?? '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const handleSaveZtAPI = async () => {
    const values = await ztForm.validateFields()
    setSavingZtAPI(true)
    try {
      await putZentaoAPIConfig({ base_url: values.zt_base_url, login_url: values.zt_login_url })
      message.success('禅道 API 已保存')
    } catch (e: any) {
      if (e?.errorFields) return
      message.error(e.response?.data?.error ?? '保存失败')
    } finally {
      setSavingZtAPI(false)
    }
  }

  const cardStyle = {
    background: 'var(--zm-bg-surface)',
    border: '1px solid var(--zm-border-subtle)',
    borderRadius: 12,
  }

  return (
    <div style={{ maxWidth: 1000 }}>
      <Title level={4} style={{ color: 'var(--zm-text-primary)', marginBottom: 24 }}>
        业务配置
      </Title>

      <Spin spinning={loading}>
        <Card
          title={<Text style={{ color: 'var(--zm-text-primary)' }}>工时标准</Text>}
          style={cardStyle}
          styles={{ header: { borderBottom: '1px solid var(--zm-border-subtle)' } }}
          extra={
            <Button
              type="primary"
              icon={<SaveOutlined />}
              loading={saving}
              onClick={handleSave}
              style={{ background: 'var(--zm-brand-gradient)', border: 'none' }}
            >
              保存
            </Button>
          }
        >
          <Form form={form} layout="vertical">
            <Form.Item
              name="daily_standard_hours"
              label={<Text style={{ color: 'var(--zm-text-secondary)' }}>每日标准工时</Text>}
              rules={[
                { required: true, message: '请输入每日标准工时' },
                { type: 'number', min: 1, max: 24, message: '范围 1～24 小时' },
              ]}
              initialValue={8}
            >
              <InputNumber
                min={1}
                max={24}
                style={{ width: 120 }}
                addonAfter="小时"
              />
            </Form.Item>
            <Text style={{ color: 'var(--zm-text-muted)', fontSize: 12 }}>
              用于分析看板等模块计算员工每日工作饱和度，范围 1～24 小时，默认 8 小时。
            </Text>
          </Form>
        </Card>

        <Divider style={{ borderColor: 'var(--zm-border-subtle)' }} />

        <Card
          title={<Text style={{ color: 'var(--zm-text-primary)' }}>禅道 API（用于写入报工等）</Text>}
          style={cardStyle}
          styles={{ header: { borderBottom: '1px solid var(--zm-border-subtle)' } }}
          extra={
            <Button
              type="primary"
              size="small"
              onClick={handleSaveZtAPI}
              loading={savingZtAPI}
              style={{ background: 'var(--zm-brand-gradient)', border: 'none' }}
            >
              保存
            </Button>
          }
        >
          <Form form={ztForm} layout="vertical">
            <Row gutter={16}>
              <Col span={12}>
                <Form.Item
                  name="zt_base_url"
                  label={<Text style={{ color: 'var(--zm-text-secondary)' }}>Base URL</Text>}
                  rules={[{ required: true, message: '请输入禅道地址，如 https://zentao.example.com' }]}
                >
                  <Input placeholder="https://zentao.example.com" />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item
                  name="zt_login_url"
                  label={<Text style={{ color: 'var(--zm-text-secondary)' }}>登录 URL</Text>}
                  rules={[{ required: true, message: '请输入禅道登录地址，如 https://.../user-login.html' }]}
                >
                  <Input placeholder="https://zentao.example.com/user-login.html" />
                </Form.Item>
              </Col>
            </Row>
            <Text style={{ color: 'var(--zm-text-muted)', fontSize: 12 }}>
              Base URL 用于后续接口回写；登录 URL 用于“方案1：登录换会话”的授权（例如你们现在的 `/user-login-xxx.html`）。
            </Text>
          </Form>
        </Card>

        <Divider style={{ borderColor: 'var(--zm-border-subtle)' }} />

        <ZentaoEffortProbeCard />
      </Spin>
    </div>
  )
}

export default BusinessConfigPage
