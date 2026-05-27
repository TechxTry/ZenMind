import React, { useEffect, useMemo, useState } from 'react'
import { createBrowserRouter, RouterProvider, Navigate } from 'react-router-dom'
import { ConfigProvider, Spin, theme } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import AppLayout from './AppLayout'
import LoginPage from './pages/Login'
import ConfigPage from './pages/Config'
import BusinessConfigPage from './pages/BusinessConfig'
import GroupsPage from './pages/Groups'
import WorkbenchPage from './pages/Workbench'
import TaskDetailPage from './pages/Workbench/TaskDetail'
import ZentaoAuthPage from './pages/ZentaoAuth'
import MyWorkbenchPage from './pages/MyWorkbench'
import CalendarAccountsPage from './pages/MyWorkbench/CalendarAccounts'
import McpAccessPage from './pages/MyWorkbench/McpAccess'
import IterationDashboardPage from './pages/Analytics/IterationDashboard'
import PeopleDashboardPage from './pages/Analytics/PeopleDashboard'
import TeamHealthPage from './pages/Analytics/TeamHealth'
import SystemUsersPage from './pages/Admin/SystemUsers'
import AuditLogsPage from './pages/Admin/AuditLogs'
import SystemUsersCreateSuccessPage from './pages/Admin/SystemUsersCreateSuccess'
import { useAppStore } from './store'
import { useAuthStore } from './store/auth'

const isAuthenticated = () => !!localStorage.getItem('token')

const AuthBootstrap: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const me = useAuthStore((s) => s.me)
  const loading = useAuthStore((s) => s.loading)
  const fetchMe = useAuthStore((s) => s.fetchMe)

  useEffect(() => {
    if (isAuthenticated() && !me && !loading) {
      void fetchMe()
    }
  }, [fetchMe, loading, me])

  if (isAuthenticated() && !me) {
    return (
      <div style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <Spin size="large" />
      </div>
    )
  }
  return <>{children}</>
}

const ProtectedRoute: React.FC<{ element: React.ReactNode }> = ({ element }) => {
  return isAuthenticated() ? <AuthBootstrap>{element}</AuthBootstrap> : <Navigate to="/login" replace />
}

const router = createBrowserRouter([
  {
    path: '/login',
    element: <LoginPage />,
  },
  {
    path: '/',
    element: <ProtectedRoute element={<AppLayout />} />,
    children: [
      { index: true, element: <Navigate to="/my-workbench" replace /> },
      { path: 'config', element: <ConfigPage /> },
      { path: 'business-config', element: <BusinessConfigPage /> },
      { path: 'zentao-auth', element: <ZentaoAuthPage /> },
      { path: 'groups', element: <GroupsPage /> },
      { path: 'my-workbench', element: <MyWorkbenchPage /> },
      { path: 'calendar-accounts', element: <CalendarAccountsPage /> },
      { path: 'mcp-access', element: <McpAccessPage /> },
      { path: 'workbench', element: <WorkbenchPage /> },
      { path: 'workbench/task/:taskId', element: <TaskDetailPage /> },
      { path: 'analytics/iteration', element: <IterationDashboardPage /> },
      { path: 'analytics/people', element: <PeopleDashboardPage /> },
      { path: 'analytics/team-health', element: <TeamHealthPage /> },
      { path: 'admin/system-users', element: <SystemUsersPage /> },
      { path: 'admin/system-users/create-success', element: <SystemUsersCreateSuccessPage /> },
      { path: 'admin/audit-logs', element: <AuditLogsPage /> },
    ],
  },
])

type ResolvedTheme = 'light' | 'dark'

const getSystemTheme = (): ResolvedTheme => {
  return window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

const App: React.FC = () => {
  const themeMode = useAppStore((s) => s.themeMode)
  const [systemTheme, setSystemTheme] = useState<ResolvedTheme>(() => getSystemTheme())

  useEffect(() => {
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = () => setSystemTheme(mq.matches ? 'dark' : 'light')
    onChange()

    mq.addEventListener('change', onChange)

    return () => {
      mq.removeEventListener('change', onChange)
    }
  }, [])

  const resolvedTheme: ResolvedTheme = themeMode === 'system' ? systemTheme : themeMode

  useEffect(() => {
    document.documentElement.dataset.theme = resolvedTheme
    document.documentElement.style.colorScheme = resolvedTheme
  }, [resolvedTheme])

  const antdTheme = useMemo(() => {
    const algorithm = resolvedTheme === 'dark' ? theme.darkAlgorithm : theme.defaultAlgorithm
    const baseToken = {
      // Keep UI consistent across themes; brand gradient stays in CSS vars for logo/avatar.
      colorPrimary: '#2563EB',
      borderRadius: 8,
      fontFamily: 'Inter, -apple-system, sans-serif',
    }

    if (resolvedTheme === 'dark') {
      return {
        algorithm,
        token: {
          ...baseToken,
          colorBgLayout: '#0B1220',
          colorBgContainer: '#111827',
          colorBgElevated: '#111827',
          colorBorderSecondary: '#1F2937',
          colorText: '#E5E7EB',
          colorTextSecondary: '#CBD5E1',
          colorTextTertiary: '#94A3B8',
          colorTextQuaternary: '#64748B',
        },
      }
    }

    return {
      algorithm,
      token: {
        ...baseToken,
        colorBgLayout: '#F8FAFC',
        colorBgContainer: '#FFFFFF',
        colorBgElevated: '#FFFFFF',
        colorBorderSecondary: '#E2E8F0',
        colorText: '#0F172A',
        colorTextSecondary: '#334155',
        colorTextTertiary: '#64748B',
        colorTextQuaternary: '#94A3B8',
      },
    }
  }, [resolvedTheme])

  return (
    <ConfigProvider locale={zhCN} theme={antdTheme}>
      <RouterProvider router={router} />
    </ConfigProvider>
  )
}

export default App
