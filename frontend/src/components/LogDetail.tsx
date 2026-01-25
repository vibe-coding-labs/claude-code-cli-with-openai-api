import React, { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { usePageTitle } from '../utils/pageTitle';
import {
  Card,
  Descriptions,
  Button,
  Tag,
  Typography,
  Spin,
  message,
  Space,
} from 'antd';
import { ArrowLeftOutlined } from '@ant-design/icons';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import { vscDarkPlus } from 'react-syntax-highlighter/dist/esm/styles/prism';
import axios from 'axios';

const { Title, Paragraph, Text } = Typography;

interface LogDetailData {
  id: number;
  config_id: string;
  model: string;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  duration_ms: number;
  status: string;
  error_message?: string;
  request_body?: string;
  response_body?: string;
  request_summary?: string;
  response_preview?: string;
  created_at: string;
}

interface ConfigInfo {
  id: string;
  name: string;
  openai_base_url: string;
}

const LogDetail: React.FC = () => {
  const { id: configId, log_id: logId } = useParams<{ id: string; log_id: string }>();
  const navigate = useNavigate();
  const [log, setLog] = useState<LogDetailData | null>(null);
  const [config, setConfig] = useState<ConfigInfo | null>(null);
  const [loading, setLoading] = useState(true);
  
  usePageTitle(log ? `日志 #${log.id}` : '日志详情');

  useEffect(() => {
    fetchLogDetail();
    fetchConfigInfo();
  }, [configId, logId]);

  const fetchLogDetail = async () => {
    if (!configId || !logId) {
      message.error('Invalid configuration or log ID');
      setLoading(false);
      return;
    }

    try {
      const response = await axios.get<LogDetailData>(`/api/configs/${configId}/logs/${logId}`);
      setLog(response.data);
    } catch (error: any) {
      message.error(error.response?.data?.error || '获取日志详情失败');
    } finally {
      setLoading(false);
    }
  };

  const fetchConfigInfo = async () => {
    if (!configId) return;
    
    try {
      const response = await axios.get(`/api/configs/${configId}`);
      setConfig({
        id: response.data.config.id,
        name: response.data.config.name,
        openai_base_url: response.data.config.openai_base_url,
      });
    } catch (error: any) {
      console.error('获取配置信息失败', error);
    }
  };

  const formatJson = (jsonString: string | undefined): string => {
    if (!jsonString) return '';
    try {
      const parsed = JSON.parse(jsonString);
      return JSON.stringify(parsed, null, 2);
    } catch (e) {
      return jsonString;
    }
  };

  const parseRequestInfo = () => {
    if (!log?.request_body) return null;
    try {
      const req = JSON.parse(log.request_body);
      return {
        model: req.model,
        max_tokens: req.max_tokens,
        stream: req.stream,
        message_count: req.messages?.length || 0,
        has_system: req.system && req.system.length > 0,
        metadata: req.metadata,
      };
    } catch (e) {
      return null;
    }
  };

  const requestInfo = parseRequestInfo();

  if (loading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '60vh' }}>
        <Spin size="large">
          <div style={{ padding: '50px 0' }}>加载日志详情...</div>
        </Spin>
      </div>
    );
  }

  if (!log) {
    return (
      <Card>
        <Paragraph>日志不存在</Paragraph>
        <Button onClick={() => navigate(`/ui/configs/${configId}?tab=logs`)}>返回日志列表</Button>
      </Card>
    );
  }

  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <Button
          icon={<ArrowLeftOutlined />}
          onClick={() => navigate(`/ui/configs/${configId}?tab=logs`)}
        >
          返回日志列表
        </Button>
      </Space>

      <Title level={3}>日志详情 #{log.id}</Title>

      <Card title="基本信息" style={{ marginBottom: 16 }}>
        <Descriptions column={2} bordered>
          <Descriptions.Item label="日志ID">{log.id}</Descriptions.Item>
          <Descriptions.Item label="配置ID">
            <Text copyable>{log.config_id}</Text>
          </Descriptions.Item>
          {config && (
            <>
              <Descriptions.Item label="配置名称" span={2}>{config.name}</Descriptions.Item>
              <Descriptions.Item label="OpenAI 端点" span={2}>
                <Text code copyable>{config.openai_base_url}</Text>
              </Descriptions.Item>
            </>
          )}
          <Descriptions.Item label="创建时间">
            {new Date(log.created_at).toLocaleString('zh-CN', {
              year: 'numeric',
              month: '2-digit',
              day: '2-digit',
              hour: '2-digit',
              minute: '2-digit',
              second: '2-digit',
              hour12: false
            })}
          </Descriptions.Item>
          <Descriptions.Item label="状态">
            <Tag color={log.status === 'success' ? 'success' : 'error'}>
              {log.status === 'success' ? '成功' : '失败'}
            </Tag>
          </Descriptions.Item>
          <Descriptions.Item label="Claude 模型" span={2}>
            <Text code>{log.model}</Text>
          </Descriptions.Item>
          <Descriptions.Item label="输入Token">
            <Text strong>{(log.input_tokens || 0).toLocaleString()}</Text>
          </Descriptions.Item>
          <Descriptions.Item label="输出Token">
            <Text strong>{(log.output_tokens || 0).toLocaleString()}</Text>
          </Descriptions.Item>
          <Descriptions.Item label="总Token">
            <Text strong type="success">{(log.total_tokens || 0).toLocaleString()}</Text>
          </Descriptions.Item>
          <Descriptions.Item label="耗时">
            <Text strong type={log.duration_ms > 10000 ? 'warning' : 'secondary'}>
              {log.duration_ms}ms ({(log.duration_ms / 1000).toFixed(2)}s)
            </Text>
          </Descriptions.Item>
        </Descriptions>
      </Card>

      {log.error_message && (
        <Card title="错误信息" style={{ marginBottom: 16 }}>
          <Paragraph style={{ color: 'red', whiteSpace: 'pre-wrap' }}>
            {log.error_message}
          </Paragraph>
        </Card>
      )}

      {requestInfo && (
        <Card title="请求参数" style={{ marginBottom: 16 }}>
          <Descriptions column={3} bordered size="small">
            <Descriptions.Item label="Claude 请求模型">
              <Text code>{requestInfo.model}</Text>
            </Descriptions.Item>
            <Descriptions.Item label="最大Token">
              {requestInfo.max_tokens || 'N/A'}
            </Descriptions.Item>
            <Descriptions.Item label="流式输出">
              <Tag color={requestInfo.stream ? 'blue' : 'default'}>
                {requestInfo.stream ? '是' : '否'}
              </Tag>
            </Descriptions.Item>
            <Descriptions.Item label="消息数量">
              {requestInfo.message_count}
            </Descriptions.Item>
            <Descriptions.Item label="System 提示">
              <Tag color={requestInfo.has_system ? 'green' : 'default'}>
                {requestInfo.has_system ? '有' : '无'}
              </Tag>
            </Descriptions.Item>
            {requestInfo.metadata && (
              <Descriptions.Item label="元数据" span={3}>
                <Text code style={{ fontSize: 11 }}>
                  {JSON.stringify(requestInfo.metadata)}
                </Text>
              </Descriptions.Item>
            )}
          </Descriptions>
        </Card>
      )}

      {log.request_summary && (
        <Card title="请求摘要" style={{ marginBottom: 16 }}>
          <Paragraph style={{ whiteSpace: 'pre-wrap' }}>{log.request_summary}</Paragraph>
        </Card>
      )}

      {log.response_preview && (
        <Card title="响应预览" style={{ marginBottom: 16 }}>
          <Paragraph>{log.response_preview}</Paragraph>
        </Card>
      )}

      {log.request_body && (
        <Card title="完整请求体" style={{ marginBottom: 16 }}>
          <SyntaxHighlighter
            language="json"
            style={vscDarkPlus}
            customStyle={{
              margin: 0,
              borderRadius: 4,
              fontSize: 12,
            }}
            wrapLines={true}
            wrapLongLines={true}
          >
            {formatJson(log.request_body)}
          </SyntaxHighlighter>
        </Card>
      )}

      {log.response_body && (
        <Card title="完整响应体" style={{ marginBottom: 16 }}>
          <SyntaxHighlighter
            language="json"
            style={vscDarkPlus}
            customStyle={{
              margin: 0,
              borderRadius: 4,
              fontSize: 12,
            }}
            wrapLines={true}
            wrapLongLines={true}
          >
            {formatJson(log.response_body)}
          </SyntaxHighlighter>
        </Card>
      )}

      {!log.request_body && !log.response_body && !log.error_message && (
        <Card>
          <Text type="secondary">暂无详细信息</Text>
        </Card>
      )}
    </div>
  );
};

export default LogDetail;
