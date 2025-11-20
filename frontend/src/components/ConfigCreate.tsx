import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Form,
  Input,
  InputNumber,
  Button,
  Card,
  message,
  Space,
  Typography,
  Divider,
} from 'antd';
import { ArrowLeftOutlined, SaveOutlined } from '@ant-design/icons';
import axios from 'axios';
import ModelSelector from './ModelSelector';

const { TextArea } = Input;
const { Title, Paragraph } = Typography;

const ConfigCreate: React.FC = () => {
  const navigate = useNavigate();
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (values: any) => {
    setLoading(true);
    try {
      const response = await axios.post('/api/configs', values);
      message.success('配置创建成功');
      // 跳转到新创建的配置详情页
      navigate(`/ui/configs/${response.data.id}`);
    } catch (error: any) {
      message.error(error.response?.data?.error || '创建配置失败');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div style={{ padding: '24px', maxWidth: 1000, margin: '0 auto' }}>
      <Card>
        <Space style={{ marginBottom: 24 }}>
          <Button
            icon={<ArrowLeftOutlined />}
            onClick={() => navigate('/ui')}
          >
            返回列表
          </Button>
        </Space>

        <Title level={3}>新建配置</Title>
        <Paragraph type="secondary">
          创建一个新的 OpenAI API 配置，用于 Claude Code CLI 的调用。
        </Paragraph>

        <Divider />

        <Form
          form={form}
          onFinish={handleSubmit}
          layout="vertical"
          size="large"
        >
          <Title level={5}>基本信息</Title>
          <Form.Item
            name="name"
            label="配置名称"
            rules={[{ required: true, message: '请输入配置名称' }]}
            tooltip="配置名称是用于标识和区分不同OpenAI API配置的显示名称。建议使用有意义的名称，例如：'生产环境API'、'测试环境API'、'iFlow转发服务'等。这个名称会在配置列表、详情页和测试页面中显示，方便您快速识别和管理多个配置。名称支持中英文、数字和特殊字符，建议长度控制在50字符以内以保证界面美观。"
          >
            <Input placeholder="例如: iFlow API" />
          </Form.Item>

          <Form.Item
            name="description"
            label="配置描述"
            tooltip="配置描述是可选字段，用于详细说明该配置的用途、使用场景、注意事项等信息。例如可以描述该配置连接的是哪个环境（开发/测试/生产）、使用的是哪家服务商的API、有什么特殊限制或配额说明等。良好的描述能帮助团队成员快速理解配置用途，避免误用。支持多行文本输入，建议包含：环境信息、API来源、使用限制、负责人等关键信息。"
          >
            <TextArea rows={3} placeholder="配置描述，例如：用于开发环境的 API 配置" />
          </Form.Item>

          <Form.Item
            name="anthropic_api_key"
            label="Anthropic API Token"
            tooltip="Anthropic API Token是Claude CLI客户端用于识别和认证配置的唯一标识符。留空时系统会自动生成UUID格式的Token；也可以自定义便于记忆的Token名称，只能包含英文大小写字母、数字和下划线，长度不超过100字符。此Token会配置在Claude Desktop或CLI的配置文件中，作为ANTHROPIC_API_KEY使用。自定义Token示例：'dev_api_2024'、'production_key'等。注意：Token必须在系统中保持唯一，不能与其他配置重复。"
            help="这是 Claude CLI 用于识别配置的唯一标识，会在 Claude 配置文件中使用"
          >
            <Input 
              placeholder="留空自动生成，或输入自定义Token（例如：my_custom_token_123）" 
              maxLength={100}
            />
          </Form.Item>

          <Divider />

          <Title level={5}>OpenAI API 配置</Title>
          <Form.Item
            name="openai_api_key"
            label="OpenAI API Key"
            rules={[{ required: true, message: '请输入 OpenAI API Key' }]}
            tooltip="OpenAI API Key是调用OpenAI或兼容API服务的认证密钥，通常以'sk-'开头。此密钥将被加密存储在数据库中，用于实际调用大模型API。支持使用：1) 官方OpenAI API密钥；2) 国内转发服务提供的密钥；3) 任何OpenAI兼容接口的API Key（如Azure OpenAI、本地部署的模型服务等）。请妥善保管您的API Key，不要泄露给他人。密钥在保存后将以掩码形式显示（如：sk-***abc），确保安全性。修改配置时留空则保持原密钥不变。"
          >
            <Input.Password placeholder="sk-xxx" />
          </Form.Item>

          <Form.Item
            name="openai_base_url"
            label="Base URL"
            rules={[{ required: true, message: '请输入 Base URL' }]}
            initialValue="https://api.openai.com/v1"
            tooltip="Base URL是OpenAI API的基础访问地址，系统会在此URL后拼接具体的API端点（如/chat/completions）来调用模型服务。使用场景：1) 官方OpenAI服务使用默认值'https://api.openai.com/v1'；2) 使用国内API转发服务时填写转发服务的地址；3) 使用Azure OpenAI时填写Azure的endpoint；4) 使用本地部署的模型服务时填写本地地址（如'http://localhost:8000/v1'）。注意URL格式必须正确，通常以'/v1'结尾，不要包含尾部斜杠。"
          >
            <Input placeholder="https://api.openai.com/v1" />
          </Form.Item>

          <Divider />

          <Title level={5}>模型映射</Title>
          <Paragraph type="secondary" style={{ marginBottom: 16 }}>
            配置 Claude 模型到 OpenAI 模型的映射关系
          </Paragraph>

          <Form.Item
            name="big_model"
            label="大模型 (claude-opus-4-20250514)"
            rules={[{ required: true, message: '请输入模型名称' }]}
            initialValue="gpt-4o"
            tooltip="大模型映射配置用于将Claude Desktop/CLI请求的claude-opus-4-20250514模型转换为您指定的OpenAI模型。Claude Opus系列是Anthropic最强大的模型，适合复杂推理、长文本分析等高难度任务。您可以映射到OpenAI的高性能模型，如gpt-4o、gpt-4-turbo等。也可以映射到其他服务商的旗舰模型。模型名称必须与您的API服务支持的模型完全一致，否则调用会失败。建议选择性能强、上下文长度大的模型以保证使用体验。"
          >
            <Input placeholder="gpt-4o" />
          </Form.Item>

          <Form.Item
            name="middle_model"
            label="中模型 (claude-sonnet-4-20250514)"
            rules={[{ required: true, message: '请输入模型名称' }]}
            initialValue="gpt-4o"
            tooltip="中模型映射配置用于将Claude Desktop/CLI请求的claude-sonnet-4-20250514模型转换为您指定的OpenAI模型。Claude Sonnet系列在性能和成本之间取得平衡，适合日常编程辅助、代码生成、文档编写等常规任务。您可以映射到gpt-4o、gpt-4等中高端模型，也可以根据成本考虑映射到gpt-3.5-turbo。模型选择建议：如果预算充足选择gpt-4o以获得最佳体验；如果注重性价比可选择gpt-3.5-turbo-16k等模型。确保模型名称准确无误。"
          >
            <Input placeholder="gpt-4o" />
          </Form.Item>

          <Form.Item
            name="small_model"
            label="小模型 (claude-haiku-4-20250514)"
            rules={[{ required: true, message: '请输入模型名称' }]}
            initialValue="gpt-4o-mini"
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

          <Title level={5}>高级选项</Title>

          <Form.Item
            name="max_tokens_limit"
            label="最大 Token 限制"
            initialValue={16384}
            tooltip="最大Token限制控制单次API请求中生成内容的最大token数量。Token是模型处理文本的基本单位，1个token约等于0.75个英文单词或1-2个中文字符。此参数影响：1) 响应长度 - 限制模型生成内容的最大长度；2) 成本控制 - 较小的限制可以降低API调用费用；3) 响应速度 - 较小的限制通常意味着更快的响应。建议值：日常对话4096、代码生成8192-16384、长文本分析32768。现代模型（如GPT-4o、Claude 3.5）支持128k+的上下文，默认16384可满足大多数场景。注意：此值不能超过模型本身的上下文窗口限制，也要考虑输入tokens的占用。设置过小可能导致回答被截断，过大则增加成本和延迟。"
          >
            <InputNumber min={1} max={1000000} style={{ width: '100%' }} />
          </Form.Item>

          <Form.Item
            name="request_timeout"
            label="请求超时时间（秒）"
            initialValue={180}
            tooltip="请求超时时间定义了等待API响应的最长时间，超过此时间未收到响应则请求失败。合理设置超时时间能够：1) 避免无限等待 - 当API服务异常时及时终止请求；2) 提升用户体验 - 防止Claude CLI长时间无响应；3) 资源管理 - 释放被挂起的连接资源。建议值：简单请求60-90秒、复杂推理120-180秒、长文本/代码生成180-300秒。默认180秒（3分钟）可以应对大部分复杂场景，包括长代码生成和深度分析任务。注意：设置过短可能导致正常的长响应被中断，设置过长则可能在网络故障时浪费时间。最大支持600秒（10分钟）。"
          >
            <InputNumber min={1} max={600} style={{ width: '100%' }} />
          </Form.Item>

          <Divider />

          <Form.Item>
            <Space>
              <Button
                type="primary"
                htmlType="submit"
                icon={<SaveOutlined />}
                loading={loading}
                size="large"
              >
                创建配置
              </Button>
              <Button onClick={() => navigate('/ui')} size="large">
                取消
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Card>
    </div>
  );
};

export default ConfigCreate;
