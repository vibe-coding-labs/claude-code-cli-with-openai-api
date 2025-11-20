import React from 'react';
import { BrowserRouter as Router, Routes, Route, Link, useLocation } from 'react-router-dom';
import { Layout, Menu, Typography, Button } from 'antd';
import {
  SettingOutlined,
  LogoutOutlined,
} from '@ant-design/icons';
import ConfigList from './components/ConfigListV2';
import ConfigDetailV2 from './components/ConfigDetailV2';
import ConfigEdit from './components/ConfigEdit';
import ConfigCreate from './components/ConfigCreate';
import ConfigTestPage from './components/ConfigTestPage';
import Login from './components/Login';
import Initialize from './components/Initialize';
import ProtectedRoute from './components/ProtectedRoute';
import { logout, getCurrentUser } from './services/auth';
import './App.css';

const { Header, Content, Sider } = Layout;
const { Title } = Typography;

const AppLayout: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const location = useLocation();
  const currentUser = getCurrentUser();
  
  const normalizedPath = location.pathname === '/ui' || location.pathname === '/ui/' ? '/ui' : location.pathname;

  const menuItems = [
    {
      key: '/ui',
      icon: <SettingOutlined />,
      label: <Link to="/ui">OpenAI API配置</Link>,
    },
  ];

  const handleLogout = () => {
    logout();
    window.location.href = '/ui/login';
  };

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Header style={{ background: '#001529', padding: '0 24px', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Title level={4} style={{ color: '#fff', margin: '16px 0' }}>
          Use ClaudeCode CLI With OpenAI API
        </Title>
        {currentUser && (
          <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
            <span style={{ color: '#fff' }}>欢迎, {currentUser.username}</span>
            <Button 
              type="text" 
              icon={<LogoutOutlined />} 
              onClick={handleLogout}
              style={{ color: '#fff' }}
            >
              退出登录
            </Button>
          </div>
        )}
      </Header>
      <Layout>
        <Sider width={200} style={{ background: '#fff' }}>
          <Menu
            mode="inline"
            selectedKeys={[normalizedPath]}
            style={{ height: '100%', borderRight: 0 }}
            items={menuItems}
          />
        </Sider>
        <Layout style={{ padding: '24px' }}>
          <Content
            style={{
              background: '#fff',
              padding: 24,
              margin: 0,
              minHeight: 280,
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
        <Route path="/ui/login" element={<Login />} />
        <Route path="/ui/initialize" element={<Initialize />} />
        <Route path="/ui/*" element={
          <AppLayout>
            <Routes>
              <Route path="/" element={<ProtectedRoute><ConfigList /></ProtectedRoute>} />
              <Route path="configs/create" element={<ProtectedRoute><ConfigCreate /></ProtectedRoute>} />
              <Route path="configs/:id" element={<ProtectedRoute><ConfigDetailV2 /></ProtectedRoute>} />
              <Route path="configs/:id/edit" element={<ProtectedRoute><ConfigEdit /></ProtectedRoute>} />
              <Route path="configs/:id/test" element={<ProtectedRoute><ConfigTestPage /></ProtectedRoute>} />
            </Routes>
          </AppLayout>
        } />
      </Routes>
    </Router>
  );
};

export default App;
