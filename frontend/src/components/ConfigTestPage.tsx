import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { usePageTitle } from '../utils/pageTitle';
import {
  Card,
  Input,
  Button,
  Space,
  Spin,
  message,
  Typography,
  Divider,
  Select,
  Alert,
  Collapse,
  Tag,
} from 'antd';
import {
  ArrowLeftOutlined,
  ThunderboltOutlined,
  CopyOutlined,
} from '@ant-design/icons';
import axios from 'axios';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import { tomorrow } from 'react-syntax-highlighter/dist/esm/styles/prism';

const { TextArea } = Input;
const { Title, Text, Paragraph } = Typography;
const { Panel } = Collapse;

const ConfigTestPage: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [userMessage, setUserMessage] = useState('Hello! Please introduce yourself.');
  const [testing, setTesting] = useState(false);
  const [result, setResult] = useState<any>(null);
  const [config, setConfig] = useState<any>(null);
  const [selectedModel, setSelectedModel] = useState<string>('');
  const [loading, setLoading] = useState(true);
  usePageTitle(config ? `测试 ${config.name}` : '配置测试');
  const [curlCommand, setCurlCommand] = useState<string>('');
  const [requestPayload, setRequestPayload] = useState<any>(null);

  useEffect(() => {
    fetchConfig();
  }, [id]);

  const fetchConfig = async () => {
    try {
      const response = await axios.get(`/api/configs/${id}`);
      const configData = response.data.config;
      setConfig(configData);
      // 设置默认模型
      if (configData.supported_models && configData.supported_models.length > 0) {
        setSelectedModel(configData.supported_models[0]);
      } else if (configData.big_model) {
        setSelectedModel(configData.big_model);
      }
      setLoading(false);
    } catch (error) {
      message.error('获取配置失败');
      setLoading(false);
    }
  };

  const generateCurlCommand = (payload: any) => {
    const baseUrl = config?.openai_base_url || 'https://api.openai.com/v1';
    const endpoint = `${baseUrl}/chat/completions`;
    const apiKey = 'YOUR_API_KEY'; // 不显示真实密钥
    
    return `curl ${endpoint} \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer ${apiKey}" \\
  -d '${JSON.stringify(payload, null, 2)}'`;
  };

  const handleTest = async () => {
    if (!userMessage.trim()) {
      message.warning('请输入测试消息');
      return;
    }

    if (!selectedModel) {
      message.warning('请选择一个模型');
      return;
    }

    setTesting(true);
    setResult(null);
    setCurlCommand('');
    setRequestPayload(null);

    // 构造请求payload
    const payload = {
      model: selectedModel,
      messages: [{
        role: 'user',
        content: userMessage
      }],
      temperature: 0.7,
      max_tokens: 1000
    };
    setRequestPayload(payload);
    
    // 生成curl命令
    const curl = generateCurlCommand(payload);
    setCurlCommand(curl);

    try {
      const response = await axios.post(`/api/configs/${id}/test`, {
        message: userMessage,
        model: selectedModel,
      });
      setResult(response.data);
      message.success('测试成功');
    } catch (error: any) {
      const errorMsg = error.response?.data?.error || error.message || '测试失败';
      setResult({
        success: false,
        error: errorMsg,
        response: error.response?.data
      });
      message.error('测试失败');
    } finally {
      setTesting(false);
    }
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    message.success('已复制到剪贴板');
  };

  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: 50 }}>
        <Spin size="large">
          <div style={{ padding: '50px 0' }}>加载配置中...</div>
        </Spin>
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
          返回配置详情
        </Button>
      </Space>

      <Card title="在线测试">
        <Alert
          message="测试说明"
          description="在此页面可以测试配置是否正常工作。选择一个模型并发送测试消息到OpenAI API，查看响应结果、curl命令和完整JSON响应。"
          type="info"
          showIcon
          style={{ marginBottom: 24 }}
        />

        {config && (
          <div style={{ marginBottom: 16, padding: 12, background: '#f5f5f5', borderRadius: 4 }}>
            <Text strong>当前配置：</Text>
            <div style={{ marginTop: 8 }}>
              <Tag color="blue">{config.name}</Tag>
              <Tag>{config.openai_base_url}</Tag>
            </div>
          </div>
        )}

        <div style={{ marginBottom: 16 }}>
          <Text strong>选择模型：</Text>
          <Select
            value={selectedModel}
            onChange={setSelectedModel}
            style={{ width: '100%', marginTop: 8 }}
            placeholder="选择要测试的模型"
          >
            {config?.supported_models && config.supported_models.length > 0 ? (
              config.supported_models.map((model: string) => (
                <Select.Option key={model} value={model}>
                  {model}
                </Select.Option>
              ))
            ) : (
              <>
                <Select.Option value={config?.big_model}>{config?.big_model} (大模型)</Select.Option>
                <Select.Option value={config?.middle_model}>{config?.middle_model} (中模型)</Select.Option>
                <Select.Option value={config?.small_model}>{config?.small_model} (小模型)</Select.Option>
              </>
            )}
          </Select>
        </div>

        <div style={{ marginBottom: 16 }}>
          <Text strong>测试消息：</Text>
          <TextArea
            value={userMessage}
            onChange={(e) => setUserMessage(e.target.value)}
            placeholder="输入要发送给AI的消息"
            autoSize={{ minRows: 3, maxRows: 8 }}
            style={{ marginTop: 8 }}
          />
        </div>

        <Button
          type="primary"
          icon={<ThunderboltOutlined />}
          onClick={handleTest}
          loading={testing}
          size="large"
        >
          {testing ? '测试中...' : '开始测试'}
        </Button>

        {(curlCommand || result) && (
          <>
            <Divider />
            
            {curlCommand && (
              <Collapse defaultActiveKey={['curl', 'request']} style={{ marginBottom: 16 }}>
                <Panel 
                  header="cURL 命令" 
                  key="curl"
                  extra={
                    <Button 
                      type="link" 
                      size="small" 
                      icon={<CopyOutlined />}
                      onClick={(e) => {
                        e.stopPropagation();
                        copyToClipboard(curlCommand);
                      }}
                    >
                      复制
                    </Button>
                  }
                >
                  <SyntaxHighlighter
                    language="bash"
                    style={tomorrow}
                    customStyle={{
                      margin: 0,
                      borderRadius: 4,
                      fontSize: 13,
                    }}
                  >
                    {curlCommand}
                  </SyntaxHighlighter>
                </Panel>
                
                <Panel 
                  header="请求 Payload (JSON)" 
                  key="request"
                  extra={
                    <Button 
                      type="link" 
                      size="small" 
                      icon={<CopyOutlined />}
                      onClick={(e) => {
                        e.stopPropagation();
                        copyToClipboard(JSON.stringify(requestPayload, null, 2));
                      }}
                    >
                      复制
                    </Button>
                  }
                >
                  <SyntaxHighlighter
                    language="json"
                    style={tomorrow}
                    customStyle={{
                      margin: 0,
                      borderRadius: 4,
                      fontSize: 13,
                    }}
                  >
                    {JSON.stringify(requestPayload, null, 2)}
                  </SyntaxHighlighter>
                </Panel>
              </Collapse>
            )}

            {result && (
              <Card
                type="inner"
                title={result.success !== false ? '测试结果' : '错误信息'}
                style={{
                  background: result.success !== false ? '#f6ffed' : '#fff2f0',
                  borderColor: result.success !== false ? '#b7eb8f' : '#ffccc7',
                }}
              >
                {result.success !== false ? (
                  <>
                    <div style={{ marginBottom: 12 }}>
                      <Text strong>响应消息：</Text>
                      <Paragraph style={{ marginTop: 8, whiteSpace: 'pre-wrap' }}>
                        {result.response?.content || result.message || '无响应内容'}
                      </Paragraph>
                    </div>

                    {result.model && (
                      <div style={{ marginBottom: 12 }}>
                        <Text strong>使用模型：</Text>
                        <Tag color="green" style={{ marginLeft: 8 }}>{result.model}</Tag>
                      </div>
                    )}

                    {result.usage && (
                      <div style={{ marginBottom: 12 }}>
                        <Text strong>Token使用：</Text>
                        <div style={{ marginTop: 8 }}>
                          <Tag>输入: {result.usage.input_tokens || result.usage.prompt_tokens || 0}</Tag>
                          <Tag>输出: {result.usage.output_tokens || result.usage.completion_tokens || 0}</Tag>
                          <Tag color="blue">总计: {result.usage.total_tokens || 0}</Tag>
                        </div>
                      </div>
                    )}

                    {result.duration_ms && (
                      <div style={{ marginBottom: 16 }}>
                        <Text strong>响应时间：</Text>
                        <Tag color="orange" style={{ marginLeft: 8 }}>{result.duration_ms}ms</Tag>
                      </div>
                    )}

                    <Collapse>
                      <Panel 
                        header="完整 JSON 响应" 
                        key="response"
                        extra={
                          <Button 
                            type="link" 
                            size="small" 
                            icon={<CopyOutlined />}
                            onClick={(e) => {
                              e.stopPropagation();
                              copyToClipboard(JSON.stringify(result, null, 2));
                            }}
                          >
                            复制
                          </Button>
                        }
                      >
                        <div style={{ maxHeight: 400, overflow: 'auto' }}>
                          <SyntaxHighlighter
                            language="json"
                            style={tomorrow}
                            customStyle={{
                              margin: 0,
                              borderRadius: 4,
                              fontSize: 13,
                            }}
                          >
                            {JSON.stringify(result, null, 2)}
                          </SyntaxHighlighter>
                        </div>
                      </Panel>
                    </Collapse>
                  </>
                ) : (
                  <>
                    <Paragraph type="danger" style={{ marginBottom: 12 }}>
                      {result.error || '未知错误'}
                    </Paragraph>
                    {result.response && (
                      <Collapse>
                        <Panel header="错误详情 (JSON)" key="error">
                          <div style={{ maxHeight: 400, overflow: 'auto' }}>
                            <SyntaxHighlighter
                              language="json"
                              style={tomorrow}
                              customStyle={{
                                margin: 0,
                                borderRadius: 4,
                                fontSize: 13,
                                background: '#fff2f0',
                              }}
                            >
                              {JSON.stringify(result.response, null, 2)}
                            </SyntaxHighlighter>
                          </div>
                        </Panel>
                      </Collapse>
                    )}
                  </>
                )}
              </Card>
            )}
          </>
        )}
      </Card>
    </div>
  );
};

export default ConfigTestPage;
