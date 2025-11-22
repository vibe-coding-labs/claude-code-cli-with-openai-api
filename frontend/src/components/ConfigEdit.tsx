import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { usePageTitle } from '../utils/pageTitle';
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
  Typography,
  Divider,
  DatePicker,
} from 'antd';
import { ArrowLeftOutlined, SaveOutlined } from '@ant-design/icons';
import axios from 'axios';
import ModelSelector from './ModelSelector';
import dayjs from 'dayjs';

const { TextArea } = Input;
const { Title, Paragraph } = Typography;

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
  retry_count: number;
  enabled: boolean;
  expires_at?: string;
}

const ConfigEdit: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [form] = Form.useForm();
  const [config, setConfig] = useState<Config | null>(null);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  
  usePageTitle(config ? `编辑 ${config.name}` : '编辑配置');

  useEffect(() => {
    fetchConfig();
  }, [id]);

  const fetchConfig = async () => {
    try {
      const response = await axios.get(`/api/configs/${id}`);
      const configData = response.data.config;
      setConfig(configData);
      // 处理过期时间
      const formData = {
        ...configData,
        expires_at: configData.expires_at ? dayjs(configData.expires_at) : null
      };
      form.setFieldsValue(formData);
      setLoading(false);
    } catch (error) {
      message.error('获取配置失败');
      setLoading(false);
    }
  };

  const handleSubmit = async (values: any) => {
    setSubmitting(true);
    try {
      // 处理过期时间
      const submitData = {
        ...values,
        expires_at: values.expires_at ? values.expires_at.toISOString() : null
      };
      await axios.put(`/api/configs/${id}`, submitData);
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
            tooltip="配置名称是用于标识和区分不同OpenAI API配置的显示名称。建议使用有意义的名称，例如：'生产环境API'、'测试环境API'、'iFlow转发服务'等。这个名称会在配置列表、详情页和测试页面中显示，方便您快速识别和管理多个配置。名称支持中英文、数字和特殊字符，建议长度控制在50字符以内以保证界面美观。"
          >
            <Input placeholder="例如: 生产环境配置" />
          </Form.Item>

          <Form.Item 
            name="description" 
            label="描述"
            tooltip="配置描述是可选字段，用于详细说明该配置的用途、使用场景、注意事项等信息。例如可以描述该配置连接的是哪个环境（开发/测试/生产）、使用的是哪家服务商的API、有什么特殊限制或配额说明等。良好的描述能帮助团队成员快速理解配置用途，避免误用。支持多行文本输入，建议包含：环境信息、API来源、使用限制、负责人等关键信息。"
          >
            <TextArea rows={3} placeholder="可选的配置描述" />
          </Form.Item>

          <Form.Item
            name="anthropic_api_key"
            label="Anthropic API Token"
            tooltip="Anthropic API Token是Claude CLI客户端用于识别和认证配置的唯一标识符。您可以保持原Token不变，或修改为新的自定义Token。自定义Token只能包含英文大小写字母、数字和下划线，长度不超过100字符。此Token会配置在Claude Desktop或CLI的配置文件中，作为ANTHROPIC_API_KEY使用。自定义Token示例：'dev_api_2024'、'production_key'等。注意：Token必须在系统中保持唯一，不能与其他配置重复。修改Token后需要同步更新Claude配置文件。"
            help="当前Token显示在详情页，修改需确保唯一性（英文大小写、数字、下划线，最多100字符）"
          >
            <Input placeholder="修改Anthropic API Token" maxLength={100} />
          </Form.Item>

          <Form.Item
            name="openai_api_key"
            label="OpenAI API Key"
            tooltip="OpenAI API Key是调用OpenAI或兼容API服务的认证密钥，通常以'sk-'开头。此密钥将被加密存储在数据库中，用于实际调用大模型API。支持使用：1) 官方OpenAI API密钥；2) 国内转发服务提供的密钥；3) 任何OpenAI兼容接口的API Key（如Azure OpenAI、本地部署的模型服务等）。请妥善保管您的API Key，不要泄露给他人。密钥在保存后将以掩码形式显示（如：sk-***abc），确保安全性。编辑时留空则保持原密钥不变，只有输入新值时才会更新。"
            help="留空则不修改现有密钥"
          >
            <Input.Password placeholder="sk-..." />
          </Form.Item>

          <Form.Item
            name="openai_base_url"
            label="OpenAI Base URL"
            rules={[{ required: true, message: '请输入Base URL' }]}
            tooltip="Base URL是OpenAI API的基础访问地址，系统会在此URL后拼接具体的API端点（如/chat/completions）来调用模型服务。使用场景：1) 官方OpenAI服务使用默认值'https://api.openai.com/v1'；2) 使用国内API转发服务时填写转发服务的地址；3) 使用Azure OpenAI时填写Azure的endpoint；4) 使用本地部署的模型服务时填写本地地址（如'http://localhost:8000/v1'）。注意URL格式必须正确，通常以'/v1'结尾，不要包含尾部斜杠。"
          >
            <Input placeholder="https://api.openai.com/v1" />
          </Form.Item>

          <Form.Item
            name="big_model"
            label="大模型 (Opus)"
            rules={[{ required: true, message: '请输入大模型名称' }]}
            tooltip="大模型映射配置用于将Claude Desktop/CLI请求的claude-opus-4-20250514模型转换为您指定的OpenAI模型。Claude Opus系列是Anthropic最强大的模型，适合复杂推理、长文本分析等高难度任务。您可以映射到OpenAI的高性能模型，如gpt-4o、gpt-4-turbo等。也可以映射到其他服务商的旗舰模型。模型名称必须与您的API服务支持的模型完全一致，否则调用会失败。建议选择性能强、上下文长度大的模型以保证使用体验。"
          >
            <Input placeholder="gpt-4o" />
          </Form.Item>

          <Form.Item
            name="middle_model"
            label="中模型 (Sonnet)"
            rules={[{ required: true, message: '请输入中模型名称' }]}
            tooltip="中模型映射配置用于将Claude Desktop/CLI请求的claude-sonnet-4-20250514模型转换为您指定的OpenAI模型。Claude Sonnet系列在性能和成本之间取得平衡，适合日常编程辅助、代码生成、文档编写等常规任务。您可以映射到gpt-4o、gpt-4等中高端模型，也可以根据成本考虑映射到gpt-3.5-turbo。模型选择建议：如果预算充足选择gpt-4o以获得最佳体验；如果注重性价比可选择gpt-3.5-turbo-16k等模型。确保模型名称准确无误。"
          >
            <Input placeholder="gpt-4o" />
          </Form.Item>

          <Form.Item
            name="small_model"
            label="小模型 (Haiku)"
            rules={[{ required: true, message: '请输入小模型名称' }]}
            tooltip="小模型映射配置用于将Claude Desktop/CLI请求的claude-haiku-4-20250514模型转换为您指定的OpenAI模型。Claude Haiku系列是轻量级快速模型，适合简单问答、代码补全、语法检查等低延迟场景。建议映射到gpt-4o-mini、gpt-3.5-turbo等经济型模型以控制成本。这类请求通常频繁但简单，选择响应速度快、价格低廉的模型即可满足需求。如果您的API服务有速率限制，小模型的低成本特性能让您获得更高的调用配额。注意验证模型在目标服务中的可用性。"
          >
            <Input placeholder="gpt-4o-mini" />
          </Form.Item>

          <Divider />

          <Title level={5}>支持的模型列表</Title>
          <Paragraph type="secondary" style={{ marginBottom: 16 }}>
            配置此 OpenAI API 支持的所有模型，可用于测试和查询。如不配置，将使用上方映射的模型作为默认列表。
            <br />
            💡 <strong>可以直接输入任意模型名称</strong>，从下拉列表快速选择常用模型，或输入自定义模型后按回车添加。
          </Paragraph>

          <Form.Item
            name="supported_models"
            label="模型列表"
            tooltip="支持的模型列表定义了此OpenAI API配置可以调用的所有模型。在测试页面中，您可以从这个列表中选择模型进行测试。如果不配置此列表，系统将使用上方三个模型映射（大中小模型）作为默认可用模型。您可以：1) 从下拉列表快速选择常用模型（GPT、Claude、Gemini等）；2) 直接输入任意自定义模型名称并按回车添加；3) 使用逗号分隔批量输入多个模型。此配置不影响实际的模型映射调用，仅用于测试和查询场景，让您能够验证API对不同模型的支持情况。"
          >
            <ModelSelector />
          </Form.Item>

          <Divider />

          <Form.Item
            name="max_tokens_limit"
            label="最大Token限制"
            rules={[{ required: true, message: '请输入最大Token限制' }]}
            tooltip="最大Token限制控制单次API请求中生成内容的最大token数量。Token是模型处理文本的基本单位，1个token约等于0.75个英文单词或1-2个中文字符。此参数影响：1) 响应长度 - 限制模型生成内容的最大长度；2) 成本控制 - 较小的限制可以降低API调用费用；3) 响应速度 - 较小的限制通常意味着更快的响应。建议值：日常对话4096、代码生成8192-16384、长文本分析32768。现代模型（如GPT-4o、Claude 3.5）支持128k+的上下文，默认16384可满足大多数场景。注意：此值不能超过模型本身的上下文窗口限制，也要考虑输入tokens的占用。设置过小可能导致回答被截断，过大则增加成本和延迟。"
          >
            <InputNumber min={1} max={1000000} style={{ width: '100%' }} />
          </Form.Item>

          <Form.Item
            name="request_timeout"
            label="请求超时时间（秒）"
            rules={[{ required: true, message: '请输入请求超时时间' }]}
            tooltip="请求超时时间定义了等待API响应的最长时间，超过此时间未收到响应则请求失败。合理设置超时时间能够：1) 避免无限等待 - 当API服务异常时及时终止请求；2) 提升用户体验 - 防止Claude CLI长时间无响应；3) 资源管理 - 释放被挂起的连接资源。建议值：简单请求60-90秒、复杂推理120-180秒、长文本/代码生成180-300秒。默认180秒（3分钟）可以应对大部分复杂场景，包括长代码生成和深度分析任务。注意：设置过短可能导致正常的长响应被中断，设置过长则可能在网络故障时浪费时间。最大支持600秒（10分钟）。"
          >
            <InputNumber min={1} max={600} style={{ width: '100%' }} />
          </Form.Item>

          <Form.Item
            name="retry_count"
            label="失败重试次数"
            rules={[{ required: true, message: '请输入失败重试次数' }]}
            initialValue={3}
            tooltip="失败重试次数定义了当OpenAI API调用失败时，系统自动重试的次数。合理的重试机制能够：1) 提高成功率 - 应对网络抖动或API临时故障；2) 改善用户体验 - 避免因偶发错误导致请求失败；3) 节省成本 - 减少因临时问题导致的手动重试。建议值：稳定环境3次、不稳定网络5次、高可用需求5-10次。默认3次可以应对大多数场景。注意：设置过少可能导致偶发故障影响使用，设置过多则可能在API持续故障时增加响应延迟。每次重试之间会有短暂延迟（指数退避），最大支持100次重试。"
          >
            <InputNumber min={1} max={100} style={{ width: '100%' }} placeholder="默认为3次" />
          </Form.Item>

          <Form.Item
            name="expires_at"
            label="API密钥过期时间"
            tooltip="API密钥过期时间是可选配置，用于管理临时API密钥的生命周期。当设置了过期时间后，系统会在认证时检查密钥是否已过期，过期的密钥将无法使用。使用场景：1) 临时合作项目 - 为外部合作者设置有限期访问；2) 测试环境 - 为测试密钥设置自动过期；3) 安全管理 - 定期轮换API密钥增强安全性。如果不设置，密钥将永久有效。建议对临时密钥设置合理的过期时间，避免长期暴露风险。"
            help="可选，设置后密钥将在指定时间后自动过期无法使用"
          >
            <DatePicker 
              showTime 
              format="YYYY-MM-DD HH:mm:ss"
              style={{ width: '100%' }}
              placeholder="选择过期时间（可选）"
            />
          </Form.Item>

          <Form.Item
            name="enabled"
            label="启用配置"
            valuePropName="checked"
            tooltip="启用配置开关控制此OpenAI API配置是否可以被Claude CLI使用。当启用时（开关打开），此配置可以正常响应Claude CLI的请求；当禁用时（开关关闭），使用此配置的Anthropic API Token进行的请求将被拒绝。使用场景：1) 临时停用某个配置而不删除它；2) 在配置出现问题时快速禁用以防止错误调用；3) 管理多个配置时选择性启用需要的配置。禁用配置不会删除任何数据，您可以随时重新启用。建议在修改配置参数后先禁用、测试无误后再启用，以避免影响正在使用的Claude CLI客户端。"
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
