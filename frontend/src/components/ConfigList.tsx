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
} from 'antd';
import {
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  EyeOutlined,
} from '@ant-design/icons';
import axios from 'axios';

const { TextArea } = Input;

const { Title } = Typography;

interface Config {
  id: string;
  name: string;
  description: string;
  openai_api_key_masked: string;
  openai_base_url: string;
  big_model: string;
  middle_model: string;
  small_model: string;
  enabled: boolean;
  created_at: string;
}

const ConfigList: React.FC = () => {
  const navigate = useNavigate();
  const [configs, setConfigs] = useState<Config[]>([]);
  const [loading, setLoading] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [form] = Form.useForm();

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
      message.error('创建配置失败');
    }
  };

  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      render: (text: string, record: Config) => (
        <Space>
          {text}
          {!record.enabled && <Tag color="default">已禁用</Tag>}
        </Space>
      ),
    },
    {
      title: '描述',
      dataIndex: 'description',
      key: 'description',
      ellipsis: true,
    },
    {
      title: 'Base URL',
      dataIndex: 'openai_base_url',
      key: 'openai_base_url',
      ellipsis: true,
    },
    {
      title: '模型配置',
      key: 'models',
      render: (_: any, record: Config) => (
        <Space direction="vertical" size="small">
          <Tag>大: {record.big_model}</Tag>
          <Tag>中: {record.middle_model}</Tag>
          <Tag>小: {record.small_model}</Tag>
        </Space>
      ),
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (text: string) => new Date(text).toLocaleString('zh-CN'),
    },
    {
      title: '操作',
      key: 'action',
      width: 200,
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
      <Table
        columns={columns}
        dataSource={configs}
        rowKey="id"
        loading={loading}
        pagination={{ pageSize: 10 }}
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
            name="openai_api_key"
            label="API Key"
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

export default ConfigList;

