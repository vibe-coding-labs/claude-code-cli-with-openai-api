import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Card,
  Button,
  Space,
  Input,
  Spin,
  message,
  Form,
  Select,
  InputNumber,
  Switch,
  Typography,
  Divider,
  Tag,
} from 'antd';
import {
  ArrowLeftOutlined,
  SendOutlined,
  ClearOutlined,
} from '@ant-design/icons';
import axios from 'axios';

const { TextArea } = Input;
const { Title, Paragraph, Text } = Typography;
const { Option } = Select;

interface Config {
  id: string;
  name: string;
  anthropic_api_key?: string;
}

interface TestResponse {
  status: string;
  model: string;
  response: string;
  usage: {
    input_tokens: number;
    output_tokens: number;
    total_tokens: number;
  };
  duration_ms: number;
  error?: string;
}

const ConfigTest: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [config, setConfig] = useState<Config | null>(null);
  const [loading, setLoading] = useState(true);
  const [testing, setTesting] = useState(false);
  const [response, setResponse] = useState<TestResponse | null>(null);
  const [form] = Form.useForm();

  useEffect(() => {
    fetchConfig();
  }, [id]);

  const fetchConfig = async () => {
    try {
      const res = await axios.get(`/api/configs/${id}`);
      setConfig(res.data.config);
      setLoading(false);
    } catch (error) {
      message.error('获取配置失败');
      setLoading(false);
    }
  };

  const handleTest = async (values: any) => {
    setTesting(true);
    setResponse(null);
    
    try {
      const testData = {
        model: values.model,
        max_tokens: values.max_tokens,
        temperature: values.temperature,
        message: values.message,
      };
      
      const res = await axios.post(`/api/configs/${id}/test`, testData);
      setResponse(res.data);
      
      if (res.data.status === 'success') {
        message.success('测试成功！');
      } else {
        message.error(`测试失败: ${res.data.error}`);
      }
    } catch (error: any) {
      message.error(`测试失败: ${error.response?.data?.error || error.message}`);
      setResponse({
        status: 'error',
        model: values.model,
        response: '',
        usage: { input_tokens: 0, output_tokens: 0, total_tokens: 0 },
        duration_ms: 0,
        error: error.response?.data?.error || error.message,
      });
    } finally {
      setTesting(false);
    }
  };

  const handleClear = () => {
    form.resetFields(['message']);
    setResponse(null);
  };

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

  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <Button
          icon={<ArrowLeftOutlined />}
          onClick={() => navigate(`/ui/configs/${id}`)}
        >
          返回配置详情
        </Button>
      </Space>

      <Card title={`在线测试 - ${config.name}`}>
        <Form
          form={form}
          onFinish={handleTest}
          layout="vertical"
          initialValues={{
            model: 'claude-3-5-sonnet-20241022',
            max_tokens: 1024,
            temperature: 1.0,
            stream: false,
          }}
        >
          <Form.Item
            label="模型"
            name="model"
            rules={[{ required: true, message: '请选择模型' }]}
          >
            <Select>
              <Option value="claude-3-opus-20240229">Claude 3 Opus</Option>
              <Option value="claude-3-5-sonnet-20241022">Claude 3.5 Sonnet</Option>
              <Option value="claude-3-5-haiku-20241022">Claude 3.5 Haiku</Option>
              <Option value="claude-3-haiku-20240307">Claude 3 Haiku</Option>
            </Select>
          </Form.Item>

          <Form.Item
            label="Max Tokens"
            name="max_tokens"
            rules={[{ required: true, message: '请输入最大token数' }]}
          >
            <InputNumber min={1} max={8192} style={{ width: '100%' }} />
          </Form.Item>

          <Form.Item
            label="Temperature"
            name="temperature"
            extra="控制输出随机性，范围 0-1"
          >
            <InputNumber min={0} max={1} step={0.1} style={{ width: '100%' }} />
          </Form.Item>

          <Form.Item
            label="测试消息"
            name="message"
            rules={[{ required: true, message: '请输入测试消息' }]}
          >
            <TextArea
              rows={6}
              placeholder="输入您想要测试的消息内容..."
              showCount
            />
          </Form.Item>

          <Form.Item>
            <Space>
              <Button
                type="primary"
                htmlType="submit"
                icon={<SendOutlined />}
                loading={testing}
              >
                发送测试
              </Button>
              <Button
                icon={<ClearOutlined />}
                onClick={handleClear}
              >
                清空
              </Button>
            </Space>
          </Form.Item>
        </Form>

        {response && (
          <>
            <Divider />
            <Title level={4}>测试结果</Title>
            
            <Space direction="vertical" style={{ width: '100%' }} size="large">
              <div>
                <Text strong>状态：</Text>
                <Tag color={response.status === 'success' ? 'success' : 'error'} style={{ marginLeft: 8 }}>
                  {response.status === 'success' ? '成功' : '失败'}
                </Tag>
              </div>

              {response.status === 'success' && (
                <>
                  <div>
                    <Text strong>模型：</Text>
                    <Text style={{ marginLeft: 8 }}>{response.model}</Text>
                  </div>

                  <div>
                    <Text strong>响应时间：</Text>
                    <Text style={{ marginLeft: 8 }}>{response.duration_ms} ms</Text>
                  </div>

                  <div>
                    <Text strong>Token 使用：</Text>
                    <div style={{ marginLeft: 8, marginTop: 8 }}>
                      <Tag>输入: {response.usage.input_tokens}</Tag>
                      <Tag>输出: {response.usage.output_tokens}</Tag>
                      <Tag>总计: {response.usage.total_tokens}</Tag>
                    </div>
                  </div>

                  <div>
                    <Text strong>响应内容：</Text>
                    <Card style={{ marginTop: 8, backgroundColor: '#f5f5f5' }}>
                      <Paragraph style={{ whiteSpace: 'pre-wrap', marginBottom: 0 }}>
                        {response.response}
                      </Paragraph>
                    </Card>
                  </div>
                </>
              )}

              {response.error && (
                <div>
                  <Text strong style={{ color: 'red' }}>错误信息：</Text>
                  <Card style={{ marginTop: 8, backgroundColor: '#fff2f0', borderColor: '#ffccc7' }}>
                    <Text style={{ color: 'red' }}>{response.error}</Text>
                  </Card>
                </div>
              )}
            </Space>
          </>
        )}
      </Card>
    </div>
  );
};

export default ConfigTest;
