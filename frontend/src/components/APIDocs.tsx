import React, { useState, useEffect } from 'react';
import {
  Card,
  Typography,
  Collapse,
  Tag,
  Space,
  Button,
  message,
  Descriptions,
  Divider,
} from 'antd';
import { CopyOutlined } from '@ant-design/icons';
import { configAPI } from '../services/api';

const { Title, Paragraph, Text } = Typography;
const { Panel } = Collapse;

const APIDocs: React.FC = () => {
  const [docs, setDocs] = useState<any>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    loadDocs();
  }, []);

  const loadDocs = async () => {
    setLoading(true);
    try {
      const response = await configAPI.getAPIDocs();
      setDocs(response);
    } catch (error: any) {
      message.error('加载API文档失败: ' + (error.response?.data?.error?.message || error.message));
    } finally {
      setLoading(false);
    }
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    message.success('已复制到剪贴板');
  };

  if (!docs) {
    return <Card loading={loading}>加载中...</Card>;
  }

  return (
    <Card>
      <Title level={2}>{docs.title}</Title>
      <Paragraph>{docs.description}</Paragraph>
      <Text type="secondary">版本: {docs.version}</Text>

      <Divider />

      <Collapse>
        <Panel header="Claude API 兼容端点" key="claude_api">
          {docs.endpoints?.claude_api?.endpoints?.map((endpoint: any, index: number) => (
            <Card key={index} style={{ marginBottom: 16 }}>
              <Space>
                <Tag color="blue">{endpoint.method}</Tag>
                <Text strong>{endpoint.path}</Text>
              </Space>
              <Paragraph>{endpoint.description}</Paragraph>
              <Text type="secondary">认证: {endpoint.auth}</Text>
            </Card>
          ))}
        </Panel>

        <Panel header="OpenAI API配置管理端点" key="config_management">
          {docs.endpoints?.config_management?.endpoints?.map((endpoint: any, index: number) => (
            <Card key={index} style={{ marginBottom: 16 }}>
              <Space>
                <Tag color="green">{endpoint.method}</Tag>
                <Text strong>{endpoint.path}</Text>
              </Space>
              <Paragraph>{endpoint.description}</Paragraph>
            </Card>
          ))}
        </Panel>

        <Panel header="工具端点" key="utility">
          {docs.endpoints?.utility?.endpoints?.map((endpoint: any, index: number) => (
            <Card key={index} style={{ marginBottom: 16 }}>
              <Space>
                <Tag color="orange">{endpoint.method}</Tag>
                <Text strong>{endpoint.path}</Text>
              </Space>
              <Paragraph>{endpoint.description}</Paragraph>
            </Card>
          ))}
        </Panel>

        <Panel header="使用示例" key="usage">
          <Card>
            <Title level={4}>Claude Code CLI 使用</Title>
            <Paragraph>
              <Text code>{docs.usage?.claude_code_cli}</Text>
              <Button
                type="link"
                icon={<CopyOutlined />}
                onClick={() => copyToClipboard(docs.usage?.claude_code_cli)}
              />
            </Paragraph>
            <Paragraph>
              <Text strong>多配置使用:</Text>
            </Paragraph>
            <Paragraph>
              <Text code>{docs.usage?.multiple_configs}</Text>
              <Button
                type="link"
                icon={<CopyOutlined />}
                onClick={() => copyToClipboard(docs.usage?.multiple_configs)}
              />
            </Paragraph>
          </Card>
        </Panel>
      </Collapse>
    </Card>
  );
};

export default APIDocs;

