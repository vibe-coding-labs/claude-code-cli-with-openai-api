/* eslint-disable react-hooks/exhaustive-deps */
/* eslint-disable @typescript-eslint/no-unused-vars */
import React, { useState, useEffect } from 'react';
import { useParams, useNavigate, useSearchParams } from 'react-router-dom';
import { usePageTitle } from '../utils/pageTitle';
import { getApiOrigin } from '../services/apiBase';
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
} from 'antd';
import {
  ArrowLeftOutlined,
  EditOutlined,
  DeleteOutlined,
  ReloadOutlined,
  CopyOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import { oneDark } from 'react-syntax-highlighter/dist/esm/styles/prism';
import axios from 'axios';
import RequestLogs from './RequestLogs';
import ConfigTestInline from './ConfigTestInline';

const { Text } = Typography;

// Syntax-highlighted code block with copy button
const CodeBlock: React.FC<{
  code: string;
  language?: string;
  title?: string;
}> = ({ code, language = 'bash', title }) => (
  <div style={{ position: 'relative', borderRadius: 8, overflow: 'hidden', marginBottom: 4 }}>
    {title ? (
      <div style={{
        background: '#2d2d2d', color: '#ccc', padding: '6px 12px',
        fontSize: 12, fontFamily: 'monospace', borderBottom: '1px solid #444',
        display: 'flex', justifyContent: 'space-between', alignItems: 'center',
      }}>
        <span>{title}</span>
        <Button
          type="text" size="small" icon={<CopyOutlined />}
          style={{ color: '#999', fontSize: 11 }}
          onClick={() => { navigator.clipboard.writeText(code); message.success('已复制'); }}
        >
          复制
        </Button>
      </div>
    ) : (
      <Button
        type="text" size="small" icon={<CopyOutlined />}
        style={{ position: 'absolute', top: 6, right: 6, color: '#666', zIndex: 1 }}
        onClick={() => { navigator.clipboard.writeText(code); message.success('已复制'); }}
      />
    )}
    <SyntaxHighlighter
      language={language} style={oneDark}
      customStyle={{
        margin: 0, borderRadius: title ? 0 : 8,
        fontSize: 13, lineHeight: 1.6, padding: '12px 16px',
      }}
    >
      {code}
    </SyntaxHighlighter>
  </div>
);

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
  retry_backoff_base?: number;
  retry_backoff_max?: number;
  reasoning_effort?: string;
  big_model_reasoning_effort?: string;
  middle_model_reasoning_effort?: string;
  small_model_reasoning_effort?: string;
  enabled: boolean;
  expires_at?: string;
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
  const [clientStats, setClientStats] = useState<any>(null);

  const serverUrl = getApiOrigin() || `${window.location.protocol}//${window.location.host}`;

  useEffect(() => {
    fetchConfigDetail();
    fetchClientStats();
    const interval = setInterval(fetchClientStats, 30000);
    return () => clearInterval(interval);
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

  const fetchClientStats = async () => {
    try {
      const response = await axios.get(`/api/configs/${id}/client-stats`);
      setClientStats(response.data);
    } catch (error) {
      console.error('获取客户端统计失败:', error);
    }
  };

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

  const handleCustomToken = () => {
    let customToken = '';

    const validateToken = (value: string): { valid: boolean; message?: string } => {
      if (!value) return { valid: false, message: 'Token不能为空' };
      if (value.length < 1 || value.length > 100) return { valid: false, message: 'Token长度必须在1-100个字符之间' };
      if (!/^[a-zA-Z0-9_-]+$/.test(value)) return { valid: false, message: 'Token只能包含英文字母、数字、下划线(_)、连字符(-)' };
      return { valid: true };
    };

    Modal.confirm({
      title: '自定义 Anthropic API Token',
      content: (
        <div>
          <p style={{ marginBottom: 12, color: '#ff4d4f', fontWeight: 500 }}>
            ⚠️ 设置自定义Token后，旧的 Token 将<strong>立即失效</strong>。
          </p>
          <p style={{ marginBottom: 8, fontSize: 13, color: '#666' }}>请输入自定义Token：</p>
          <Input
            id="custom-token-input"
            placeholder="英文字母、数字、下划线、连字符，长度1-100"
            onChange={(e) => {
              customToken = e.target.value;
              const validation = validateToken(customToken);
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
          <div id="token-error-message" style={{ display: 'none', color: '#ff4d4f', fontSize: 12, marginTop: 4 }} />
          <p style={{ marginTop: 12, fontSize: 12, color: '#999' }}>
            格式要求：英文字母(a-z, A-Z)、数字(0-9)、下划线(_)和连字符(-)，长度 1-100
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
                <Input.TextArea value={response.data.new_api_key} readOnly autoSize style={{ fontFamily: 'monospace', fontSize: 13 }} />
                <Button type="link" icon={<CopyOutlined />}
                  onClick={() => { navigator.clipboard.writeText(response.data.new_api_key); message.success('已复制到剪贴板'); }}
                  style={{ marginTop: 8 }}>
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
          navigate('/ui/configs');
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
    return <div style={{ textAlign: 'center', padding: 50 }}><Spin size="large" /></div>;
  }

  if (!config) {
    return <div>配置未找到</div>;
  }

  const anthropicApiKey = config.anthropic_api_key || config.id;
  const normalizedServerUrl = serverUrl.endsWith('/') ? serverUrl : `${serverUrl}/`;

  const apiTimeoutMs = (config.request_timeout || 180) * 1000;
  const maxRetries = config.retry_count || 3;
  const defaultModel = config.big_model || '';

  const buildClaudeCliCommand = (prompt?: string) => {
    const commandLine = prompt !== undefined
      ? `  claude --dangerously-skip-permissions -p "${prompt}"`
      : '  claude --dangerously-skip-permissions';
    return [
      `API_TIMEOUT_MS=${apiTimeoutMs} \\`,
      `  CLAUDE_CODE_MAX_RETRIES=${maxRetries} \\`,
      '  CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1 \\',
      `  ANTHROPIC_BASE_URL=${normalizedServerUrl} \\`,
      `  ANTHROPIC_API_KEY="${anthropicApiKey}" \\`,
      `  CLAUDE_CODE_MAX_OUTPUT_TOKENS=${config.max_tokens_limit} \\`,
      `  ANTHROPIC_MODEL=${defaultModel} \\`,
      commandLine,
    ].join('\n');
  };

  const oneTimeCommand = buildClaudeCliCommand();
  const promptCommand = buildClaudeCliCommand(promptText);
  const persistentConfig = `export API_TIMEOUT_MS=${apiTimeoutMs}
export CLAUDE_CODE_MAX_RETRIES=${maxRetries}
export CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1
export ANTHROPIC_BASE_URL=${normalizedServerUrl}
export ANTHROPIC_API_KEY="${anthropicApiKey}"
export CLAUDE_CODE_MAX_OUTPUT_TOKENS=${config.max_tokens_limit}
export ANTHROPIC_MODEL=${defaultModel}
alias claude='command claude --dangerously-skip-permissions'`;
  const oneClickSetupScript = `echo 'export API_TIMEOUT_MS=${apiTimeoutMs}' >> ~/.zshrc && echo 'export CLAUDE_CODE_MAX_RETRIES=${maxRetries}' >> ~/.zshrc && echo 'export CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1' >> ~/.zshrc && echo 'export ANTHROPIC_BASE_URL=${normalizedServerUrl}' >> ~/.zshrc && echo 'export ANTHROPIC_API_KEY="${anthropicApiKey}"' >> ~/.zshrc && echo 'export CLAUDE_CODE_MAX_OUTPUT_TOKENS=${config.max_tokens_limit}' >> ~/.zshrc && echo 'export ANTHROPIC_MODEL=${defaultModel}' >> ~/.zshrc && echo "alias claude='command claude --dangerously-skip-permissions'" >> ~/.zshrc && source ~/.zshrc`;

  return (
    <div>
      {/* Compact header with name, status, meta */}
      <div style={{
        marginBottom: 20, display: 'flex', justifyContent: 'space-between',
        alignItems: 'flex-start', gap: 16, flexWrap: 'wrap',
      }}>
        <div style={{ flex: '1 1 auto', minWidth: 300 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 6 }}>
            <Button
              type="text" icon={<ArrowLeftOutlined />}
              onClick={() => navigate('/ui/configs')}
              style={{ marginLeft: -8, color: 'var(--color-text-secondary)' }}
            />
            <h2 style={{ margin: 0, fontSize: 20, fontWeight: 600, color: 'var(--color-text-primary)' }}>
              {config.name}
            </h2>
            <Tag color={config.enabled ? 'success' : 'default'} style={{ fontSize: 12 }}>
              {config.enabled ? '启用' : '禁用'}
            </Tag>
          </div>
          {config.description && (
            <Text type="secondary" style={{ fontSize: 13, marginLeft: 42, display: 'block', marginBottom: 4 }}>
              {config.description}
            </Text>
          )}
          <div style={{ display: 'flex', gap: 16, marginTop: 6, marginLeft: 42, flexWrap: 'wrap' }}>
            <Text type="secondary" style={{ fontSize: 12 }}>
              ID: <Text copyable style={{ fontFamily: 'monospace', fontSize: 11 }}>{config.id}</Text>
            </Text>
            <Text type="secondary" style={{ fontSize: 12 }}>
              创建于 {new Date(config.created_at).toLocaleDateString('zh-CN')}
            </Text>
            <Text type="secondary" style={{ fontSize: 12 }}>
              Base URL: <Text copyable style={{ fontFamily: 'monospace', fontSize: 11 }}>{config.openai_base_url}</Text>
            </Text>
          </div>
        </div>
        <Space>
          <Button type="primary" icon={<EditOutlined />}
            onClick={() => navigate(`/ui/configs/${id}/edit`)}>
            编辑
          </Button>
          <Button icon={<ReloadOutlined />} onClick={fetchConfigDetail}>刷新</Button>
          <Button danger icon={<DeleteOutlined />} onClick={handleDelete}>删除</Button>
        </Space>
      </div>

      {/* Client status — compact inline bar */}
      {clientStats && (
        <div style={{ marginBottom: 12 }}>
          {clientStats.active_clients > 0 ? (
            <div style={{
              padding: '8px 16px', borderRadius: 8,
              background: 'var(--color-success-bg)', border: '1px solid #bbf7d0',
              display: 'flex', alignItems: 'center', gap: 8, fontSize: 13,
            }}>
              <Tag color="success" style={{ margin: 0 }}>活跃</Tag>
              <span>
                {clientStats.has_concurrent
                  ? `至少 ${clientStats.estimated_clients} 个客户端在线`
                  : `${clientStats.active_clients} 个客户端在线`}
              </span>
              <span style={{ color: 'var(--color-text-tertiary)', fontSize: 12, marginLeft: 'auto' }}>
                最后请求: {clientStats.last_request_at ? new Date(clientStats.last_request_at).toLocaleTimeString('zh-CN') : '-'}
              </span>
            </div>
          ) : clientStats.total_clients > 0 ? (
            <div style={{
              padding: '8px 16px', borderRadius: 8,
              background: 'var(--color-warning-bg)', border: '1px solid #fed7aa',
              display: 'flex', alignItems: 'center', gap: 8, fontSize: 13,
            }}>
              <Tag color="warning" style={{ margin: 0 }}>空闲</Tag>
              <span>24h 内 {clientStats.total_clients} 个客户端使用过</span>
              <span style={{ color: 'var(--color-text-tertiary)', fontSize: 12, marginLeft: 'auto' }}>
                最后请求: {clientStats.last_request_at ? new Date(clientStats.last_request_at).toLocaleTimeString('zh-CN') : '-'}
              </span>
            </div>
          ) : (
            <div style={{
              padding: '8px 16px', borderRadius: 8,
              background: 'var(--color-border-light)',
              display: 'flex', alignItems: 'center', gap: 8, fontSize: 13,
            }}>
              <Tag style={{ margin: 0 }}>未使用</Tag>
              <span style={{ color: 'var(--color-text-tertiary)' }}>最近 24h 无使用记录</span>
            </div>
          )}
        </div>
      )}

      {/* Expiry warning */}
      {config.expires_at && new Date(config.expires_at) < new Date() && (
        <div style={{
          marginBottom: 12, padding: '8px 16px', borderRadius: 8,
          background: 'var(--color-error-bg)', border: '1px solid #fecaca',
          display: 'flex', alignItems: 'center', gap: 8, fontSize: 13,
        }}>
          <Tag color="error" style={{ margin: 0 }}>已过期</Tag>
          <span>密钥已于 {new Date(config.expires_at).toLocaleString('zh-CN')} 过期，API 调用将被拒绝</span>
        </div>
      )}

      <Tabs
        activeKey={activeTab}
        onChange={handleTabChange}
        items={[
          {
            key: 'overview',
            label: '详情',
            children: (
              <>
                {/* Two-column: Basic info + OpenAI config */}
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, marginBottom: 16 }}>
                  <Card title="基本信息" size="small">
                    <Descriptions column={1} size="small">
                      <Descriptions.Item label="配置 ID">
                        <Text copyable style={{ fontFamily: 'monospace', fontSize: 12 }}>{config.id}</Text>
                      </Descriptions.Item>
                      <Descriptions.Item label="状态">
                        <Tag color={config.enabled ? 'success' : 'default'}>
                          {config.enabled ? '启用' : '禁用'}
                        </Tag>
                      </Descriptions.Item>
                      <Descriptions.Item label="创建时间">
                        {new Date(config.created_at).toLocaleString('zh-CN')}
                      </Descriptions.Item>
                      <Descriptions.Item label="更新时间">
                        {new Date(config.updated_at).toLocaleString('zh-CN')}
                      </Descriptions.Item>
                      <Descriptions.Item label="过期时间">
                        {config.expires_at ? (
                          <span>
                            {new Date(config.expires_at).toLocaleString('zh-CN')}
                            {new Date(config.expires_at) < new Date()
                              ? <Tag color="error" style={{ marginLeft: 8 }}>已过期</Tag>
                              : <Tag color="success" style={{ marginLeft: 8 }}>有效</Tag>
                            }
                          </span>
                        ) : (
                          <span style={{ color: 'var(--color-text-tertiary)' }}>永久有效</span>
                        )}
                      </Descriptions.Item>
                    </Descriptions>
                  </Card>

                  <Card title="OpenAI 配置" size="small">
                    <Descriptions column={1} size="small">
                      <Descriptions.Item label="API Key">
                        <Text style={{ fontFamily: 'monospace', fontSize: 12 }}>
                          {config.openai_api_key_masked}
                        </Text>
                      </Descriptions.Item>
                      <Descriptions.Item label="Base URL">
                        <Text copyable style={{ fontFamily: 'monospace', fontSize: 12 }}>
                          {config.openai_base_url}
                        </Text>
                      </Descriptions.Item>
                      <Descriptions.Item label="大模型">
                        <Text strong>{config.big_model}</Text>
                        {config.big_model_reasoning_effort && (
                          <Tag color="blue" style={{ marginLeft: 6, fontSize: 11 }}>
                            {config.big_model_reasoning_effort.toUpperCase()}
                          </Tag>
                        )}
                      </Descriptions.Item>
                      <Descriptions.Item label="中模型">
                        <Text strong>{config.middle_model}</Text>
                        {config.middle_model_reasoning_effort && (
                          <Tag color="blue" style={{ marginLeft: 6, fontSize: 11 }}>
                            {config.middle_model_reasoning_effort.toUpperCase()}
                          </Tag>
                        )}
                      </Descriptions.Item>
                      <Descriptions.Item label="小模型">
                        <Text strong>{config.small_model}</Text>
                        {config.small_model_reasoning_effort && (
                          <Tag color="blue" style={{ marginLeft: 6, fontSize: 11 }}>
                            {config.small_model_reasoning_effort.toUpperCase()}
                          </Tag>
                        )}
                      </Descriptions.Item>
                    </Descriptions>
                  </Card>
                </div>

                {/* Request parameters — compact stat row */}
                <Card title="请求参数" size="small" style={{ marginBottom: 16 }}>
                  <div style={{ display: 'flex', gap: 24, flexWrap: 'wrap' }}>
                    {[
                      { label: '最大 Token', value: config.max_tokens_limit },
                      { label: '超时', value: `${config.request_timeout}s` },
                      { label: '重试次数', value: config.retry_count || 3 },
                      { label: '退避基数', value: `${config.retry_backoff_base || 1}s` },
                      { label: '退避上限', value: `${config.retry_backoff_max || 60}s` },
                      { label: '思考级别', value: config.reasoning_effort ? config.reasoning_effort.toUpperCase() : '默认' },
                    ].map(item => (
                      <div key={item.label} style={{ textAlign: 'center', minWidth: 80 }}>
                        <div style={{ fontSize: 11, color: 'var(--color-text-tertiary)', marginBottom: 2 }}>
                          {item.label}
                        </div>
                        <div style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-text-primary)' }}>
                          {item.value}
                        </div>
                      </div>
                    ))}
                  </div>
                </Card>

                {/* Anthropic API Token */}
                <Card
                  title="Anthropic API Token"
                  size="small"
                  extra={
                    <Space size="small">
                      <Tooltip title="自动生成 UUID Token">
                        <Button type="link" icon={<SyncOutlined spin={renewingKey} />}
                          onClick={handleRenewKey} loading={renewingKey} size="small">
                          更新
                        </Button>
                      </Tooltip>
                      <Tooltip title="自定义 Token">
                        <Button type="link" icon={<EditOutlined />}
                          onClick={handleCustomToken} loading={renewingKey} size="small">
                          自定义
                        </Button>
                      </Tooltip>
                    </Space>
                  }
                  style={{ marginBottom: 16 }}
                >
                  <CodeBlock code={anthropicApiKey} language="text" />
                  <Text type="secondary" style={{ fontSize: 12, marginTop: 8, display: 'block' }}>
                    使用此 Token 作为 ANTHROPIC_API_KEY
                  </Text>
                </Card>

                {/* CLI config — with syntax highlighting */}
                <Card title="Claude Code CLI 配置" size="small">
                  <Space direction="vertical" style={{ width: '100%' }} size="middle">
                    <div>
                      <Text strong style={{ fontSize: 13, display: 'block', marginBottom: 8 }}>
                        单次执行（推荐）
                      </Text>
                      <CodeBlock code={oneTimeCommand} language="bash" title="bash" />
                    </div>

                    <div>
                      <Text strong style={{ fontSize: 13, display: 'block', marginBottom: 8 }}>
                        永久配置（添加到 ~/.zshrc）
                      </Text>
                      <CodeBlock code={persistentConfig} language="bash" title="~/.zshrc" />
                    </div>

                    <div>
                      <Text strong style={{ fontSize: 13, display: 'block', marginBottom: 8 }}>
                        一键配置脚本
                      </Text>
                      <CodeBlock code={oneClickSetupScript} language="bash" title="one-click setup" />
                      <Text type="secondary" style={{ fontSize: 11, display: 'block', marginTop: 4 }}>
                        使用 bash 的话，将 ~/.zshrc 改为 ~/.bashrc
                      </Text>
                    </div>

                    <div>
                      <Text strong style={{ fontSize: 13, display: 'block', marginBottom: 8 }}>
                        带提示词执行
                      </Text>
                      <Input
                        placeholder="输入提示词，例如：Hello, Claude!"
                        value={promptText}
                        onChange={(e) => setPromptText(e.target.value)}
                        style={{ marginBottom: 8 }}
                        suffix={promptText && (
                          <Button type="text" size="small"
                            onClick={() => setPromptText('')}
                            style={{ padding: 0, height: 'auto' }}>
                            清空
                          </Button>
                        )}
                      />
                      <CodeBlock code={promptCommand} language="bash" title="bash -p" />
                    </div>

                    <div style={{
                      padding: 12, borderRadius: 8,
                      background: 'var(--color-warning-bg)', border: '1px solid #fed7aa',
                    }}>
                      <Text style={{ fontSize: 12, display: 'block', marginBottom: 4 }}>
                        <Text strong>--dangerously-skip-permissions</Text> 跳过权限确认，适合自动化
                      </Text>
                      <Text style={{ fontSize: 12, display: 'block', marginBottom: 4 }}>
                        <Text strong>CLAUDE_CODE_MAX_OUTPUT_TOKENS={config.max_tokens_limit}</Text> 与 OpenAI 配置一致
                      </Text>
                      <Text style={{ fontSize: 12, display: 'block' }}>
                        <Text strong>-p "提示词"</Text> 直接传递提示词
                      </Text>
                    </div>
                  </Space>
                </Card>
              </>
            )
          },
          {
            key: 'logs',
            label: '请求日志',
            children: <RequestLogs configId={id!} />
          },
          {
            key: 'test',
            label: '在线测试',
            children: <ConfigTestInline configId={id!} />
          }
        ]}
      />
    </div>
  );
};

export default ConfigDetailV2;
