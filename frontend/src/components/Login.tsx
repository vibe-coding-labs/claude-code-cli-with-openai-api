import React, { useState } from 'react';
import { Form, Input, Button, Card, Typography, message, Space, Tooltip } from 'antd';
import { UserOutlined, LockOutlined, CopyOutlined, GithubOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { login, setToken, setCurrentUser } from '../services/auth';

const { Title, Paragraph } = Typography;

const Login: React.FC = () => {
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();

  const resetCommand = './claude-code-cli-with-openai-api reset-password';

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

  const handleCopyCommand = () => {
    navigator.clipboard.writeText(resetCommand).then(() => {
      message.success('命令已复制到剪贴板');
    }).catch(() => {
      message.error('复制失败，请手动复制');
    });
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
        </Form>

        <div style={{ marginTop: 16, padding: 12, background: '#f6f8fa', borderRadius: 4 }}>
          <Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 8 }}>
            忘记密码？请使用命令行工具重置：
          </Paragraph>
          <Space.Compact style={{ width: '100%' }}>
            <Input
              value={resetCommand}
              readOnly
              style={{ 
                fontFamily: 'monospace',
                fontSize: 12,
                background: '#fff'
              }}
            />
            <Tooltip title="复制命令">
              <Button 
                icon={<CopyOutlined />} 
                onClick={handleCopyCommand}
              >
                复制
              </Button>
            </Tooltip>
          </Space.Compact>
        </div>
      </Card>
    </div>
  );
};

export default Login;
