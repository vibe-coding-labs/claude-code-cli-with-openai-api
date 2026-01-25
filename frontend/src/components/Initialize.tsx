import React, { useState, useEffect } from 'react';
import { Form, Input, Button, Card, Typography, message, Space, Tooltip } from 'antd';
import { UserOutlined, LockOutlined, CopyOutlined, GithubOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { usePageTitle } from '../utils/pageTitle';
import { initializeSystem, setToken, setCurrentUser } from '../services/auth';

const { Title, Paragraph } = Typography;

const Initialize: React.FC = () => {
  usePageTitle('系统初始化');
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();

  const resetCommand = './claude-code-cli-with-openai-api reset-password';

  const onFinish = async (values: { username: string; password: string; confirmPassword: string }) => {
    if (values.password !== values.confirmPassword) {
      message.error('两次输入的密码不一致');
      return;
    }

    setLoading(true);
    try {
      const response = await initializeSystem(values.username, values.password);
      setToken(response.token);
      setCurrentUser(response.user);
      message.success('系统初始化成功！');
      navigate('/ui');
    } catch (error: any) {
      message.error(error.response?.data?.error || '初始化失败，请重试');
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

      <Card style={{ width: 500, boxShadow: '0 4px 12px rgba(0,0,0,0.15)' }}>
        <div style={{ textAlign: 'center', marginBottom: 24 }}>
          <Title level={3}>欢迎使用 Use ClaudeCode CLI With OpenAI API</Title>
          <Paragraph type="secondary">
            系统首次运行需要初始化，请设置管理员账户
          </Paragraph>
        </div>

        <Form
          name="initialize"
          onFinish={onFinish}
          autoComplete="off"
          size="large"
          layout="vertical"
        >
          <Form.Item
            label="用户名"
            name="username"
            rules={[
              { required: true, message: '请输入用户名' },
              { min: 3, message: '用户名至少3个字符' },
              { max: 50, message: '用户名最多50个字符' },
            ]}
          >
            <Input
              prefix={<UserOutlined />}
              placeholder="请输入用户名（3-50个字符）"
            />
          </Form.Item>

          <Form.Item
            label="密码"
            name="password"
            rules={[
              { required: true, message: '请输入密码' },
              { min: 6, message: '密码至少6个字符' },
            ]}
          >
            <Input.Password
              prefix={<LockOutlined />}
              placeholder="请输入密码（至少6个字符）"
              autoComplete="new-password"
            />
          </Form.Item>

          <Form.Item
            label="确认密码"
            name="confirmPassword"
            rules={[
              { required: true, message: '请再次输入密码' },
            ]}
          >
            <Input.Password
              prefix={<LockOutlined />}
              placeholder="请再次输入密码"
              autoComplete="new-password"
            />
          </Form.Item>

          <Form.Item>
            <Button type="primary" htmlType="submit" loading={loading} block>
              创建管理员账户
            </Button>
          </Form.Item>
        </Form>

        <div style={{ marginTop: 16, padding: 16, background: '#f6f8fa', borderRadius: 4 }}>
          <Paragraph style={{ marginBottom: 12, fontSize: 13 }}>
            <strong>提示：</strong>
          </Paragraph>
          <ul style={{ margin: 0, paddingLeft: 20, fontSize: 12 }}>
            <li style={{ marginBottom: 8 }}>请妥善保管您的账户信息</li>
            <li>
              <div style={{ marginBottom: 4 }}>如忘记密码，可使用命令行工具重置：</div>
              <Space.Compact style={{ width: '100%' }}>
                <Input
                  value={resetCommand}
                  readOnly
                  size="small"
                  style={{ 
                    fontFamily: 'monospace',
                    fontSize: 11,
                    background: '#fff'
                  }}
                />
                <Tooltip title="复制命令">
                  <Button 
                    size="small"
                    icon={<CopyOutlined />} 
                    onClick={handleCopyCommand}
                  >
                    复制
                  </Button>
                </Tooltip>
              </Space.Compact>
            </li>
          </ul>
        </div>
      </Card>
    </div>
  );
};

export default Initialize;
