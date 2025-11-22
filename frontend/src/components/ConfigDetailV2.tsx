import React, { useState, useEffect } from 'react';
import { useParams, useNavigate, useSearchParams } from 'react-router-dom';
import { usePageTitle } from '../utils/pageTitle';
import {
  Card,
  Descriptions,
  Button,
  Space,
  message,
  Modal,
  Tag,
  Spin,
  Tabs,
  Typography,
  Input,
  Tooltip,
  Form,
} from 'antd';
import {
  ArrowLeftOutlined,
  EditOutlined,
  DeleteOutlined,
  ReloadOutlined,
  CopyOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import axios from 'axios';
import RequestLogs from './RequestLogs';
import ConfigTestInline from './ConfigTestInline';

const { Paragraph } = Typography;

interface Config {
  id: string;
  name: string;
  description: string;
  openai_api_key_masked: string;
  openai_base_url: string;
  big_model: string;
  middle_model: string;
  small_model: string;
  anthropic_api_key?: string;
  max_tokens_limit: number;
  request_timeout: number;
  retry_count: number;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

const ConfigDetailV2: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const activeTab = searchParams.get('tab') || 'overview';
  
  const [config, setConfig] = useState<Config | null>(null);
  const [loading, setLoading] = useState(true);
  
  usePageTitle(config ? `${config.name} - 配置详情` : '配置详情');
  const [renewingKey, setRenewingKey] = useState(false);
  const [promptText, setPromptText] = useState('Hello, Claude!');

  // Get server info from window.location
  const protocol = window.location.protocol; // 'http:' or 'https:'
  const serverHost = window.location.hostname || 'localhost';
  const serverPort = window.location.port;
  
  // 构建URL：只有非标准端口才显示端口号（http:80, https:443不显示）
  let serverUrl = `${protocol}//${serverHost}`;
  if (serverPort && 
      !((protocol === 'http:' && serverPort === '80') || 
        (protocol === 'https:' && serverPort === '443'))) {
    serverUrl += `:${serverPort}`;
  }

  useEffect(() => {
    fetchConfigDetail();
  }, [id]);

  const fetchConfigDetail = async () => {
    try {
      const response = await axios.get(`/api/configs/${id}`);
      setConfig(response.data.config);
      setLoading(false);
    } catch (error) {
      message.error('获取配置详情失败');
      setLoading(false);
    }
  };

  // 自动生成UUID Token
  const handleRenewKey = () => {
    Modal.confirm({
      title: '更新 API Token',
      content: (
        <div>
          <p style={{ marginBottom: 12, color: '#ff4d4f', fontWeight: 500 }}>
            ⚠️ 确定要自动生成新的 Anthropic API Token 吗？
          </p>
          <p style={{ marginBottom: 0, fontSize: 13, color: '#666' }}>
            系统将自动生成一个UUID作为新Token，旧的 Token 将<strong>立即失效</strong>。
          </p>
        </div>
      ),
      okText: '确认生成',
      cancelText: '取消',
      okType: 'primary',
      okButtonProps: { danger: true },
      width: 500,
      onOk: async () => {
        setRenewingKey(true);
        try {
          const response = await axios.post(`/api/configs/${id}/renew-key`, {
            custom_token: undefined,
          });
          Modal.success({
            title: 'API Token 已更新',
            content: (
              <div>
                <p style={{ marginBottom: 12 }}>新的 Anthropic API Token：</p>
                <Input.TextArea 
                  value={response.data.new_api_key} 
                  readOnly 
                  autoSize
                  style={{ fontFamily: 'monospace', fontSize: 13 }}
                />
                <Button 
                  type="link"
                  icon={<CopyOutlined />}
                  onClick={() => {
                    navigator.clipboard.writeText(response.data.new_api_key);
                    message.success('已复制到剪贴板');
                  }}
                  style={{ marginTop: 8 }}
                >
                  复制 Token
                </Button>
                <p style={{ marginTop: 16, color: '#ff4d4f', fontSize: 13 }}>
                  ⚠️ 请立即保存此 Token，关闭后将无法再次查看！
                </p>
              </div>
            ),
            width: 650,
          });
          fetchConfigDetail();
        } catch (error: any) {
          message.error(error.response?.data?.error || '更新失败');
        } finally {
          setRenewingKey(false);
        }
      },
    });
  };

  // 自定义Token
  const handleCustomToken = () => {
    let customToken = '';
    let inputValid = false;
    
    const validateToken = (value: string): { valid: boolean; message?: string } => {
      if (!value) {
        return { valid: false, message: 'Token不能为空' };
      }
      if (value.length < 1 || value.length > 100) {
        return { valid: false, message: 'Token长度必须在1-100个字符之间' };
      }
      // 只允许英文大小写字母、数字、下划线、连字符
      if (!/^[a-zA-Z0-9_-]+$/.test(value)) {
        return { valid: false, message: 'Token只能包含英文字母、数字、下划线(_)、连字符(-)' };
      }
      return { valid: true };
    };

    const modal = Modal.confirm({
      title: '自定义 Anthropic API Token',
      content: (
        <div>
          <p style={{ marginBottom: 12, color: '#ff4d4f', fontWeight: 500 }}>
            ⚠️ 设置自定义Token后，旧的 Token 将<strong>立即失效</strong>。
          </p>
          <p style={{ marginBottom: 8, fontSize: 13, color: '#666' }}>
            请输入自定义Token：
          </p>
          <Input 
            id="custom-token-input"
            placeholder="英文字母、数字、下划线、连字符，长度1-100"
            onChange={(e) => {
              customToken = e.target.value;
              const validation = validateToken(customToken);
              inputValid = validation.valid;
              
              // 实时显示验证结果
              const errorDiv = document.getElementById('token-error-message');
              if (errorDiv) {
                if (validation.valid) {
                  errorDiv.style.display = 'none';
                } else {
                  errorDiv.style.display = 'block';
                  errorDiv.textContent = validation.message || '';
                }
              }
            }}
            maxLength={100}
            style={{ marginBottom: 8 }}
          />
          <div 
            id="token-error-message"
            style={{ 
              display: 'none',
              color: '#ff4d4f', 
              fontSize: 12,
              marginTop: 4
            }}
          />
          <p style={{ marginTop: 12, fontSize: 12, color: '#999' }}>
            格式要求：
            <br />• 只能包含英文大小写字母(a-z, A-Z)
            <br />• 数字(0-9)
            <br />• 下划线(_)和连字符(-)
            <br />• 长度：1-100个字符
          </p>
        </div>
      ),
      okText: '确认设置',
      cancelText: '取消',
      okType: 'primary',
      okButtonProps: { danger: true },
      width: 550,
      onOk: async () => {
        const validation = validateToken(customToken);
        if (!validation.valid) {
          message.error(validation.message || '请输入有效的Token');
          return Promise.reject();
        }

        setRenewingKey(true);
        try {
          const response = await axios.post(`/api/configs/${id}/renew-key`, {
            custom_token: customToken,
          });
          Modal.success({
            title: '自定义 Token 已设置',
            content: (
              <div>
                <p style={{ marginBottom: 12 }}>新的 Anthropic API Token：</p>
                <Input.TextArea 
                  value={response.data.new_api_key} 
                  readOnly 
                  autoSize
                  style={{ fontFamily: 'monospace', fontSize: 13 }}
                />
                <Button 
                  type="link"
                  icon={<CopyOutlined />}
                  onClick={() => {
                    navigator.clipboard.writeText(response.data.new_api_key);
                    message.success('已复制到剪贴板');
                  }}
                  style={{ marginTop: 8 }}
                >
                  复制 Token
                </Button>
                <p style={{ marginTop: 16, color: '#ff4d4f', fontSize: 13 }}>
                  ⚠️ 请立即保存此 Token，关闭后将无法再次查看！
                </p>
              </div>
            ),
            width: 650,
          });
          fetchConfigDetail();
        } catch (error: any) {
          message.error(error.response?.data?.error || '设置失败');
          return Promise.reject();
        } finally {
          setRenewingKey(false);
        }
      },
    });
  };

  const handleDelete = () => {
    Modal.confirm({
      title: '确认删除',
      content: `确定要删除配置 "${config?.name}" 吗？此操作不可恢复。`,
      okText: '删除',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        try {
          await axios.delete(`/api/configs/${id}`);
          message.success('删除成功');
          navigate('/ui');
        } catch (error) {
          message.error('删除失败');
        }
      },
    });
  };

  const handleTabChange = (key: string) => {
    setSearchParams({ tab: key });
  };

  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: 50 }}>
        <Spin size="large" />
      </div>
    );
  }

  if (!config) {
    return <div>配置未找到</div>;
  }

  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/ui')}>
          返回列表
        </Button>
        <Button
          type="primary"
          icon={<EditOutlined />}
          onClick={() => navigate(`/ui/configs/${id}/edit`)}
        >
          编辑配置
        </Button>
        <Button icon={<ReloadOutlined />} onClick={fetchConfigDetail}>
          刷新
        </Button>
        <Button danger icon={<DeleteOutlined />} onClick={handleDelete}>
          删除
        </Button>
      </Space>

      {/* 配置名称标题 */}
      <div style={{ 
        marginBottom: 20, 
        padding: '16px 24px',
        background: 'linear-gradient(135deg, #1890ff 0%, #096dd9 100%)',
        borderRadius: 8,
        boxShadow: '0 4px 12px rgba(24, 144, 255, 0.15)'
      }}>
        <div style={{ 
          fontSize: 24, 
          fontWeight: 600, 
          color: '#fff',
          display: 'flex',
          alignItems: 'center',
          gap: 12
        }}>
          <span style={{ 
            fontSize: 20,
            opacity: 0.9
          }}>📋</span>
          {config.name}
          <Tag 
            color={config.enabled ? 'success' : 'default'} 
            style={{ 
              marginLeft: 8,
              fontSize: 12,
              padding: '2px 8px'
            }}
          >
            {config.enabled ? '启用' : '禁用'}
          </Tag>
        </div>
        {config.description && (
          <div style={{ 
            marginTop: 8, 
            fontSize: 14, 
            color: 'rgba(255, 255, 255, 0.85)',
            fontWeight: 400
          }}>
            {config.description}
          </div>
        )}
      </div>

      <Tabs activeKey={activeTab} onChange={handleTabChange}>
        {/* Overview Tab */}
        <Tabs.TabPane tab="详情" key="overview">
          <Card title="基本信息" style={{ marginBottom: 16 }}>
            <Descriptions column={2} bordered>
              <Descriptions.Item label="配置ID">{config.id}</Descriptions.Item>
              <Descriptions.Item label="状态">
                <Tag color={config.enabled ? 'success' : 'default'}>
                  {config.enabled ? '启用' : '禁用'}
                </Tag>
              </Descriptions.Item>
              <Descriptions.Item label="名称" span={2}>{config.name}</Descriptions.Item>
              <Descriptions.Item label="描述" span={2}>
                {config.description || '-'}
              </Descriptions.Item>
              <Descriptions.Item label="创建时间">
                {new Date(config.created_at).toLocaleString('zh-CN')}
              </Descriptions.Item>
              <Descriptions.Item label="更新时间">
                {new Date(config.updated_at).toLocaleString('zh-CN')}
              </Descriptions.Item>
            </Descriptions>
          </Card>

          <Card title="OpenAI 配置" style={{ marginBottom: 16 }}>
            <Descriptions column={2} bordered>
              <Descriptions.Item label="OpenAI API Key" span={2}>
                {config.openai_api_key_masked}
              </Descriptions.Item>
              <Descriptions.Item label="Base URL" span={2}>
                {config.openai_base_url}
              </Descriptions.Item>
              <Descriptions.Item label="大模型 (Opus)">{config.big_model}</Descriptions.Item>
              <Descriptions.Item label="中模型 (Sonnet)">{config.middle_model}</Descriptions.Item>
              <Descriptions.Item label="小模型 (Haiku)">{config.small_model}</Descriptions.Item>
              <Descriptions.Item label="最大Token限制">{config.max_tokens_limit}</Descriptions.Item>
              <Descriptions.Item label="请求超时(秒)">{config.request_timeout}</Descriptions.Item>
              <Descriptions.Item label="失败重试次数">{config.retry_count || 3}</Descriptions.Item>
            </Descriptions>
          </Card>

          <Card
            title="Anthropic API Token"
            extra={
              <Space size="small">
                <Tooltip title="自动生成UUID作为Token">
                  <Button
                    type="link"
                    icon={<SyncOutlined spin={renewingKey} />}
                    onClick={handleRenewKey}
                    loading={renewingKey}
                    size="small"
                  >
                    更新 Token
                  </Button>
                </Tooltip>
                <Tooltip title="自定义Token内容">
                  <Button
                    type="link"
                    icon={<EditOutlined />}
                    onClick={handleCustomToken}
                    loading={renewingKey}
                    size="small"
                    style={{ color: '#1890ff' }}
                  >
                    自定义 Token
                  </Button>
                </Tooltip>
              </Space>
            }
            style={{ marginBottom: 16 }}
          >
            <div style={{ marginBottom: 12 }}>
              <Paragraph copyable={{ text: config.anthropic_api_key || config.id }}>
                <code style={{
                  background: '#f5f5f5',
                  padding: '4px 8px',
                  borderRadius: 4,
                  fontSize: 13,
                  fontFamily: 'monospace',
                }}>
                  {config.anthropic_api_key || config.id}
                </code>
              </Paragraph>
            </div>
            <Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 0 }}>
              使用此 Token 作为 Anthropic API Key 来调用代理服务
            </Paragraph>
          </Card>

          <Card title="Claude Code CLI 配置" style={{ marginBottom: 16 }}>
            <Space direction="vertical" style={{ width: '100%' }} size="middle">
              {/* 方式一：单次执行（推荐） */}
              <div>
                <Typography.Text strong style={{ fontSize: 13, display: 'block', marginBottom: 8 }}>
                  📌 单次执行（推荐）- 直接复制执行：
                </Typography.Text>
                <Input.TextArea
                  value={`ANTHROPIC_BASE_URL=${serverUrl} ANTHROPIC_API_KEY="${config.anthropic_api_key || config.id}" CLAUDE_CODE_MAX_OUTPUT_TOKENS=${config.max_tokens_limit} claude --dangerously-skip-permissions`}
                  readOnly
                  autoSize={{ minRows: 1, maxRows: 3 }}
                  style={{
                    fontFamily: 'Monaco, Consolas, "Courier New", monospace',
                    fontSize: 12,
                    background: '#f5f5f5',
                  }}
                />
                <Button
                  type="primary"
                  size="small"
                  icon={<CopyOutlined />}
                  onClick={() => {
                    navigator.clipboard.writeText(
                      `ANTHROPIC_BASE_URL=${serverUrl} ANTHROPIC_API_KEY="${config.anthropic_api_key || config.id}" CLAUDE_CODE_MAX_OUTPUT_TOKENS=${config.max_tokens_limit} claude --dangerously-skip-permissions`
                    );
                    message.success('已复制到剪贴板，可直接粘贴执行');
                  }}
                  style={{ marginTop: 8 }}
                >
                  复制命令
                </Button>
              </div>

              {/* 方式二：添加到 shell 配置 */}
              <div>
                <Typography.Text strong style={{ fontSize: 13, display: 'block', marginBottom: 8 }}>
                  🔧 永久配置（添加到 ~/.zshrc 或 ~/.bashrc）：
                </Typography.Text>
                <Input.TextArea
                  value={`export ANTHROPIC_BASE_URL=${serverUrl}
export ANTHROPIC_API_KEY="${config.anthropic_api_key || config.id}"
export CLAUDE_CODE_MAX_OUTPUT_TOKENS=${config.max_tokens_limit}
alias claude='command claude --dangerously-skip-permissions'`}
                  readOnly
                  autoSize={{ minRows: 4, maxRows: 4 }}
                  style={{
                    fontFamily: 'Monaco, Consolas, "Courier New", monospace',
                    fontSize: 12,
                    background: '#f5f5f5',
                  }}
                />
                <Button
                  size="small"
                  icon={<CopyOutlined />}
                  onClick={() => {
                    navigator.clipboard.writeText(
                      `export ANTHROPIC_BASE_URL=${serverUrl}\nexport ANTHROPIC_API_KEY="${config.anthropic_api_key || config.id}"\nexport CLAUDE_CODE_MAX_OUTPUT_TOKENS=${config.max_tokens_limit}\nalias claude='command claude --dangerously-skip-permissions'`
                    );
                    message.success('已复制，粘贴到 shell 配置文件后执行 source ~/.zshrc 生效');
                  }}
                  style={{ marginTop: 8 }}
                >
                  复制配置
                </Button>
              </div>

              {/* 方式三：一键配置脚本 */}
              <div>
                <Typography.Text strong style={{ fontSize: 13, display: 'block', marginBottom: 8 }}>
                  ⚡ 一键配置脚本（自动追加到 shell 配置）：
                </Typography.Text>
                <Input.TextArea
                  value={`echo 'export ANTHROPIC_BASE_URL=${serverUrl}' >> ~/.zshrc && echo 'export ANTHROPIC_API_KEY="${config.anthropic_api_key || config.id}"' >> ~/.zshrc && echo 'export CLAUDE_CODE_MAX_OUTPUT_TOKENS=${config.max_tokens_limit}' >> ~/.zshrc && echo "alias claude='command claude --dangerously-skip-permissions'" >> ~/.zshrc && source ~/.zshrc`}
                  readOnly
                  autoSize={{ minRows: 1, maxRows: 3 }}
                  style={{
                    fontFamily: 'Monaco, Consolas, "Courier New", monospace',
                    fontSize: 12,
                    background: '#f5f5f5',
                  }}
                />
                <Button
                  size="small"
                  icon={<CopyOutlined />}
                  onClick={() => {
                    navigator.clipboard.writeText(
                      `echo 'export ANTHROPIC_BASE_URL=${serverUrl}' >> ~/.zshrc && echo 'export ANTHROPIC_API_KEY="${config.anthropic_api_key || config.id}"' >> ~/.zshrc && echo 'export CLAUDE_CODE_MAX_OUTPUT_TOKENS=${config.max_tokens_limit}' >> ~/.zshrc && echo "alias claude='command claude --dangerously-skip-permissions'" >> ~/.zshrc && source ~/.zshrc`
                    );
                    message.success('已复制一键配置脚本');
                  }}
                  style={{ marginTop: 8 }}
                >
                  复制脚本
                </Button>
                <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginTop: 4 }}>
                  （使用 bash 的话，将 ~/.zshrc 改为 ~/.bashrc）
                </Typography.Text>
              </div>

              {/* 方式四：带提示词的快速执行 */}
              <div>
                <Typography.Text strong style={{ fontSize: 13, display: 'block', marginBottom: 8 }}>
                  💬 带提示词快速执行（使用 -p 参数）：
                </Typography.Text>
                <Typography.Text style={{ fontSize: 12, display: 'block', marginBottom: 8, color: '#666' }}>
                  输入提示词，生成可直接执行的命令：
                </Typography.Text>
                <Input
                  placeholder="请输入提示词，例如：Hello, Claude!"
                  value={promptText}
                  onChange={(e) => setPromptText(e.target.value)}
                  style={{ marginBottom: 8 }}
                  suffix={
                    promptText && (
                      <Button
                        type="text"
                        size="small"
                        onClick={() => setPromptText('')}
                        style={{ padding: 0, height: 'auto' }}
                      >
                        清空
                      </Button>
                    )
                  }
                />
                <Input.TextArea
                  value={`ANTHROPIC_BASE_URL=${serverUrl} ANTHROPIC_API_KEY="${config.anthropic_api_key || config.id}" CLAUDE_CODE_MAX_OUTPUT_TOKENS=${config.max_tokens_limit} claude --dangerously-skip-permissions -p "${promptText}"`}
                  readOnly
                  autoSize={{ minRows: 2, maxRows: 4 }}
                  style={{
                    fontFamily: 'Monaco, Consolas, "Courier New", monospace',
                    fontSize: 12,
                    background: '#f5f5f5',
                  }}
                />
                <Button
                  type="primary"
                  size="small"
                  icon={<CopyOutlined />}
                  onClick={() => {
                    if (!promptText.trim()) {
                      message.warning('请先输入提示词');
                      return;
                    }
                    navigator.clipboard.writeText(
                      `ANTHROPIC_BASE_URL=${serverUrl} ANTHROPIC_API_KEY="${config.anthropic_api_key || config.id}" CLAUDE_CODE_MAX_OUTPUT_TOKENS=${config.max_tokens_limit} claude --dangerously-skip-permissions -p "${promptText}"`
                    );
                    message.success('已复制命令，可直接粘贴执行');
                  }}
                  style={{ marginTop: 8 }}
                  disabled={!promptText.trim()}
                >
                  复制命令
                </Button>
              </div>

              {/* 提示信息 */}
              <div style={{ padding: 12, background: '#fff7e6', borderRadius: 4, border: '1px solid #ffd591' }}>
                <Typography.Text style={{ fontSize: 12, display: 'block', marginBottom: 6 }}>
                  💡 <strong>--dangerously-skip-permissions</strong> 参数会跳过权限确认，适合自动化场景
                </Typography.Text>
                <Typography.Text style={{ fontSize: 12, display: 'block', marginBottom: 6 }}>
                  🔢 <strong>CLAUDE_CODE_MAX_OUTPUT_TOKENS={config.max_tokens_limit}</strong> 设置最大输出token与OpenAI API配置一致，避免token错误
                </Typography.Text>
                <Typography.Text style={{ fontSize: 12, display: 'block' }}>
                  💬 <strong>-p "提示词"</strong> 参数用于直接传递提示词给 Claude，适合快速提问
                </Typography.Text>
              </div>
            </Space>
          </Card>
        </Tabs.TabPane>

        {/* Logs Tab */}
        <Tabs.TabPane tab="请求日志" key="logs">
          <RequestLogs configId={id!} />
        </Tabs.TabPane>

        {/* Test Tab */}
        <Tabs.TabPane tab="在线测试" key="test">
          <ConfigTestInline configId={id!} />
        </Tabs.TabPane>
      </Tabs>
    </div>
  );
};

export default ConfigDetailV2;
