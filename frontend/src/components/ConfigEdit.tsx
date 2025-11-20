import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Card,
  Form,
  Input,
  InputNumber,
  Switch,
  Button,
  Space,
  message,
  Spin,
} from 'antd';
import { ArrowLeftOutlined, SaveOutlined } from '@ant-design/icons';
import axios from 'axios';

const { TextArea } = Input;

interface Config {
  id: string;
  name: string;
  description: string;
  openai_base_url: string;
  big_model: string;
  middle_model: string;
  small_model: string;
  max_tokens_limit: number;
  request_timeout: number;
  enabled: boolean;
}

const ConfigEdit: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    fetchConfig();
  }, [id]);

  const fetchConfig = async () => {
    try {
      const response = await axios.get(`/api/configs/${id}`);
      const config = response.data.config;
      form.setFieldsValue(config);
      setLoading(false);
    } catch (error) {
      message.error('获取配置失败');
      setLoading(false);
    }
  };

  const handleSubmit = async (values: any) => {
    setSubmitting(true);
    try {
      await axios.put(`/api/configs/${id}`, values);
      message.success('保存成功');
      navigate(`/ui/configs/${id}`);
    } catch (error: any) {
      message.error(error.response?.data?.error || '保存失败');
    } finally {
      setSubmitting(false);
    }
  };

  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: 50 }}>
        <Spin size="large" />
      </div>
    );
  }

  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <Button
          icon={<ArrowLeftOutlined />}
          onClick={() => navigate(`/ui/configs/${id}`)}
        >
          返回
        </Button>
      </Space>

      <Card title="编辑配置">
        <Form
          form={form}
          layout="vertical"
          onFinish={handleSubmit}
          style={{ maxWidth: 800 }}
        >
          <Form.Item
            name="name"
            label="配置名称"
            rules={[{ required: true, message: '请输入配置名称' }]}
          >
            <Input placeholder="例如: 生产环境配置" />
          </Form.Item>

          <Form.Item name="description" label="描述">
            <TextArea rows={3} placeholder="可选的配置描述" />
          </Form.Item>

          <Form.Item
            name="anthropic_api_key"
            label="Anthropic API Token"
            help="当前Token显示在详情页，修改需确保唯一性（英文大小写、数字、下划线，最多100字符）"
          >
            <Input placeholder="修改Anthropic API Token" maxLength={100} />
          </Form.Item>

          <Form.Item
            name="openai_api_key"
            label="OpenAI API Key"
            help="留空则不修改现有密钥"
          >
            <Input.Password placeholder="sk-..." />
          </Form.Item>

          <Form.Item
            name="openai_base_url"
            label="OpenAI Base URL"
            rules={[{ required: true, message: '请输入Base URL' }]}
          >
            <Input placeholder="https://api.openai.com/v1" />
          </Form.Item>

          <Form.Item
            name="big_model"
            label="大模型 (Opus)"
            rules={[{ required: true, message: '请输入大模型名称' }]}
          >
            <Input placeholder="gpt-4o" />
          </Form.Item>

          <Form.Item
            name="middle_model"
            label="中模型 (Sonnet)"
            rules={[{ required: true, message: '请输入中模型名称' }]}
          >
            <Input placeholder="gpt-4o" />
          </Form.Item>

          <Form.Item
            name="small_model"
            label="小模型 (Haiku)"
            rules={[{ required: true, message: '请输入小模型名称' }]}
          >
            <Input placeholder="gpt-4o-mini" />
          </Form.Item>

          <Form.Item
            name="max_tokens_limit"
            label="最大Token限制"
            rules={[{ required: true, message: '请输入最大Token限制' }]}
          >
            <InputNumber min={1} max={1000000} style={{ width: '100%' }} />
          </Form.Item>

          <Form.Item
            name="request_timeout"
            label="请求超时时间（秒）"
            rules={[{ required: true, message: '请输入请求超时时间' }]}
          >
            <InputNumber min={1} max={300} style={{ width: '100%' }} />
          </Form.Item>

          <Form.Item
            name="enabled"
            label="启用配置"
            valuePropName="checked"
          >
            <Switch />
          </Form.Item>

          <Form.Item>
            <Space>
              <Button
                type="primary"
                htmlType="submit"
                loading={submitting}
                icon={<SaveOutlined />}
              >
                保存
              </Button>
              <Button onClick={() => navigate(`/ui/configs/${id}`)}>
                取消
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Card>
    </div>
  );
};

export default ConfigEdit;
