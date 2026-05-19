import React from 'react';
import { BrowserRouter as Router, Routes, Route, Link, useLocation, Navigate } from 'react-router-dom';
import { Layout, Menu, Typography, Button } from 'antd';
import {
  SettingOutlined,
  LogoutOutlined,
  ClusterOutlined,
} from '@ant-design/icons';
import ConfigList from './components/ConfigListV2';
import ConfigDetailV2 from './components/ConfigDetailV2';
import ConfigEdit from './components/ConfigEdit';
import ConfigCreate from './components/ConfigCreate';
import ConfigTestPage from './components/ConfigTestPage';
import LogDetail from './components/LogDetail';
import LoadBalancerList from './components/LoadBalancerList';
import LoadBalancerEditor from './components/LoadBalancerEditor';
import LoadBalancerDetail from './components/LoadBalancerDetail';
import UserList from './components/UserList';
import UserUsage from './components/UserUsage';
import TenantList from './components/TenantList';
import TenantCreate from './components/TenantCreate';
import TenantDetail from './components/TenantDetail';
import AuditLogViewer from './components/AuditLogViewer';
import SystemSettings from './components/SystemSettings';
import Login from './components/Login';
import ForgotPassword from './components/ForgotPassword';
import Initialize from './components/Initialize';
import ProtectedRoute from './components/ProtectedRoute';
import { logout, getCurrentUser } from './services/auth';
import './App.css';

const { Header, Content, Sider } = Layout;
const { Title } = Typography;

const AppLayout: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const location = useLocation();
  const currentUser = getCurrentUser();
  
  // Normalize: /ui, /ui/, and /ui/configs all map to the config list menu item
  const normalizedPath = (() => {
    const p = location.pathname;
    if (p === '/ui' || p === '/ui/') return '/ui/configs';
    return p;
  })();

  const menuItems = [
    {
      key: '/ui/configs',
      icon: <SettingOutlined />,
      label: <Link to="/ui/configs">OpenAI API配置</Link>,
    },
    {
      key: '/ui/load-balancers',
      icon: <ClusterOutlined />,
      label: <Link to="/ui/load-balancers">负载均衡器</Link>,
    },
    ...(currentUser?.role === 'admin'
      ? [
          {
            key: '/ui/users',
            icon: <SettingOutlined />,
            label: <Link to="/ui/users">用户管理</Link>,
          },
          {
            key: '/ui/tenants',
            icon: <ClusterOutlined />,
            label: <Link to="/ui/tenants">租户管理</Link>,
          },
          {
            key: '/ui/audit-logs',
            icon: <SettingOutlined />,
            label: <Link to="/ui/audit-logs">审计日志</Link>,
          },
          {
            key: '/ui/settings',
            icon: <SettingOutlined />,
            label: <Link to="/ui/settings">系统设置</Link>,
          },
        ]
      : []),
  ];

  const handleLogout = () => {
    logout();
    window.location.href = '/ui/login';
  };

  return (
    <Layout style={{ minHeight: '100vh', background: 'var(--color-bg)' }}>
      <Header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Title level={4} style={{ margin: 0 }}>
          Use ClaudeCode CLI With OpenAI API
        </Title>
        {currentUser && (
          <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
            <span style={{ color: 'var(--color-header-text)', fontSize: 13 }}>欢迎, {currentUser.username}</span>
            <Button
              type="text"
              icon={<LogoutOutlined />}
              onClick={handleLogout}
              style={{ color: 'var(--color-header-text)', fontSize: 13 }}
            >
              退出登录
            </Button>
          </div>
        )}
      </Header>
      <Layout>
        <Sider width={220}>
          <Menu
            mode="inline"
            selectedKeys={[normalizedPath]}
            style={{ height: '100%' }}
            items={menuItems}
          />
        </Sider>
        <Layout style={{ padding: '28px', background: 'var(--color-bg)' }}>
          <Content
            style={{
              padding: 32,
              minHeight: 280,
              width: '100%',
            }}
          >
            {children}
          </Content>
        </Layout>
      </Layout>
    </Layout>
  );
};

const App: React.FC = () => {
  return (
    <Router>
      <Routes>
        <Route path="/" element={<Navigate to="/ui/configs" replace />} />
        <Route path="/ui" element={<Navigate to="/ui/configs" replace />} />
        <Route path="/ui/login" element={<Login />} />
        <Route path="/ui/forgot-password" element={<ForgotPassword />} />
        <Route path="/ui/initialize" element={<Initialize />} />
        <Route path="/ui/*" element={
          <AppLayout>
            <Routes>
              <Route path="/" element={<Navigate to="/ui/configs" replace />} />
              <Route path="configs" element={<ProtectedRoute><ConfigList /></ProtectedRoute>} />
              <Route path="configs/create" element={<ProtectedRoute><ConfigCreate /></ProtectedRoute>} />
              <Route path="configs/:id" element={<ProtectedRoute><ConfigDetailV2 /></ProtectedRoute>} />
              <Route path="configs/:id/edit" element={<ProtectedRoute><ConfigEdit /></ProtectedRoute>} />
              <Route path="configs/:id/test" element={<ProtectedRoute><ConfigTestPage /></ProtectedRoute>} />
              <Route path="configs/:id/logs/:log_id" element={<ProtectedRoute><LogDetail /></ProtectedRoute>} />
              <Route path="load-balancers" element={<ProtectedRoute><LoadBalancerList /></ProtectedRoute>} />
              <Route path="load-balancers/create" element={<ProtectedRoute><LoadBalancerEditor /></ProtectedRoute>} />
              <Route path="load-balancers/:id" element={<ProtectedRoute><LoadBalancerDetail /></ProtectedRoute>} />
              <Route path="load-balancers/:id/edit" element={<ProtectedRoute><LoadBalancerEditor /></ProtectedRoute>} />
              <Route path="users" element={<ProtectedRoute><UserList /></ProtectedRoute>} />
              <Route path="users/:id/usage" element={<ProtectedRoute><UserUsage /></ProtectedRoute>} />
              <Route path="tenants" element={<ProtectedRoute><TenantList /></ProtectedRoute>} />
              <Route path="tenants/create" element={<ProtectedRoute><TenantCreate /></ProtectedRoute>} />
              <Route path="tenants/:id" element={<ProtectedRoute><TenantDetail /></ProtectedRoute>} />
              <Route path="audit-logs" element={<ProtectedRoute><AuditLogViewer /></ProtectedRoute>} />
              <Route path="settings" element={<ProtectedRoute><SystemSettings /></ProtectedRoute>} />
            </Routes>
          </AppLayout>
        } />
        <Route path="*" element={<Navigate to="/ui/configs" replace />} />
      </Routes>
    </Router>
  );
};

export default App;
