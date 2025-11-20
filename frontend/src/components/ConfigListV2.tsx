import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Table,
  Button,
  Space,
  Tag,
  Popconfirm,
  message,
  Card,
  Typography,
  Modal,
  Form,
  Input,
  InputNumber,
  Select,
  Row,
  Col,
} from 'antd';
import {
  PlusOutlined,
  DeleteOutlined,
  EyeOutlined,
  SearchOutlined,
  ReloadOutlined,
  FilterOutlined,
} from '@ant-design/icons';
import axios from 'axios';

const { TextArea } = Input;
const { Title } = Typography;
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
  enabled: boolean;
  created_at: string;
}

const ConfigListV2: React.FC = () => {
  const navigate = useNavigate();
  const [configs, setConfigs] = useState<Config[]>([]);
  const [filteredConfigs, setFilteredConfigs] = useState<Config[]>([]);
  const [loading, setLoading] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [form] = Form.useForm();
  
  // Filter states
  const [searchText, setSearchText] = useState('');
  const [statusFilter, setStatusFilter] = useState<string>('');
  const [sortField, setSortField] = useState<string>('created_at');
  const [sortOrder, setSortOrder] = useState<'ascend' | 'descend'>('descend');

  const loadConfigs = async () => {
    setLoading(true);
    try {
      const response = await axios.get('/api/configs');
      setConfigs(response.data.configs || []);
    } catch (error: any) {
      message.error('加载配置失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadConfigs();
  }, []);

  // Apply filters
  useEffect(() => {
    let result = [...configs];

    // Search filter
    if (searchText) {
      result = result.filter(config =>
        config.name.toLowerCase().includes(searchText.toLowerCase()) ||
        config.description?.toLowerCase().includes(searchText.toLowerCase()) ||
        config.openai_base_url.toLowerCase().includes(searchText.toLowerCase())
      );
    }

    // Status filter
    if (statusFilter === 'enabled') {
      result = result.filter(config => config.enabled);
    } else if (statusFilter === 'disabled') {
      result = result.filter(config => !config.enabled);
    }

    // Sorting
    result.sort((a, b) => {
      let aVal: any = a[sortField as keyof Config];
      let bVal: any = b[sortField as keyof Config];

      if (sortField === 'created_at') {
        aVal = new Date(aVal).getTime();
        bVal = new Date(bVal).getTime();
      }

      if (sortOrder === 'ascend') {
        return aVal > bVal ? 1 : -1;
      } else {
        return aVal < bVal ? 1 : -1;
      }
    });

    setFilteredConfigs(result);
  }, [configs, searchText, statusFilter, sortField, sortOrder]);

  const handleCreate = () => {
    form.resetFields();
    setModalVisible(true);
  };

  const handleDelete = async (id: string) => {
    try {
      await axios.delete(`/api/configs/${id}`);
      message.success('配置已删除');
      loadConfigs();
    } catch (error: any) {
      message.error('删除配置失败');
    }
  };

  const handleSubmit = async (values: any) => {
    try {
      await axios.post('/api/configs', values);
      message.success('配置创建成功');
      setModalVisible(false);
      form.resetFields();
      loadConfigs();
    } catch (error: any) {
      message.error(error.response?.data?.error || '创建配置失败');
    }
  };

  const handleResetFilters = () => {
    setSearchText('');
    setStatusFilter('');
    setSortField('created_at');
    setSortOrder('descend');
  };

  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      width: 200,
      render: (text: string, record: Config) => (
        <Space>
          <span style={{ fontWeight: 500 }}>{text}</span>
          {!record.enabled && <Tag color="default">已禁用</Tag>}
        </Space>
      ),
    },
    {
      title: '描述',
      dataIndex: 'description',
      key: 'description',
      ellipsis: true,
      render: (text: string) => text || '-',
    },
    {
      title: 'Base URL',
      dataIndex: 'openai_base_url',
      key: 'openai_base_url',
      ellipsis: true,
      width: 250,
    },
    {
      title: 'Anthropic Token',
      dataIndex: 'anthropic_api_key',
      key: 'anthropic_api_key',
      width: 150,
      ellipsis: true,
      render: (text: string) => (
        <code style={{ fontSize: 11, background: '#f5f5f5', padding: '2px 6px', borderRadius: 3 }}>
          {text || '-'}
        </code>
      ),
    },
    {
      title: '状态',
      dataIndex: 'enabled',
      key: 'enabled',
      width: 80,
      align: 'center' as const,
      render: (enabled: boolean) => (
        <Tag color={enabled ? 'success' : 'default'}>
          {enabled ? '启用' : '禁用'}
        </Tag>
      ),
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 180,
      render: (text: string) => new Date(text).toLocaleString('zh-CN'),
    },
    {
      title: '操作',
      key: 'action',
      width: 150,
      fixed: 'right' as const,
      render: (_: any, record: Config) => (
        <Space>
          <Button
            type="link"
            icon={<EyeOutlined />}
            onClick={() => navigate(`/ui/configs/${record.id}`)}
          >
            详情
          </Button>
          <Popconfirm
            title="确定要删除这个配置吗？"
            onConfirm={() => handleDelete(record.id)}
            okText="确定"
            cancelText="取消"
          >
            <Button
              type="link"
              danger
              icon={<DeleteOutlined />}
            />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <Card>
      <Space style={{ marginBottom: 16, width: '100%', justifyContent: 'space-between' }}>
        <Title level={4} style={{ margin: 0 }}>
          配置管理
        </Title>
        <Button
          type="primary"
          icon={<PlusOutlined />}
          onClick={handleCreate}
        >
          新建配置
        </Button>
      </Space>

      {/* Filters */}
      <Card size="small" style={{ marginBottom: 16, background: '#fafafa' }}>
        <Row gutter={[16, 16]} align="middle">
          <Col flex="auto">
            <Input
              placeholder="搜索名称、描述或Base URL"
              prefix={<SearchOutlined />}
              value={searchText}
              onChange={(e) => setSearchText(e.target.value)}
              allowClear
            />
          </Col>
          <Col>
            <Select
              placeholder="状态筛选"
              value={statusFilter}
              onChange={setStatusFilter}
              style={{ width: 130 }}
              allowClear
            >
              <Option value="enabled">仅启用</Option>
              <Option value="disabled">仅禁用</Option>
            </Select>
          </Col>
          <Col>
            <Select
              value={sortField}
              onChange={setSortField}
              style={{ width: 130 }}
            >
              <Option value="created_at">按时间</Option>
              <Option value="name">按名称</Option>
            </Select>
          </Col>
          <Col>
            <Select
              value={sortOrder}
              onChange={setSortOrder}
              style={{ width: 100 }}
            >
              <Option value="descend">降序</Option>
              <Option value="ascend">升序</Option>
            </Select>
          </Col>
          <Col>
            <Button icon={<FilterOutlined />} onClick={handleResetFilters}>
              重置
            </Button>
          </Col>
          <Col>
            <Button icon={<ReloadOutlined />} onClick={loadConfigs}>
              刷新
            </Button>
          </Col>
        </Row>
        <div style={{ marginTop: 8, fontSize: 12, color: '#666' }}>
          共 {configs.length} 个配置，显示 {filteredConfigs.length} 个
        </div>
      </Card>

      <Table
        columns={columns}
        dataSource={filteredConfigs}
        rowKey="id"
        loading={loading}
        pagination={{
          pageSize: 10,
          showSizeChanger: true,
          showQuickJumper: true,
          showTotal: (total) => `共 ${total} 条`,
          pageSizeOptions: ['10', '20', '50', '100'],
        }}
        scroll={{ x: 1200 }}
      />

      <Modal
        title="新建配置"
        open={modalVisible}
        onCancel={() => setModalVisible(false)}
        onOk={() => form.submit()}
        width={600}
      >
        <Form form={form} onFinish={handleSubmit} layout="vertical">
          <Form.Item
            name="name"
            label="名称"
            rules={[{ required: true, message: '请输入名称' }]}
          >
            <Input placeholder="例如: iFlow API" />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <TextArea rows={3} placeholder="配置描述" />
          </Form.Item>
          <Form.Item
            name="anthropic_api_key"
            label="Anthropic API Token (可选)"
            help="留空将自动生成UUID，也可自定义（英文大小写、数字、下划线，最多100字符）"
          >
            <Input placeholder="留空自动生成，或输入自定义Token" maxLength={100} />
          </Form.Item>
          <Form.Item
            name="openai_api_key"
            label="OpenAI API Key"
            rules={[{ required: true, message: '请输入API Key' }]}
          >
            <Input.Password placeholder="sk-xxx" />
          </Form.Item>
          <Form.Item
            name="openai_base_url"
            label="Base URL"
            rules={[{ required: true, message: '请输入Base URL' }]}
            initialValue="https://api.openai.com/v1"
          >
            <Input placeholder="https://api.openai.com/v1" />
          </Form.Item>
          <Form.Item
            name="big_model"
            label="大模型 (Opus)"
            rules={[{ required: true, message: '请输入模型名称' }]}
            initialValue="gpt-4o"
          >
            <Input placeholder="gpt-4o" />
          </Form.Item>
          <Form.Item
            name="middle_model"
            label="中模型 (Sonnet)"
            rules={[{ required: true, message: '请输入模型名称' }]}
            initialValue="gpt-4o"
          >
            <Input placeholder="gpt-4o" />
          </Form.Item>
          <Form.Item
            name="small_model"
            label="小模型 (Haiku)"
            rules={[{ required: true, message: '请输入模型名称' }]}
            initialValue="gpt-4o-mini"
          >
            <Input placeholder="gpt-4o-mini" />
          </Form.Item>
          <Form.Item name="max_tokens_limit" label="最大Token限制" initialValue={4096}>
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="request_timeout" label="请求超时(秒)" initialValue={90}>
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
};

export default ConfigListV2;
