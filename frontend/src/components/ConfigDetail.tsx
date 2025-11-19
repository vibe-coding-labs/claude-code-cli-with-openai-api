import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
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
} from 'antd';
import {
  ArrowLeftOutlined,
  EditOutlined,
  DeleteOutlined,
  ThunderboltOutlined,
  ReloadOutlined,
  CopyOutlined,
} from '@ant-design/icons';
import axios from 'axios';

const { TextArea } = Input;
const { TabPane } = Tabs;

interface Config {
  id: string;
  name: string;
  description: string;
  openai_api_key_masked: string;
  openai_base_url: string;
  big_model: string;
  middle_model: string;
  small_model: string;
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
  created_at: string;
}

const ConfigDetail: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [config, setConfig] = useState<Config | null>(null);
  const [stats, setStats] = useState<Stats | null>(null);
  const [logs, setLogs] = useState<RequestLog[]>([]);
  const [loading, setLoading] = useState(true);
  const [testing, setTesting] = useState(false);
  const [editModalVisible, setEditModalVisible] = useState(false);
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

  const handleTest = async () => {
    setTesting(true);
    try {
      const response = await axios.post(`/api/configs/${id}/test`);
      if (response.data.status === 'success') {
        message.success(`测试成功！响应时间: ${response.data.duration_ms}ms`);
        Modal.info({
          title: '测试结果',
          content: (
            <div>
              <p><strong>状态:</strong> 成功</p>
              <p><strong>模型:</strong> {response.data.model}</p>
              <p><strong>响应时间:</strong> {response.data.duration_ms}ms</p>
              <p><strong>Token使用:</strong></p>
              <ul>
                <li>输入: {response.data.usage.input_tokens}</li>
                <li>输出: {response.data.usage.output_tokens}</li>
                <li>总计: {response.data.usage.total_tokens}</li>
              </ul>
              <p><strong>响应内容:</strong></p>
              <TextArea value={response.data.response} autoSize={{ minRows: 3, maxRows: 10 }} readOnly />
            </div>
          ),
          width: 600,
        });
        fetchStats();
        fetchLogs();
      } else {
        message.error(`测试失败: ${response.data.error}`);
      }
    } catch (error: any) {
      message.error(`测试失败: ${error.response?.data?.error || error.message}`);
    } finally {
      setTesting(false);
    }
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
      form.setFieldsValue({
        name: config.name,
        description: config.description,
        openai_base_url: config.openai_base_url,
        big_model: config.big_model,
        middle_model: config.middle_model,
        small_model: config.small_model,
        max_tokens_limit: config.max_tokens_limit,
        request_timeout: config.request_timeout,
        enabled: config.enabled,
      });
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

  const logColumns = [
    {
      title: '时间',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (text: string) => new Date(text).toLocaleString('zh-CN'),
      width: 180,
    },
    {
      title: '模型',
      dataIndex: 'model',
      key: 'model',
      width: 150,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status: string) => (
        <Tag color={status === 'success' ? 'success' : 'error'}>
          {status === 'success' ? '成功' : '失败'}
        </Tag>
      ),
    },
    {
      title: '输入Token',
      dataIndex: 'input_tokens',
      key: 'input_tokens',
      width: 100,
    },
    {
      title: '输出Token',
      dataIndex: 'output_tokens',
      key: 'output_tokens',
      width: 100,
    },
    {
      title: '总Token',
      dataIndex: 'total_tokens',
      key: 'total_tokens',
      width: 100,
    },
    {
      title: '耗时(ms)',
      dataIndex: 'duration_ms',
      key: 'duration_ms',
      width: 100,
    },
    {
      title: '错误信息',
      dataIndex: 'error_message',
      key: 'error_message',
      ellipsis: true,
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
          type="primary"
          icon={<ThunderboltOutlined />}
          onClick={handleTest}
          loading={testing}
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

      <Tabs defaultActiveKey="overview">
        <TabPane tab="概览" key="overview">
          <Card title="配置信息" style={{ marginBottom: 16 }}>
            <Descriptions column={2} bordered>
              <Descriptions.Item label="名称">{config.name}</Descriptions.Item>
              <Descriptions.Item label="状态">
                <Tag color={config.enabled ? 'success' : 'default'}>
                  {config.enabled ? '启用' : '禁用'}
                </Tag>
              </Descriptions.Item>
              <Descriptions.Item label="描述" span={2}>
                {config.description || '-'}
              </Descriptions.Item>
              <Descriptions.Item label="API密钥">{config.openai_api_key_masked}</Descriptions.Item>
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
              使用以下环境变量配置Claude Code CLI以使用此API配置：
            </p>
            <pre style={{
              background: '#f5f5f5',
              padding: '16px',
              borderRadius: '4px',
              overflow: 'auto'
            }}>
{`# 设置环境变量
export ANTHROPIC_BASE_URL=http://localhost:10086/proxy/${id}
export ANTHROPIC_API_KEY="${id}"

# 或在命令行直接使用
ANTHROPIC_BASE_URL=http://localhost:10086/proxy/${id} \\
ANTHROPIC_API_KEY="${id}" \\
claude`}
            </pre>
            <Button
              type="primary"
              icon={<CopyOutlined />}
              onClick={() => {
                const configText = `export ANTHROPIC_BASE_URL=http://localhost:10086/proxy/${id}\nexport ANTHROPIC_API_KEY="${id}"`;
                navigator.clipboard.writeText(configText);
                message.success('配置已复制到剪贴板');
              }}
              style={{ marginTop: 8 }}
            >
              复制配置
            </Button>
            <p style={{ marginTop: 16, color: '#666', fontSize: '12px' }}>
              💡 提示：ANTHROPIC_API_KEY 设置为配置ID即可，系统会根据路径自动识别使用哪个配置
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
        </TabPane>

        <TabPane tab="请求日志" key="logs">
          <Table
            dataSource={logs}
            columns={logColumns}
            rowKey="id"
            pagination={{ pageSize: 20 }}
            scroll={{ x: 1000 }}
          />
        </TabPane>
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
          <Form.Item
            name="big_model"
            label="大模型 (Opus)"
            rules={[{ required: true, message: '请输入模型名称' }]}
          >
            <Input />
          </Form.Item>
          <Form.Item
            name="middle_model"
            label="中模型 (Sonnet)"
            rules={[{ required: true, message: '请输入模型名称' }]}
          >
            <Input />
          </Form.Item>
          <Form.Item
            name="small_model"
            label="小模型 (Haiku)"
            rules={[{ required: true, message: '请输入模型名称' }]}
          >
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
