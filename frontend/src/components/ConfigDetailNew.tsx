import React, { useState, useEffect } from 'react';
import { useParams, useNavigate, useSearchParams } from 'react-router-dom';
import {
  Card,
  Descriptions,
  Button,
  Space,
  Statistic,
  Row,
  Col,
  Table,
  message,
  Modal,
  Tag,
  Spin,
  Tabs,
  Form,
  Input,
  Select,
  Switch,
  Typography,
} from 'antd';
import {
  ArrowLeftOutlined,
  EditOutlined,
  DeleteOutlined,
  ThunderboltOutlined,
  ReloadOutlined,
  CopyOutlined,
  KeyOutlined,
} from '@ant-design/icons';
import axios from 'axios';

const { TextArea } = Input;
const { Text, Paragraph } = Typography;
const { Option } = Select;

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

interface Stats {
  total_requests: number;
  success_requests: number;
  error_requests: number;
  total_input_tokens: number;
  total_output_tokens: number;
  total_tokens: number;
  avg_duration_ms: number;
}

interface RequestLog {
  id: number;
  model: string;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  duration_ms: number;
  status: string;
  error_message?: string;
  request_summary?: string;
  response_preview?: string;
  request_body?: string;
  response_body?: string;
  created_at: string;
}

const ConfigDetail: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const activeTab = searchParams.get('tab') || 'overview';
  
  const [config, setConfig] = useState<Config | null>(null);
  const [stats, setStats] = useState<Stats | null>(null);
  const [logs, setLogs] = useState<RequestLog[]>([]);
  const [loading, setLoading] = useState(true);
  const [editModalVisible, setEditModalVisible] = useState(false);
  const [renewingKey, setRenewingKey] = useState(false);
  const [form] = Form.useForm();

  useEffect(() => {
    fetchConfigDetail();
    fetchStats();
    fetchLogs();
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

  const fetchStats = async () => {
    try {
      const response = await axios.get(`/api/configs/${id}/stats?days=30`);
      setStats(response.data.stats);
    } catch (error) {
      message.error('获取统计信息失败');
    }
  };

  const fetchLogs = async () => {
    try {
      const response = await axios.get(`/api/configs/${id}/logs?limit=50`);
      setLogs(response.data.logs || []);
    } catch (error) {
      message.error('获取请求日志失败');
    }
  };

  const handleRenewKey = () => {
    Modal.confirm({
      title: '更新 API Key',
      content: '确定要生成新的 Anthropic API Key 吗？旧的 Key 将失效。',
      okText: '更新',
      cancelText: '取消',
      onOk: async () => {
        setRenewingKey(true);
        try {
          const response = await axios.post(`/api/configs/${id}/renew-key`);
          Modal.success({
            title: 'API Key 已更新',
            content: (
              <div>
                <p>新的 API Key：</p>
                <Input.TextArea 
                  value={response.data.new_api_key} 
                  readOnly 
                  autoSize
                  style={{ fontFamily: 'monospace' }}
                />
                <Button 
                  type="link"
                  icon={<CopyOutlined />}
                  onClick={() => {
                    navigator.clipboard.writeText(response.data.new_api_key);
                    message.success('已复制到剪贴板');
                  }}
                >
                  复制
                </Button>
                <p style={{ marginTop: 16, color: '#ff4d4f' }}>
                  ⚠️ 请立即保存此 Key，关闭后将无法再次查看！
                </p>
              </div>
            ),
            width: 600,
          });
          fetchConfigDetail();
        } catch (error: any) {
          message.error(`更新失败: ${error.response?.data?.error || error.message}`);
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

  const handleEdit = () => {
    if (config) {
      form.setFieldsValue(config);
      setEditModalVisible(true);
    }
  };

  const handleUpdate = async (values: any) => {
    try {
      await axios.put(`/api/configs/${id}`, values);
      message.success('更新成功');
      setEditModalVisible(false);
      fetchConfigDetail();
    } catch (error) {
      message.error('更新失败');
    }
  };

  const handleTabChange = (key: string) => {
    setSearchParams({ tab: key });
  };

  const logColumns = [
    {
      title: '时间',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (text: string) => new Date(text).toLocaleString('zh-CN'),
      width: 180,
      fixed: 'left' as const,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 80,
      render: (status: string) => (
        <Tag color={status === 'success' ? 'success' : 'error'}>
          {status === 'success' ? '成功' : '失败'}
        </Tag>
      ),
    },
    {
      title: '模型',
      dataIndex: 'model',
      key: 'model',
      width: 180,
      ellipsis: true,
    },
    {
      title: '请求摘要',
      dataIndex: 'request_summary',
      key: 'request_summary',
      width: 300,
      ellipsis: true,
      render: (text: string) => text || '-',
    },
    {
      title: '响应预览',
      dataIndex: 'response_preview',
      key: 'response_preview',
      width: 300,
      ellipsis: true,
      render: (text: string) => text || '-',
    },
    {
      title: '输入Token',
      dataIndex: 'input_tokens',
      key: 'input_tokens',
      width: 100,
      align: 'right' as const,
    },
    {
      title: '输出Token',
      dataIndex: 'output_tokens',
      key: 'output_tokens',
      width: 100,
      align: 'right' as const,
    },
    {
      title: '总Token',
      dataIndex: 'total_tokens',
      key: 'total_tokens',
      width: 100,
      align: 'right' as const,
    },
    {
      title: '耗时(ms)',
      dataIndex: 'duration_ms',
      key: 'duration_ms',
      width: 100,
      align: 'right' as const,
    },
    {
      title: '操作',
      key: 'action',
      width: 100,
      fixed: 'right' as const,
      render: (_: any, record: RequestLog) => (
        <Button
          type="link"
          size="small"
          onClick={() => {
            Modal.info({
              title: '请求详情',
              width: 800,
              content: (
                <div>
                  <Descriptions column={2} bordered size="small" style={{ marginBottom: 16 }}>
                    <Descriptions.Item label="ID">{record.id}</Descriptions.Item>
                    <Descriptions.Item label="时间">
                      {new Date(record.created_at).toLocaleString('zh-CN')}
                    </Descriptions.Item>
                    <Descriptions.Item label="模型" span={2}>{record.model}</Descriptions.Item>
                    <Descriptions.Item label="状态">{record.status}</Descriptions.Item>
                    <Descriptions.Item label="耗时">{record.duration_ms}ms</Descriptions.Item>
                    <Descriptions.Item label="输入Token">{record.input_tokens}</Descriptions.Item>
                    <Descriptions.Item label="输出Token">{record.output_tokens}</Descriptions.Item>
                  </Descriptions>
                  
                  {record.error_message && (
                    <>
                      <Text strong>错误信息：</Text>
                      <Paragraph style={{ color: 'red', marginTop: 8 }}>
                        {record.error_message}
                      </Paragraph>
                    </>
                  )}
                  
                  {record.request_body && (
                    <>
                      <Text strong>完整请求体：</Text>
                      <TextArea
                        value={JSON.stringify(JSON.parse(record.request_body), null, 2)}
                        autoSize={{ minRows: 5, maxRows: 15 }}
                        readOnly
                        style={{ marginTop: 8, fontFamily: 'monospace', fontSize: 12 }}
                      />
                    </>
                  )}
                  
                  {record.response_body && (
                    <>
                      <Text strong style={{ marginTop: 16, display: 'block' }}>完整响应体：</Text>
                      <TextArea
                        value={JSON.stringify(JSON.parse(record.response_body), null, 2)}
                        autoSize={{ minRows: 5, maxRows: 15 }}
                        readOnly
                        style={{ marginTop: 8, fontFamily: 'monospace', fontSize: 12 }}
                      />
                    </>
                  )}
                </div>
              ),
            });
          }}
        >
          详情
        </Button>
      ),
    },
  ];

  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: '50px' }}>
        <Spin size="large" />
      </div>
    );
  }

  if (!config) {
    return <div>配置未找到</div>;
  }

  const successRate = stats && stats.total_requests > 0
    ? ((stats.success_requests / stats.total_requests) * 100).toFixed(2)
    : '0';

  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/ui')}>
          返回
        </Button>
        <Button icon={<EditOutlined />} onClick={handleEdit}>
          编辑
        </Button>
        <Button
          icon={<KeyOutlined />}
          onClick={handleRenewKey}
          loading={renewingKey}
        >
          更新 API Key
        </Button>
        <Button
          type="primary"
          icon={<ThunderboltOutlined />}
          onClick={() => navigate(`/ui/configs/${id}/test`)}
        >
          在线测试
        </Button>
        <Button icon={<ReloadOutlined />} onClick={() => { fetchStats(); fetchLogs(); }}>
          刷新
        </Button>
        <Button danger icon={<DeleteOutlined />} onClick={handleDelete}>
          删除
        </Button>
      </Space>

      <Tabs activeKey={activeTab} onChange={handleTabChange}>
        <Tabs.TabPane tab="概览" key="overview">
          <Card title="配置信息" style={{ marginBottom: 16 }}>
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
              <Descriptions.Item label="OpenAI API Key">{config.openai_api_key_masked}</Descriptions.Item>
              <Descriptions.Item label="Base URL">{config.openai_base_url}</Descriptions.Item>
              <Descriptions.Item label="大模型 (Opus)">{config.big_model}</Descriptions.Item>
              <Descriptions.Item label="中模型 (Sonnet)">{config.middle_model}</Descriptions.Item>
              <Descriptions.Item label="小模型 (Haiku)">{config.small_model}</Descriptions.Item>
              <Descriptions.Item label="最大Token限制">{config.max_tokens_limit}</Descriptions.Item>
              <Descriptions.Item label="请求超时(秒)">{config.request_timeout}</Descriptions.Item>
              <Descriptions.Item label="创建时间">
                {new Date(config.created_at).toLocaleString('zh-CN')}
              </Descriptions.Item>
              <Descriptions.Item label="更新时间">
                {new Date(config.updated_at).toLocaleString('zh-CN')}
              </Descriptions.Item>
            </Descriptions>
          </Card>

          <Card title="Claude Code CLI 配置" style={{ marginBottom: 16 }}>
            <p style={{ marginBottom: 16 }}>
              使用以下环境变量配置 Claude Code CLI：
            </p>
            <pre style={{
              background: '#f5f5f5',
              padding: '16px',
              borderRadius: '4px',
              overflow: 'auto'
            }}>
{`# 设置环境变量
export ANTHROPIC_BASE_URL=http://localhost:8083
export ANTHROPIC_API_KEY="${config.anthropic_api_key || config.id}"

# 或在命令行直接使用
ANTHROPIC_BASE_URL=http://localhost:8083 \\
ANTHROPIC_API_KEY="${config.anthropic_api_key || config.id}" \\
claude`}
            </pre>
            <Button
              type="primary"
              icon={<CopyOutlined />}
              onClick={() => {
                const configText = `export ANTHROPIC_BASE_URL=http://localhost:8083\nexport ANTHROPIC_API_KEY="${config.anthropic_api_key || config.id}"`;
                navigator.clipboard.writeText(configText);
                message.success('配置已复制到剪贴板');
              }}
              style={{ marginTop: 8 }}
            >
              复制配置
            </Button>
            <p style={{ marginTop: 16, color: '#666', fontSize: '12px' }}>
              💡 提示：使用配置的 Anthropic API Key，系统会自动识别并使用对应的配置
            </p>
          </Card>

          {stats && (
            <Card title="使用统计 (最近30天)" style={{ marginBottom: 16 }}>
              <Row gutter={16}>
                <Col span={6}>
                  <Statistic title="总请求数" value={stats.total_requests} />
                </Col>
                <Col span={6}>
                  <Statistic
                    title="成功率"
                    value={successRate}
                    suffix="%"
                    valueStyle={{ color: parseFloat(successRate) > 95 ? '#3f8600' : '#cf1322' }}
                  />
                </Col>
                <Col span={6}>
                  <Statistic title="总Token消耗" value={stats.total_tokens} />
                </Col>
                <Col span={6}>
                  <Statistic
                    title="平均响应时间"
                    value={stats.avg_duration_ms.toFixed(0)}
                    suffix="ms"
                  />
                </Col>
              </Row>
              <Row gutter={16} style={{ marginTop: 16 }}>
                <Col span={8}>
                  <Statistic title="输入Token" value={stats.total_input_tokens} />
                </Col>
                <Col span={8}>
                  <Statistic title="输出Token" value={stats.total_output_tokens} />
                </Col>
                <Col span={8}>
                  <Statistic
                    title="错误数"
                    value={stats.error_requests}
                    valueStyle={{ color: stats.error_requests > 0 ? '#cf1322' : '#3f8600' }}
                  />
                </Col>
              </Row>
            </Card>
          )}
        </Tabs.TabPane>

        <Tabs.TabPane tab="请求日志" key="logs">
          <Table
            dataSource={logs}
            columns={logColumns}
            rowKey="id"
            pagination={{ pageSize: 20 }}
            scroll={{ x: 1800 }}
          />
        </Tabs.TabPane>
      </Tabs>

      <Modal
        title="编辑配置"
        open={editModalVisible}
        onCancel={() => setEditModalVisible(false)}
        onOk={() => form.submit()}
        width={600}
      >
        <Form form={form} onFinish={handleUpdate} layout="vertical">
          <Form.Item
            name="name"
            label="名称"
            rules={[{ required: true, message: '请输入名称' }]}
          >
            <Input />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <TextArea rows={3} />
          </Form.Item>
          <Form.Item name="openai_api_key" label="OpenAI API Key (留空保持不变)">
            <Input.Password placeholder="留空则不修改" />
          </Form.Item>
          <Form.Item
            name="openai_base_url"
            label="Base URL"
            rules={[{ required: true, message: '请输入Base URL' }]}
          >
            <Input />
          </Form.Item>
          <Form.Item name="big_model" label="大模型 (Opus)" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="middle_model" label="中模型 (Sonnet)" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="small_model" label="小模型 (Haiku)" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="max_tokens_limit" label="最大Token限制">
            <Input type="number" />
          </Form.Item>
          <Form.Item name="request_timeout" label="请求超时(秒)">
            <Input type="number" />
          </Form.Item>
          <Form.Item name="enabled" label="启用" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default ConfigDetail;
