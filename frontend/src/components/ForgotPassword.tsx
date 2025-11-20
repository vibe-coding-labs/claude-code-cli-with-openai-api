import React, { useState } from 'react';
import { Card, Typography, Input, Button, Space, message, Steps, Tooltip } from 'antd';
import { ArrowLeftOutlined, CopyOutlined, SafetyOutlined, GithubOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';

const { Title, Paragraph, Text } = Typography;

const ForgotPassword: React.FC = () => {
  const navigate = useNavigate();
  const [currentStep, setCurrentStep] = useState(0);

  const resetCommand = './claude-code-cli-with-openai-api reset-password';

  const handleCopyCommand = () => {
    navigator.clipboard.writeText(resetCommand).then(() => {
      message.success('命令已复制到剪贴板');
      setCurrentStep(1);
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
      background: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)',
      position: 'relative',
      padding: '20px',
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
            color: '#fff',
            transition: 'all 0.3s',
          }}
          onMouseEnter={(e) => {
            e.currentTarget.style.color = '#f0f0f0';
            e.currentTarget.style.transform = 'scale(1.1)';
          }}
          onMouseLeave={(e) => {
            e.currentTarget.style.color = '#fff';
            e.currentTarget.style.transform = 'scale(1)';
          }}
        >
          <GithubOutlined />
        </a>
      </Tooltip>

      <Card 
        style={{ 
          width: '100%',
          maxWidth: 650,
          boxShadow: '0 10px 40px rgba(0,0,0,0.2)',
          borderRadius: 12,
        }}
      >
        {/* 返回按钮 */}
        <div style={{ marginBottom: 24 }}>
          <Button 
            icon={<ArrowLeftOutlined />} 
            onClick={() => navigate('/ui/login')}
            type="text"
          >
            返回登录
          </Button>
        </div>

        {/* 标题 */}
        <div style={{ textAlign: 'center', marginBottom: 32 }}>
          <SafetyOutlined style={{ fontSize: 48, color: '#667eea', marginBottom: 16 }} />
          <Title level={2} style={{ marginBottom: 8 }}>重置密码</Title>
          <Paragraph type="secondary">
            使用命令行工具快速重置管理员密码
          </Paragraph>
        </div>

        {/* 步骤指示器 */}
        <Steps 
          current={currentStep} 
          style={{ marginBottom: 32 }}
          items={[
            {
              title: '复制命令',
              description: '复制重置命令',
            },
            {
              title: '执行命令',
              description: '在终端中运行',
            },
            {
              title: '完成',
              description: '使用新密码登录',
            },
          ]}
        />

        {/* 命令区域 */}
        <Card 
          style={{ 
            background: '#f6f8fa',
            marginBottom: 24,
            border: '1px solid #e1e4e8',
          }}
        >
          <Paragraph strong style={{ marginBottom: 12, fontSize: 14 }}>
            步骤 1：复制以下命令
          </Paragraph>
          <Space.Compact style={{ width: '100%' }}>
            <Input
              value={resetCommand}
              readOnly
              size="large"
              style={{ 
                fontFamily: 'Monaco, Consolas, "Courier New", monospace',
                fontSize: 14,
                background: '#fff',
                fontWeight: 500,
              }}
            />
            <Button 
              icon={<CopyOutlined />} 
              onClick={handleCopyCommand}
              size="large"
              type="primary"
            >
              复制命令
            </Button>
          </Space.Compact>
        </Card>

        {/* 使用说明 */}
        <Card 
          title={<Text strong>步骤 2：在服务器终端执行命令</Text>}
          style={{ marginBottom: 24 }}
          headStyle={{ background: '#fafafa' }}
        >
          <Space direction="vertical" style={{ width: '100%' }} size="middle">
            <div>
              <Paragraph style={{ marginBottom: 8 }}>
                1. 打开服务器终端（运行此应用的服务器）
              </Paragraph>
              <Paragraph style={{ marginBottom: 8 }}>
                2. 进入应用目录：
              </Paragraph>
              <Input
                value="cd /path/to/claude-code-cli-with-openai-api"
                readOnly
                style={{ 
                  fontFamily: 'monospace',
                  fontSize: 12,
                  marginBottom: 12,
                  background: '#f5f5f5',
                }}
              />
              <Paragraph style={{ marginBottom: 8 }}>
                3. 粘贴并执行刚才复制的命令
              </Paragraph>
              <Paragraph style={{ marginBottom: 8 }}>
                4. 按照提示输入新密码（密码长度至少6个字符）
              </Paragraph>
            </div>

            <div style={{ 
              padding: 12, 
              background: '#fff7e6', 
              borderRadius: 4,
              border: '1px solid #ffd591',
            }}>
              <Text strong style={{ color: '#fa8c16' }}>⚠️ 注意事项</Text>
              <ul style={{ margin: '8px 0 0 0', paddingLeft: 20 }}>
                <li style={{ marginBottom: 4 }}>
                  <Text style={{ fontSize: 13 }}>
                    该命令必须在<strong>服务器本地</strong>执行，不能远程执行
                  </Text>
                </li>
                <li style={{ marginBottom: 4 }}>
                  <Text style={{ fontSize: 13 }}>
                    执行前请确保有足够的权限
                  </Text>
                </li>
                <li>
                  <Text style={{ fontSize: 13 }}>
                    重置后，旧密码将立即失效
                  </Text>
                </li>
              </ul>
            </div>
          </Space>
        </Card>

        {/* 完成提示 */}
        <Card 
          title={<Text strong>步骤 3：使用新密码登录</Text>}
          headStyle={{ background: '#fafafa' }}
        >
          <Space direction="vertical" style={{ width: '100%' }}>
            <Paragraph>
              密码重置成功后，请使用新密码登录系统。
            </Paragraph>
            <Button 
              type="primary" 
              size="large"
              block
              onClick={() => navigate('/ui/login')}
            >
              前往登录页面
            </Button>
          </Space>
        </Card>

        {/* 帮助信息 */}
        <div style={{ 
          marginTop: 24, 
          padding: 16, 
          background: '#e6f7ff',
          borderRadius: 4,
          border: '1px solid #91d5ff',
        }}>
          <Paragraph style={{ marginBottom: 4, fontSize: 13 }}>
            <Text strong>💡 需要帮助？</Text>
          </Paragraph>
          <Paragraph style={{ marginBottom: 0, fontSize: 12 }}>
            如果遇到问题，请查看 <a href="https://github.com/vibe-coding-labs/claude-code-cli-with-openai-api" target="_blank" rel="noopener noreferrer">GitHub 文档</a> 或提交 Issue
          </Paragraph>
        </div>
      </Card>
    </div>
  );
};

export default ForgotPassword;
