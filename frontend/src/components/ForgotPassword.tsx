import React, { useState } from 'react';
import { Card, Typography, Input, Button, Space, message, Steps, Tooltip } from 'antd';
import { ArrowLeftOutlined, CopyOutlined, SafetyOutlined, GithubOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import './ForgotPassword.css';

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
    <div className="forgot-password">
      {/* GitHub链接 */}
      <Tooltip title="访问GitHub仓库">
        <a
          href="https://github.com/vibe-coding-labs/claude-code-cli-with-openai-api"
          target="_blank"
          rel="noopener noreferrer"
          className="forgot-password__github"
          onMouseEnter={(e) => {
            e.currentTarget.style.color = '#1f1f1f';
            e.currentTarget.style.transform = 'scale(1.1)';
          }}
          onMouseLeave={(e) => {
            e.currentTarget.style.color = '#1f1f1f';
            e.currentTarget.style.transform = 'scale(1)';
          }}
        >
          <GithubOutlined />
        </a>
      </Tooltip>

      <Card className="forgot-password__card">
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
        <div className="forgot-password__header">
          <SafetyOutlined className="forgot-password__icon" />
          <Title level={2} className="forgot-password__title">重置密码</Title>
          <Paragraph type="secondary">
            使用命令行工具快速重置管理员密码
          </Paragraph>
        </div>

        {/* 步骤指示器 */}
        <Steps 
          current={currentStep} 
          className="forgot-password__steps"
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
        <Card className="forgot-password__command">
          <Paragraph strong className="forgot-password__command-title">
            步骤 1：复制以下命令
          </Paragraph>
          <Space.Compact style={{ width: '100%' }}>
            <Input
              value={resetCommand}
              readOnly
              size="large"
              className="forgot-password__command-input"
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
          className="forgot-password__section"
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
                className="forgot-password__path-input"
              />
              <Paragraph style={{ marginBottom: 8 }}>
                3. 粘贴并执行刚才复制的命令
              </Paragraph>
              <Paragraph style={{ marginBottom: 8 }}>
                4. 按照提示输入新密码（密码长度至少6个字符）
              </Paragraph>
            </div>

            <div className="forgot-password__notice">
              <Text strong className="forgot-password__notice-title">⚠️ 注意事项</Text>
              <ul className="forgot-password__notice-list">
                <li className="forgot-password__notice-item">
                  <Text style={{ fontSize: 13 }}>
                    该命令必须在<strong>服务器本地</strong>执行，不能远程执行
                  </Text>
                </li>
                <li className="forgot-password__notice-item">
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
          className="forgot-password__section"
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
        <div className="forgot-password__help">
          <Paragraph className="forgot-password__help-title">
            <Text strong>💡 需要帮助？</Text>
          </Paragraph>
          <Paragraph className="forgot-password__help-text">
            如果遇到问题，请查看 <a href="https://github.com/vibe-coding-labs/claude-code-cli-with-openai-api" target="_blank" rel="noopener noreferrer">GitHub 文档</a> 或提交 Issue
          </Paragraph>
        </div>
      </Card>
    </div>
  );
};

export default ForgotPassword;
