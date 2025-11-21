import React, { useState } from 'react';
import { Form, Input, Button, Card, Typography, message, Tooltip } from 'antd';
import { UserOutlined, LockOutlined, GithubOutlined } from '@ant-design/icons';
import { useNavigate, Link } from 'react-router-dom';
import { login, setToken, setCurrentUser } from '../services/auth';
import { usePageTitle } from '../utils/pageTitle';

const { Title, Paragraph } = Typography;

const Login: React.FC = () => {
  usePageTitle('登录');
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();

  const onFinish = async (values: { username: string; password: string }) => {
    setLoading(true);
    try {
      const response = await login(values.username, values.password);
      setToken(response.token);
      setCurrentUser(response.user);
      message.success('登录成功！');
      navigate('/ui');
    } catch (error: any) {
      message.error(error.response?.data?.error || '登录失败，请检查用户名和密码');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div style={{
      display: 'flex',
      justifyContent: 'center',
      alignItems: 'center',
      minHeight: '100vh',
      background: '#f0f2f5',
      position: 'relative',
    }}>
      {/* GitHub链接 */}
      <Tooltip title="访问GitHub仓库">
        <a
          href="https://github.com/vibe-coding-labs/claude-code-cli-with-openai-api"
          target="_blank"
          rel="noopener noreferrer"
          style={{
            position: 'fixed',
            top: 24,
            right: 24,
            fontSize: 32,
            color: '#24292e',
            transition: 'all 0.3s',
          }}
          onMouseEnter={(e) => {
            e.currentTarget.style.color = '#0969da';
            e.currentTarget.style.transform = 'scale(1.1)';
          }}
          onMouseLeave={(e) => {
            e.currentTarget.style.color = '#24292e';
            e.currentTarget.style.transform = 'scale(1)';
          }}
        >
          <GithubOutlined />
        </a>
      </Tooltip>

      <Card style={{ width: 400, boxShadow: '0 4px 12px rgba(0,0,0,0.15)' }}>
        <div style={{ textAlign: 'center', marginBottom: 24 }}>
          <Title level={3}>Use ClaudeCode CLI With OpenAI API</Title>
          <Paragraph type="secondary">请登录以继续</Paragraph>
        </div>

        <Form
          name="login"
          onFinish={onFinish}
          autoComplete="off"
          size="large"
        >
          <Form.Item
            name="username"
            rules={[{ required: true, message: '请输入用户名' }]}
          >
            <Input
              prefix={<UserOutlined />}
              placeholder="用户名"
            />
          </Form.Item>

          <Form.Item
            name="password"
            rules={[{ required: true, message: '请输入密码' }]}
          >
            <Input.Password
              prefix={<LockOutlined />}
              placeholder="密码"
            />
          </Form.Item>

          <Form.Item>
            <Button type="primary" htmlType="submit" loading={loading} block>
              登录
            </Button>
          </Form.Item>

          <Form.Item style={{ marginBottom: 0 }}>
            <div style={{ textAlign: 'center' }}>
              <Link 
                to="/ui/forgot-password" 
                style={{ 
                  color: '#1890ff',
                  fontSize: 14,
                }}
              >
                忘记密码？
              </Link>
            </div>
          </Form.Item>
        </Form>
      </Card>
    </div>
  );
};

export default Login;
