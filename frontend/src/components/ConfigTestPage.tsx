import React, { useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Card,
  Input,
  Button,
  Space,
  Spin,
  message,
  Typography,
  Divider,
} from 'antd';
import {
  ArrowLeftOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';
import axios from 'axios';

const { TextArea } = Input;
const { Title, Text, Paragraph } = Typography;

const ConfigTestPage: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [userMessage, setUserMessage] = useState('Hello! Please introduce yourself.');
  const [testing, setTesting] = useState(false);
  const [result, setResult] = useState<any>(null);

  const handleTest = async () => {
    if (!userMessage.trim()) {
      message.warning('请输入测试消息');
      return;
    }

    setTesting(true);
    setResult(null);

    try {
      const response = await axios.post(`/api/configs/${id}/test`, {
        message: userMessage,
      });
      setResult(response.data);
      message.success('测试成功');
    } catch (error: any) {
      const errorMsg = error.response?.data?.error || error.message || '测试失败';
      setResult({
        success: false,
        error: errorMsg,
      });
      message.error('测试失败');
    } finally {
      setTesting(false);
    }
  };

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
        <div style={{ marginBottom: 24 }}>
          <Paragraph>
            在此页面可以测试配置是否正常工作。系统会发送一条测试消息到OpenAI API，并返回响应结果。
          </Paragraph>
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

        {result && (
          <>
            <Divider />
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
                      <Text style={{ marginLeft: 8 }}>{result.model}</Text>
                    </div>
                  )}

                  {result.usage && (
                    <div>
                      <Text strong>Token使用：</Text>
                      <div style={{ marginTop: 8 }}>
                        <Text>输入: {result.usage.input_tokens || result.usage.prompt_tokens || 0}</Text>
                        <Divider type="vertical" />
                        <Text>输出: {result.usage.output_tokens || result.usage.completion_tokens || 0}</Text>
                        <Divider type="vertical" />
                        <Text>总计: {result.usage.total_tokens || 0}</Text>
                      </div>
                    </div>
                  )}

                  {result.duration_ms && (
                    <div style={{ marginTop: 12 }}>
                      <Text strong>响应时间：</Text>
                      <Text style={{ marginLeft: 8 }}>{result.duration_ms}ms</Text>
                    </div>
                  )}
                </>
              ) : (
                <div>
                  <Paragraph type="danger" style={{ marginBottom: 12 }}>
                    {result.error || '未知错误'}
                  </Paragraph>
                  {result.details && (
                    <TextArea
                      value={JSON.stringify(result.details, null, 2)}
                      autoSize={{ minRows: 5, maxRows: 15 }}
                      readOnly
                      style={{ fontFamily: 'monospace', fontSize: 12 }}
                    />
                  )}
                </div>
              )}
            </Card>
          </>
        )}
      </Card>
    </div>
  );
};

export default ConfigTestPage;
