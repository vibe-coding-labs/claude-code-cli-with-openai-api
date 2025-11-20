import React, { useState, useEffect } from 'react';
import { useParams, useNavigate, useSearchParams } from 'react-router-dom';
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
  const [renewingKey, setRenewingKey] = useState(false);

  // Get server info from window.location
  const serverPort = window.location.port || '8083';
  const serverHost = window.location.hostname || 'localhost';
  const serverUrl = `http://${serverHost}:${serverPort}`;

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

  const handleRenewKey = () => {
    let customToken = '';
    
    Modal.confirm({
      title: '更新 API Token',
      content: (
        <div>
          <p style={{ marginBottom: 12 }}>
            确定要生成新的 Anthropic API Token 吗？旧的 Token 将立即失效。
          </p>
          <p style={{ marginBottom: 8, fontSize: 13, color: '#666' }}>
            留空将自动生成UUID，也可以自定义Token（英文大小写、数字、下划线，最多100字符）：
          </p>
          <Input 
            placeholder="留空自动生成，或输入自定义Token"
            onChange={(e) => { customToken = e.target.value; }}
            maxLength={100}
          />
        </div>
      ),
      okText: '更新',
      cancelText: '取消',
      okType: 'primary',
      width: 550,
      onOk: async () => {
        setRenewingKey(true);
        try {
          const response = await axios.post(`/api/configs/${id}/renew-key`, {
            custom_token: customToken || undefined,
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

  const copyClaudeConfig = () => {
    const apiKey = config?.anthropic_api_key || config?.id || '';
    const configText = `export ANTHROPIC_BASE_URL=${serverUrl}\nexport ANTHROPIC_API_KEY="${apiKey}"`;
    navigator.clipboard.writeText(configText);
    message.success('配置已复制到剪贴板');
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
            </Descriptions>
          </Card>

          <Card
            title="Anthropic API Token"
            extra={
              <Tooltip title="生成新的 API Token">
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
            <Paragraph style={{ marginBottom: 12 }}>
              使用以下命令配置 Claude Code CLI（请根据实际端口调整）：
            </Paragraph>
            <pre style={{
              background: '#f5f5f5',
              padding: 16,
              borderRadius: 4,
              overflow: 'auto',
              fontSize: 13,
            }}>
{`# 环境变量方式
export ANTHROPIC_BASE_URL=${serverUrl}
export ANTHROPIC_API_KEY="${config.anthropic_api_key || config.id}"

# 或直接在命令中使用
ANTHROPIC_BASE_URL=${serverUrl} \\
ANTHROPIC_API_KEY="${config.anthropic_api_key || config.id}" \\
claude

# 如果服务运行在不同端口，请修改 URL
# 例如: http://localhost:10086`}
            </pre>
            <Button
              type="primary"
              icon={<CopyOutlined />}
              onClick={copyClaudeConfig}
              style={{ marginTop: 8 }}
            >
              复制配置
            </Button>
            <div style={{ marginTop: 16, padding: 12, background: '#e6f7ff', borderRadius: 4, border: '1px solid #91d5ff' }}>
              <Paragraph style={{ marginBottom: 4, fontSize: 13 }}>
                <strong>💡 提示：</strong>
              </Paragraph>
              <ul style={{ margin: 0, paddingLeft: 20, fontSize: 12 }}>
                <li>当前服务地址：<code>{serverUrl}</code></li>
                <li>使用配置的 Anthropic API Token，系统会自动路由到此配置</li>
                <li>请确保端口号与实际运行的服务端口一致</li>
              </ul>
            </div>
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
