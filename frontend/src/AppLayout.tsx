import { Layout, Menu, Select, Space, Typography, Avatar, Button, Tooltip } from 'antd'
import {
  SettingOutlined, BarChartOutlined, LogoutOutlined, UserOutlined, MenuFoldOutlined, MenuUnfoldOutlined,
  GithubOutlined,
} from '@ant-design/icons'
import { APP_VERSION, GITHUB_REPO_URL } from './constants/release'
import { useNavigate, useLocation, Outlet } from 'react-router-dom'
import { useAppStore } from './store'
import { useEffect, useMemo, useState } from 'react'
import { useAuthStore } from './store/auth'

const { Header, Sider, Content } = Layout
const { Text } = Typography

const AppLayout: React.FC = () => {
  const navigate = useNavigate()
  const location = useLocation()
  const { themeMode, setThemeMode } = useAppStore()
  const me = useAuthStore((s) => s.me)
  const setMe = useAuthStore((s) => s.setMe)
  const [collapsed, setCollapsed] = useState<boolean>(() => localStorage.getItem('zb.sidebar.collapsed') === 'true')
  const [systemTheme, setSystemTheme] = useState<'light' | 'dark'>(() =>
    window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light',
  )
  const [openKeys, setOpenKeys] = useState<string[]>([])

  useEffect(() => {
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = () => setSystemTheme(mq.matches ? 'dark' : 'light')
    onChange()

    mq.addEventListener('change', onChange)

    return () => {
      mq.removeEventListener('change', onChange)
    }
  }, [])

  const resolvedTheme = useMemo<'light' | 'dark'>(
    () => (themeMode === 'system' ? systemTheme : themeMode),
    [systemTheme, themeMode],
  )

  const selectedKey = useMemo(() => {
    const p = location.pathname
    const from = new URLSearchParams(location.search).get('from')
    if (p.startsWith('/analytics/iteration')) return '/analytics/iteration'
    if (p.startsWith('/analytics/people')) return '/analytics/people'
    if (p.startsWith('/analytics/team-health')) return '/analytics/team-health'
    if (p.startsWith('/my-workbench')) return '/my-workbench'
    if (p.startsWith('/my-bugs')) return '/my-bugs'
    if (p.startsWith('/calendar-accounts')) return '/calendar-accounts'
    if (p.startsWith('/mcp-access')) return '/mcp-access'
    if (p.startsWith('/zentao-auth')) return '/zentao-auth'
    if (p.startsWith('/workbench/task/')) {
      if (from === '/my-workbench') return '/my-workbench'
      if (from === '/workbench') return '/workbench'
    }
    if (p.startsWith('/workbench')) return '/workbench'
    if (p.startsWith('/config')) return '/config'
    if (p.startsWith('/business-config')) return '/business-config'
    if (p.startsWith('/groups')) return '/groups'
    if (p.startsWith('/admin/system-users')) return '/admin/system-users'
    if (p.startsWith('/admin/audit-logs')) return '/admin/audit-logs'
    return '/my-workbench'
  }, [location.pathname, location.search])

  const selectedTopKey = useMemo(() => {
    if (selectedKey.startsWith('/analytics')) return 'menu_analytics'
    if (
      selectedKey.startsWith('/my-workbench')
      || selectedKey.startsWith('/my-bugs')
      || selectedKey.startsWith('/calendar-accounts')
      || selectedKey.startsWith('/mcp-access')
      || selectedKey.startsWith('/zentao-auth')
    ) return 'menu_personal'
    const role = (me?.user?.role ?? '').toLowerCase()
    const isAdmin = role === 'admin' || role === 'super_admin'
    if (isAdmin) return 'menu_system'
    return 'menu_personal'
  }, [selectedKey, me?.user?.role])

  useEffect(() => {
    if (!collapsed) {
      setOpenKeys([selectedTopKey])
    }
  }, [collapsed, selectedTopKey])

  useEffect(() => {
    localStorage.setItem('zb.sidebar.collapsed', String(collapsed))
  }, [collapsed])

  const menuItems = useMemo(
    () => {
      const role = (me?.user?.role ?? '').toLowerCase()
      const isAdmin = role === 'admin' || role === 'super_admin'
      const items: any[] = [
        {
          key: 'menu_personal',
          icon: <UserOutlined />,
          label: '个人工作台',
          children: [
            { key: '/my-workbench', label: '我的工作台' },
            { key: '/my-bugs', label: '我的Bug' },
            { key: '/calendar-accounts', label: '日历账户' },
            { key: '/mcp-access', label: 'MCP 访问' },
            { key: '/zentao-auth', label: '禅道授权' },
          ],
        },
        {
          key: 'menu_analytics',
          icon: <BarChartOutlined />,
          label: '分析看板',
          children: [
            { key: '/analytics/iteration', label: '迭代看板' },
            { key: '/analytics/people', label: '员工看板' },
            { key: '/analytics/team-health', label: '团队健康度' },
          ],
        },
      ]

      if (isAdmin) {
        items.push({
          key: 'menu_system',
          icon: <SettingOutlined />,
          label: '系统管理',
          children: [
            { key: '/config', label: '数据同步' },
            { key: '/business-config', label: '业务配置' },
            { key: '/groups', label: '小组管理' },
            { key: '/workbench', label: '数据明细' },
            { key: '/admin/system-users', label: '账号管理' },
            { key: '/admin/audit-logs', label: '审计日志' },
          ],
        })
      }

      return items
    },
    [me?.user?.role],
  )

  const logout = () => {
    localStorage.removeItem('token')
    setMe(null)
    navigate('/login')
  }

  return (
    <Layout
      style={{
        height: '100vh',
        overflow: 'hidden',
        background: 'var(--zm-bg-canvas)',
      }}
    >
      <Sider
        theme={resolvedTheme}
        width={220}
        collapsible
        collapsed={collapsed}
        collapsedWidth={72}
        trigger={null}
        style={{
          height: '100vh',
          overflowY: 'auto',
          position: 'relative',
          background: 'var(--zm-bg-surface)',
          borderRight: '1px solid var(--zm-border-subtle)',
        }}
      >
        <div style={{
          padding: collapsed ? '24px 12px 16px' : '24px 20px 16px',
          borderBottom: '1px solid var(--zm-border-subtle)',
          marginBottom: 8,
        }}>
          <Space
            size={collapsed ? 0 : 12}
            style={{ width: '100%', justifyContent: collapsed ? 'center' : 'flex-start' }}
          >
            <div style={{
              width: 32, height: 32, borderRadius: 8,
              background: 'var(--zm-brand-gradient)',
              display: 'flex', alignItems: 'center', justifyContent: 'center',
              fontSize: 16,
            }}>🧘</div>
            {!collapsed && <Text strong style={{ fontSize: 15, color: 'var(--zm-text-primary)' }}>ZenMind</Text>}
          </Space>
        </div>

        <Menu
          theme={resolvedTheme}
          mode="inline"
          selectedKeys={[selectedKey]}
          openKeys={openKeys}
          onOpenChange={(keys) => {
            if (!collapsed) {
              setOpenKeys(keys as string[])
            }
          }}
          style={{ background: 'transparent', border: 'none', padding: '0 8px' }}
          onClick={({ key }) => navigate(key)}
          items={menuItems}
        />

        <div style={{
          position: 'absolute',
          bottom: 16,
          left: 0,
          right: 0,
          padding: collapsed ? '0 12px' : '0 16px',
          display: 'flex',
          flexDirection: 'column',
          gap: 4,
        }}
        >
          <Tooltip
            placement="right"
            title={collapsed ? `GitHub · ${APP_VERSION}` : null}
          >
            <a
              href={GITHUB_REPO_URL}
              target="_blank"
              rel="noopener noreferrer"
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: collapsed ? 'center' : 'flex-start',
                gap: 8,
                padding: '8px 12px',
                borderRadius: 8,
                color: 'var(--zm-text-muted)',
                fontSize: 12,
                textDecoration: 'none',
                transition: 'all .2s',
              }}
              onMouseEnter={(e) => (e.currentTarget.style.background = 'var(--zm-bg-hover)')}
              onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
            >
              <GithubOutlined style={{ fontSize: 14 }} />
              {!collapsed && (
                <span style={{ flex: 1, minWidth: 0 }}>
                  <span style={{ display: 'block', lineHeight: 1.4 }}>GitHub</span>
                  <span style={{
                    display: 'block',
                    fontSize: 11,
                    color: 'var(--zm-text-muted)',
                    opacity: 0.85,
                    fontVariantNumeric: 'tabular-nums',
                  }}
                  >
                    {APP_VERSION}
                  </span>
                </span>
              )}
            </a>
          </Tooltip>

          <Tooltip placement="right" title={collapsed ? '退出登录' : null}>
            <div
              onClick={logout}
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: collapsed ? 'center' : 'flex-start',
                gap: 8,
                padding: '10px 12px',
                borderRadius: 8,
                cursor: 'pointer',
                color: 'var(--zm-text-muted)',
                transition: 'all .2s',
              }}
              onMouseEnter={(e) => (e.currentTarget.style.background = 'var(--zm-bg-hover)')}
              onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
            >
              <LogoutOutlined /> {!collapsed && '退出登录'}
            </div>
          </Tooltip>
        </div>
      </Sider>

      <Layout
        style={{
          flex: 1,
          minWidth: 0,
          minHeight: 0,
          overflow: 'hidden',
          display: 'flex',
          flexDirection: 'column',
        }}
      >
        <Header
          style={{
            flexShrink: 0,
            background: 'var(--app-header-bg)',
            backdropFilter: 'blur(12px)',
            borderBottom: '1px solid var(--zm-border-subtle)',
            display: 'flex', alignItems: 'center', justifyContent: 'space-between',
            padding: '0 24px', height: 56,
          }}
        >
          <Tooltip title={collapsed ? '展开导航' : '收起导航'}>
            <Button
              type="text"
              icon={collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
              onClick={() => setCollapsed((prev) => !prev)}
              aria-label={collapsed ? '展开导航' : '收起导航'}
            />
          </Tooltip>
          <Space>
            <Select
              value={themeMode}
              onChange={(val) => setThemeMode(val)}
              style={{ width: 140 }}
              options={[
                { value: 'system', label: '跟随系统' },
                { value: 'light', label: '浅色' },
                { value: 'dark', label: '深色' },
              ]}
            />
            <Avatar
              style={{ background: 'var(--zm-brand-gradient)', cursor: 'default' }}
              size={32}
            >
              {(me?.user?.display_name || me?.user?.username || 'U').slice(0, 1).toUpperCase()}
            </Avatar>
          </Space>
        </Header>

        <Content
          style={{
            flex: 1,
            minHeight: 0,
            margin: 24,
            background: 'transparent',
            borderRadius: 12,
            overflow: 'auto',
          }}
        >
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  )
}

export default AppLayout
