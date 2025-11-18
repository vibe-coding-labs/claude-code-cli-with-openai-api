import React, { useEffect } from 'react';
import {
  Modal,
  Form,
  Input,
  InputNumber,
  Switch,
  Button,
  Space,
  Row,
  Col,
  message,
  Tooltip,
} from 'antd';
import { QuestionCircleOutlined } from '@ant-design/icons';
import { configAPI } from '../services/api';
import { APIConfig, APIConfigRequest } from '../types/api';

interface ConfigModalProps {
  visible: boolean;
  config: APIConfig | null;
  onCancel: () => void;
  onSuccess: () => void;
}

const ConfigModal: React.FC<ConfigModalProps> = ({
  visible,
  config,
  onCancel,
  onSuccess,
}) => {
  const [form] = Form.useForm();

  // 辅助函数：生成带问号提示的标签
  const renderLabelWithTooltip = (label: string, tooltip: string) => (
    <Space>
      <span>{label}</span>
      <Tooltip title={tooltip}>
        <QuestionCircleOutlined style={{ color: '#8c8c8c', cursor: 'help' }} />
      </Tooltip>
    </Space>
  );

  useEffect(() => {
    if (visible) {
      if (config) {
        form.setFieldsValue({
          ...config,
          custom_headers: config.custom_headers
            ? Object.entries(config.custom_headers)
                .map(([key, value]) => `${key}: ${value}`)
                .join('\n')
            : '',
        });
      } else {
        form.resetFields();
        form.setFieldsValue({
          enabled: true,
          openai_base_url: 'https://api.openai.com/v1',
          big_model: 'gpt-4',
          middle_model: 'gpt-3.5-turbo',
          small_model: 'gpt-3.5-turbo',
          max_tokens_limit: 4096,
          min_tokens_limit: 1,
          request_timeout: 60,
        });
      }
    }
  }, [visible, config, form]);

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      const customHeaders: Record<string, string> = {};
      if (values.custom_headers) {
        values.custom_headers.split('\n').forEach((line: string) => {
          const [key, ...valueParts] = line.split(':');
          if (key && valueParts.length > 0) {
            customHeaders[key.trim()] = valueParts.join(':').trim();
          }
        });
      }

      const request: APIConfigRequest = {
        name: values.name,
        description: values.description,
        openai_api_key: values.openai_api_key,
        openai_base_url: values.openai_base_url,
        azure_api_version: values.azure_api_version,
        anthropic_api_key: values.anthropic_api_key,
        big_model: values.big_model,
        middle_model: values.middle_model,
        small_model: values.small_model,
        max_tokens_limit: values.max_tokens_limit,
        min_tokens_limit: values.min_tokens_limit,
        request_timeout: values.request_timeout,
        custom_headers: Object.keys(customHeaders).length > 0 ? customHeaders : undefined,
        enabled: values.enabled !== false,
      };

      if (config) {
        await configAPI.updateConfig(config.id, request);
        message.success('配置已更新');
      } else {
        await configAPI.createConfig(request);
        message.success('配置已创建');
      }
      onSuccess();
    } catch (error: any) {
      if (error.errorFields) {
        message.error('请检查表单输入');
      } else {
        message.error('操作失败: ' + (error.response?.data?.error?.message || error.message));
      }
    }
  };

  return (
    <Modal
      title={config ? '编辑配置' : '新建配置'}
      open={visible}
      onCancel={onCancel}
      footer={[
        <Button key="cancel" onClick={onCancel}>
          取消
        </Button>,
        <Button key="submit" type="primary" onClick={handleSubmit}>
          保存
        </Button>,
      ]}
      width={800}
    >
      <Form
        form={form}
        layout="vertical"
        initialValues={{
          enabled: true,
          openai_base_url: 'https://api.openai.com/v1',
          big_model: 'gpt-4',
          middle_model: 'gpt-3.5-turbo',
          small_model: 'gpt-3.5-turbo',
          max_tokens_limit: 4096,
          min_tokens_limit: 1,
          request_timeout: 60,
        }}
      >
        <Row gutter={16}>
          <Col span={12}>
            <Form.Item
              name="name"
              label={renderLabelWithTooltip(
                '配置名称',
                '用于标识此配置的唯一名称，建议使用有意义的名称，如"OpenAI Production"或"Azure Development"'
              )}
              rules={[{ required: true, message: '请输入配置名称' }]}
            >
              <Input placeholder="例如: OpenAI Production" />
            </Form.Item>
          </Col>
          <Col span={12}>
            <Form.Item
              name="enabled"
              label={renderLabelWithTooltip(
                '启用',
                '控制此配置是否生效。禁用的配置不会被使用，但仍会保留在系统中'
              )}
              valuePropName="checked"
            >
              <Switch />
            </Form.Item>
          </Col>
        </Row>

        <Form.Item
          name="description"
          label={renderLabelWithTooltip(
            '描述',
            '配置的详细描述信息，用于记录配置的用途、使用场景等（可选）'
          )}
        >
          <Input.TextArea rows={2} placeholder="配置描述（可选）" />
        </Form.Item>

        <Row gutter={16}>
          <Col span={12}>
            <Form.Item
              name="openai_api_key"
              label={renderLabelWithTooltip(
                'OpenAI API Key',
                'OpenAI API的密钥，格式通常以"sk-"开头。此密钥用于调用OpenAI或兼容OpenAI API的服务'
              )}
              rules={[{ required: true, message: '请输入OpenAI API Key' }]}
            >
              <Input.Password placeholder="sk-..." />
            </Form.Item>
          </Col>
          <Col span={12}>
            <Form.Item
              name="openai_base_url"
              label={renderLabelWithTooltip(
                'OpenAI Base URL',
                'OpenAI API的基础URL地址。默认值为"https://api.openai.com/v1"。如果使用Azure OpenAI或其他兼容服务，请修改为对应的API地址'
              )}
              rules={[{ required: true, message: '请输入Base URL' }]}
            >
              <Input placeholder="https://api.openai.com/v1" />
            </Form.Item>
          </Col>
        </Row>

        <Form.Item
          name="azure_api_version"
          label={renderLabelWithTooltip(
            'Azure API Version (可选)',
            '如果使用Azure OpenAI服务，需要指定API版本号，例如"2023-05-15"。如果使用标准的OpenAI API，可以留空'
          )}
        >
          <Input placeholder="例如: 2023-05-15" />
        </Form.Item>

        <Form.Item
          name="anthropic_api_key"
          label={renderLabelWithTooltip(
            'Anthropic API Key (可选，用于Claude Code CLI认证)',
            '用于Claude Code CLI客户端认证的Anthropic API密钥。此密钥用于客户端与代理服务器之间的身份验证，不是用于调用OpenAI API的密钥'
          )}
        >
          <Input.Password placeholder="用于客户端认证的API Key" />
        </Form.Item>

        <Row gutter={16}>
          <Col span={8}>
            <Form.Item
              name="big_model"
              label={renderLabelWithTooltip(
                '大模型',
                '用于处理复杂任务的大模型名称，例如"gpt-4"、"gpt-4-turbo"等。大模型通常具有更强的推理能力，但成本较高'
              )}
              rules={[{ required: true, message: '请输入大模型名称' }]}
            >
              <Input placeholder="gpt-4" />
            </Form.Item>
          </Col>
          <Col span={8}>
            <Form.Item
              name="middle_model"
              label={renderLabelWithTooltip(
                '中模型',
                '用于处理中等复杂度任务的中型模型名称，例如"gpt-3.5-turbo"。中模型在性能和成本之间取得平衡'
              )}
              rules={[{ required: true, message: '请输入中模型名称' }]}
            >
              <Input placeholder="gpt-3.5-turbo" />
            </Form.Item>
          </Col>
          <Col span={8}>
            <Form.Item
              name="small_model"
              label={renderLabelWithTooltip(
                '小模型',
                '用于处理简单任务的小模型名称，通常与中模型相同，例如"gpt-3.5-turbo"。小模型响应快速且成本较低'
              )}
              rules={[{ required: true, message: '请输入小模型名称' }]}
            >
              <Input placeholder="gpt-3.5-turbo" />
            </Form.Item>
          </Col>
        </Row>

        <Row gutter={16}>
          <Col span={8}>
            <Form.Item
              name="max_tokens_limit"
              label={renderLabelWithTooltip(
                '最大Token限制',
                '单个请求允许的最大Token数量。此限制用于控制API调用的成本，防止生成过长的响应。建议值：4096（GPT-3.5）或8192（GPT-4）'
              )}
              rules={[{ required: true, message: '请输入最大Token限制' }]}
            >
              <InputNumber min={1} max={100000} style={{ width: '100%' }} />
            </Form.Item>
          </Col>
          <Col span={8}>
            <Form.Item
              name="min_tokens_limit"
              label={renderLabelWithTooltip(
                '最小Token限制',
                '单个请求允许的最小Token数量。通常设置为1，用于确保请求的有效性'
              )}
              rules={[{ required: true, message: '请输入最小Token限制' }]}
            >
              <InputNumber min={1} max={100000} style={{ width: '100%' }} />
            </Form.Item>
          </Col>
          <Col span={8}>
            <Form.Item
              name="request_timeout"
              label={renderLabelWithTooltip(
                '请求超时（秒）',
                'API请求的超时时间（秒）。如果请求在此时间内未完成，将被取消。建议值：60秒。可根据网络情况和模型响应速度调整'
              )}
              rules={[{ required: true, message: '请输入请求超时时间' }]}
            >
              <InputNumber min={1} max={600} style={{ width: '100%' }} />
            </Form.Item>
          </Col>
        </Row>

        <Form.Item
          name="custom_headers"
          label={renderLabelWithTooltip(
            '自定义请求头（可选，每行一个，格式: Key: Value）',
            '自定义HTTP请求头，每行一个，格式为"Key: Value"。例如：\nX-Custom-Header: value1\nAuthorization: Bearer token\n这些请求头会在调用API时自动添加到请求中'
          )}
        >
          <Input.TextArea
            rows={4}
            placeholder="例如:&#10;X-Custom-Header: value1&#10;Authorization: Bearer token"
          />
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default ConfigModal;

