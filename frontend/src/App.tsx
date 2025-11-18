import React from 'react';
import { BrowserRouter as Router, Routes, Route, Link, useLocation } from 'react-router-dom';
import { Layout, Menu, Typography } from 'antd';
import {
  SettingOutlined,
  FileTextOutlined,
} from '@ant-design/icons';
import ConfigList from './components/ConfigList';
import APIDocs from './components/APIDocs';
import './App.css';

const { Header, Content, Sider } = Layout;
const { Title } = Typography;

const AppLayout: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const location = useLocation();
  
  // Normalize pathname for menu selection
  const normalizedPath = location.pathname === '/ui' || location.pathname === '/ui/' ? '/ui' : location.pathname;

  const menuItems = [
    {
      key: '/ui',
      icon: <SettingOutlined />,
      label: <Link to="/ui">配置管理</Link>,
    },
    {
      key: '/ui/docs',
      icon: <FileTextOutlined />,
      label: <Link to="/ui/docs">API文档</Link>,
    },
  ];

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Header style={{ background: '#001529', padding: '0 24px' }}>
        <Title level={4} style={{ color: '#fff', margin: '16px 0' }}>
          Claude-to-OpenAI API Proxy
        </Title>
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
      <AppLayout>
        <Routes>
          <Route path="/ui" element={<ConfigList />} />
          <Route path="/ui/" element={<ConfigList />} />
          <Route path="/ui/docs" element={<APIDocs />} />
        </Routes>
      </AppLayout>
    </Router>
  );
};

export default App;
