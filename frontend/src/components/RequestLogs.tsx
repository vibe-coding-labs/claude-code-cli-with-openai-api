import React, { useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Table,
  Card,
  Tag,
  Button,
  Input,
  Select,
  Space,
  message,
  Typography,
  Row,
  Col,
  Statistic,
  Popconfirm,
} from 'antd';
import {
  SearchOutlined,
  ReloadOutlined,
  DeleteOutlined,
  FilterOutlined,
} from '@ant-design/icons';
import axios from 'axios';

const { Text } = Typography;
const { Option } = Select;

interface RequestLog {
  id: number;
  model: string;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  duration_ms: number;
  status: string;
  error_message?: string;
  request_summary?: string;
  response_preview?: string;
  request_body?: string;
  response_body?: string;
  created_at: string;
}

interface LogsResult {
  logs: RequestLog[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

interface Stats {
  total_requests: number;
  success_requests: number;
  error_requests: number;
  total_input_tokens: number;
  total_output_tokens: number;
  total_tokens: number;
  avg_duration_ms: number;
}

interface RequestLogsProps {
  configId: string;
}

const RequestLogs: React.FC<RequestLogsProps> = ({ configId }) => {
  const navigate = useNavigate();
  const [logs, setLogs] = useState<RequestLog[]>([]);
  const [stats, setStats] = useState<Stats | null>(null);
  const [loading, setLoading] = useState(false);
  const [total, setTotal] = useState(0);
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [models, setModels] = useState<string[]>([]);
  
  // Filters
  const [searchText, setSearchText] = useState('');
  const [statusFilter, setStatusFilter] = useState<string>('');
  const [modelFilter, setModelFilter] = useState<string>('');
  const [sortBy, setSortBy] = useState('created_at');
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('desc');

  const fetchLogs = useCallback(async () => {
    setLoading(true);
    try {
      const params: any = {
        page: currentPage,
        page_size: pageSize,
        sort_by: sortBy,
        sort_order: sortOrder,
      };
      
      if (statusFilter) params.status = statusFilter;
      if (modelFilter) params.model = modelFilter;
      if (searchText) params.search = searchText;

      const response = await axios.get<LogsResult>(`/api/configs/${configId}/logs`, { params });
      setLogs(response.data.logs || []);
      setTotal(response.data.total);
    } catch (error) {
      message.error('获取日志失败');
    } finally {
      setLoading(false);
    }
  }, [configId, currentPage, modelFilter, pageSize, searchText, sortBy, sortOrder, statusFilter]);

  const fetchStats = useCallback(async () => {
    try {
      const response = await axios.get(`/api/configs/${configId}/stats?days=30`);
      setStats(response.data.stats);
    } catch (error) {
      console.error('获取统计信息失败');
    }
  }, [configId]);

  const fetchModels = useCallback(async () => {
    try {
      const response = await axios.get(`/api/configs/${configId}/models`);
      setModels(response.data.models || []);
    } catch (error) {
      console.error('获取模型列表失败');
    }
  }, [configId]);

  useEffect(() => {
    fetchLogs();
    fetchStats();
    fetchModels();
  }, [fetchLogs, fetchModels, fetchStats]);

  const handleClearLogs = async () => {
    try {
      await axios.delete(`/api/configs/${configId}/logs`);
      message.success('日志已清空');
      fetchLogs();
      fetchStats();
    } catch (error: any) {
      message.error(error.response?.data?.error || '清空日志失败');
    }
  };

  const handleResetFilters = () => {
    setSearchText('');
    setStatusFilter('');
    setModelFilter('');
    setSortBy('created_at');
    setSortOrder('desc');
    setCurrentPage(1);
  };

  const successRate = stats && stats.total_requests > 0
    ? ((stats.success_requests / stats.total_requests) * 100).toFixed(2)
    : '0';

  const columns = [
    {
      title: '时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 180,
      render: (text: string) => new Date(text).toLocaleString('zh-CN'),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 80,
      render: (status: string) => (
        <Tag color={status === 'success' ? 'success' : 'error'}>
          {status === 'success' ? '成功' : '失败'}
        </Tag>
      ),
    },
    {
      title: '模型',
      dataIndex: 'model',
      key: 'model',
      width: 180,
      ellipsis: true,
    },
    {
      title: '请求摘要',
      dataIndex: 'request_summary',
      key: 'request_summary',
      ellipsis: true,
      width: 200,
      render: (text: string) => {
        if (!text) return <span style={{ color: '#999' }}>-</span>;
        return <span title={text}>{text}</span>;
      },
    },
    {
      title: '响应预览',
      dataIndex: 'response_preview',
      key: 'response_preview',
      ellipsis: true,
      width: 250,
      render: (text: string) => {
        if (!text) return <span style={{ color: '#999' }}>-</span>;
        return <span title={text}>{text}</span>;
      },
    },
    {
      title: 'Token (输入/输出/总计)',
      key: 'tokens',
      width: 180,
      render: (_: any, record: RequestLog) => {
        const inputTokens = record.input_tokens || 0;
        const outputTokens = record.output_tokens || 0;
        const totalTokens = record.total_tokens || (inputTokens + outputTokens);
        
        return (
          <span style={{ fontSize: 12, fontFamily: 'monospace' }}>
            {inputTokens} / {outputTokens} / {totalTokens}
          </span>
        );
      },
    },
    {
      title: '耗时(ms)',
      dataIndex: 'duration_ms',
      key: 'duration_ms',
      width: 100,
      align: 'right' as const,
    },
    {
      title: '操作',
      key: 'action',
      width: 80,
      fixed: 'right' as const,
      render: (_: any, record: RequestLog) => (
        <Button 
          type="link" 
          size="small" 
          onClick={(e) => {
            e.stopPropagation();
            navigate(`/ui/configs/${configId}/logs/${record.id}`);
          }}
          style={{ padding: 0 }}
        >
          详情
        </Button>
      ),
    },
  ];

  return (
    <div>
      {/* Statistics Banner */}
      {stats && (
        <Card 
          title={
            <span>
              📊 统计概览 
              <Text type="secondary" style={{ fontSize: 14, marginLeft: 8 }}>
                (最近30天)
              </Text>
            </span>
          }
          style={{ marginBottom: 16 }}
        >
          <Row gutter={[16, 16]}>
            {/* 请求总数 */}
            <Col xs={24} sm={12} md={8}>
              <Card size="small" style={{ background: '#f0f5ff', border: '1px solid #d6e4ff', height: '100%', minHeight: 120 }}>
                <Statistic 
                  title={<span style={{ fontSize: 13 }}>📝 总请求数</span>}
                  value={stats.total_requests} 
                  valueStyle={{ color: '#1890ff', fontSize: 28 }}
                  suffix={<Text type="secondary" style={{ fontSize: 14 }}>次</Text>}
                />
                <div style={{ marginTop: 4, height: 24 }}>&nbsp;</div>
              </Card>
            </Col>

            {/* 成功请求 */}
            <Col xs={24} sm={12} md={8}>
              <Card size="small" style={{ background: '#f6ffed', border: '1px solid #b7eb8f', height: '100%', minHeight: 120 }}>
                <Statistic 
                  title={<span style={{ fontSize: 13 }}>✓ 成功请求</span>}
                  value={stats.success_requests}
                  valueStyle={{ color: '#52c41a', fontSize: 28 }}
                  suffix={
                    <Text type="secondary" style={{ fontSize: 14 }}>
                      / {stats.total_requests}
                    </Text>
                  }
                />
                <div style={{ marginTop: 4, height: 24, display: 'flex', alignItems: 'center' }}>
                  <Tag color="success" style={{ fontSize: 12, margin: 0 }}>
                    成功率: {successRate}%
                  </Tag>
                </div>
              </Card>
            </Col>

            {/* 失败请求 */}
            <Col xs={24} sm={12} md={8}>
              <Card size="small" style={{ background: stats.error_requests > 0 ? '#fff2e8' : '#fafafa', border: stats.error_requests > 0 ? '1px solid #ffbb96' : '1px solid #d9d9d9', height: '100%', minHeight: 120 }}>
                <Statistic 
                  title={<span style={{ fontSize: 13 }}>✗ 失败请求</span>}
                  value={stats.error_requests}
                  valueStyle={{ color: stats.error_requests > 0 ? '#ff4d4f' : '#8c8c8c', fontSize: 28 }}
                  suffix={
                    <Text type="secondary" style={{ fontSize: 14 }}>
                      / {stats.total_requests}
                    </Text>
                  }
                />
                <div style={{ marginTop: 4, height: 24, display: 'flex', alignItems: 'center' }}>
                  {stats.error_requests > 0 ? (
                    <Tag color="error" style={{ fontSize: 12, margin: 0 }}>
                      失败率: {(100 - parseFloat(successRate)).toFixed(2)}%
                    </Tag>
                  ) : (
                    <span>&nbsp;</span>
                  )}
                </div>
              </Card>
            </Col>

            {/* Token消耗 */}
            <Col xs={24} sm={12} md={8}>
              <Card size="small" style={{ background: '#fff7e6', border: '1px solid #ffd591', height: '100%', minHeight: 120 }}>
                <Statistic 
                  title={<span style={{ fontSize: 13 }}>🎯 总Token消耗</span>}
                  value={stats.total_tokens}
                  valueStyle={{ color: '#fa8c16', fontSize: 28 }}
                  formatter={(value) => value.toLocaleString()}
                />
                <div style={{ marginTop: 4, fontSize: 11, color: '#8c8c8c', height: 24, display: 'flex', flexDirection: 'column', justifyContent: 'center' }}>
                  <div>输入: {stats.total_input_tokens.toLocaleString()}</div>
                </div>
              </Card>
            </Col>

            {/* 平均响应时间 */}
            <Col xs={24} sm={12} md={8}>
              <Card size="small" style={{ background: '#f9f0ff', border: '1px solid #d3adf7', height: '100%', minHeight: 120 }}>
                <Statistic
                  title={<span style={{ fontSize: 13 }}>⚡ 平均响应时间</span>}
                  value={stats.avg_duration_ms.toFixed(0)}
                  valueStyle={{ 
                    color: stats.avg_duration_ms > 5000 ? '#cf1322' : stats.avg_duration_ms > 2000 ? '#fa8c16' : '#722ed1',
                    fontSize: 28 
                  }}
                  suffix={<Text type="secondary" style={{ fontSize: 14 }}>ms</Text>}
                />
                <div style={{ marginTop: 4, height: 24, display: 'flex', alignItems: 'center' }}>
                  <Tag 
                    color={stats.avg_duration_ms > 5000 ? 'error' : stats.avg_duration_ms > 2000 ? 'warning' : 'purple'}
                    style={{ fontSize: 12, margin: 0 }}
                  >
                    {(stats.avg_duration_ms / 1000).toFixed(2)}秒
                  </Tag>
                </div>
              </Card>
            </Col>

            {/* 平均Token/请求 */}
            <Col xs={24} sm={12} md={8}>
              <Card size="small" style={{ background: '#e6fffb', border: '1px solid #87e8de', height: '100%', minHeight: 120 }}>
                <Statistic
                  title={<span style={{ fontSize: 13 }}>📈 平均Token/请求</span>}
                  value={stats.total_requests > 0 ? (stats.total_tokens / stats.total_requests).toFixed(0) : 0}
                  valueStyle={{ color: '#13c2c2', fontSize: 28 }}
                  formatter={(value) => value.toLocaleString()}
                />
                <div style={{ marginTop: 4, fontSize: 11, color: '#8c8c8c', height: 24, display: 'flex', flexDirection: 'column', justifyContent: 'center' }}>
                  <div>平均: {stats.total_requests > 0 ? (stats.total_tokens / stats.total_requests).toFixed(0) : 0}</div>
                </div>
              </Card>
            </Col>
          </Row>
        </Card>
      )}

      {/* Filters */}
      <Card title="筛选和排序" style={{ marginBottom: 16 }}>
        <Space direction="vertical" style={{ width: '100%' }} size="middle">
          {/* 第一行：搜索和筛选 */}
          <Space wrap size="middle">
            <Space direction="vertical" size={4}>
              <Text strong style={{ fontSize: 12 }}>关键词搜索</Text>
              <Input
                placeholder="搜索请求或响应内容"
                prefix={<SearchOutlined />}
                value={searchText}
                onChange={(e) => setSearchText(e.target.value)}
                onPressEnter={fetchLogs}
                style={{ width: 280 }}
                allowClear
              />
            </Space>
            
            <Space direction="vertical" size={4}>
              <Text strong style={{ fontSize: 12 }}>状态筛选</Text>
              <Select
                placeholder="全部状态"
                value={statusFilter}
                onChange={setStatusFilter}
                style={{ width: 130 }}
                allowClear
              >
                <Option value="success">✓ 成功</Option>
                <Option value="error">✗ 失败</Option>
              </Select>
            </Space>

            <Space direction="vertical" size={4}>
              <Text strong style={{ fontSize: 12 }}>模型筛选</Text>
              <Select
                placeholder="全部模型"
                value={modelFilter}
                onChange={setModelFilter}
                style={{ width: 200 }}
                allowClear
                showSearch
              >
                {models.map(model => (
                  <Option key={model} value={model}>{model}</Option>
                ))}
              </Select>
            </Space>
          </Space>

          {/* 第二行：排序和操作 */}
          <Space wrap size="middle">
            <Space direction="vertical" size={4}>
              <Text strong style={{ fontSize: 12 }}>排序字段</Text>
              <Select
                value={sortBy}
                onChange={setSortBy}
                style={{ width: 140 }}
              >
                <Option value="created_at">⏰ 创建时间</Option>
                <Option value="duration_ms">⚡ 响应耗时</Option>
                <Option value="total_tokens">📊 Token消耗</Option>
              </Select>
            </Space>

            <Space direction="vertical" size={4}>
              <Text strong style={{ fontSize: 12 }}>排序方式</Text>
              <Select
                value={sortOrder}
                onChange={setSortOrder}
                style={{ width: 110 }}
              >
                <Option value="desc">↓ 降序</Option>
                <Option value="asc">↑ 升序</Option>
              </Select>
            </Space>

            <Space direction="vertical" size={4}>
              <Text strong style={{ fontSize: 12, opacity: 0 }}>操作</Text>
              <Space>
                <Button icon={<FilterOutlined />} onClick={handleResetFilters}>
                  重置
                </Button>

                <Button icon={<ReloadOutlined />} onClick={fetchLogs} type="primary">
                  刷新
                </Button>

                <Popconfirm
                  title="确认清空所有日志吗？"
                  description="此操作不可撤销"
                  onConfirm={handleClearLogs}
                  okText="确认"
                  cancelText="取消"
                >
                  <Button icon={<DeleteOutlined />} danger>
                    清空日志
                  </Button>
                </Popconfirm>
              </Space>
            </Space>
          </Space>
        </Space>
      </Card>

      {/* Logs Table */}
      <Table
        dataSource={logs}
        columns={columns}
        rowKey="id"
        loading={loading}
        onRow={(record) => ({
          onClick: (e) => {
            // 如果点击的是详情按钮，不要触发行点击
            const target = e.target as HTMLElement;
            if (target.closest('button') || target.closest('a')) {
              return;
            }
            navigate(`/ui/configs/${configId}/logs/${record.id}`);
          },
          style: { cursor: 'pointer' },
        })}
        rowClassName={() => 'log-row-hover'}
        pagination={{
          current: currentPage,
          pageSize: pageSize,
          total: total,
          showSizeChanger: true,
          showQuickJumper: true,
          showTotal: (total) => `共 ${total} 条记录`,
          onChange: (page, size) => {
            setCurrentPage(page);
            if (size !== pageSize) {
              setPageSize(size);
              setCurrentPage(1);
            }
          },
        }}
        scroll={{ x: 1400 }}
      />
      
      <style>{`
        .log-row-hover:hover {
          background-color: #f5f5f5 !important;
        }
        .log-row-hover {
          transition: background-color 0.2s ease;
        }
      `}</style>
    </div>
  );
};

export default RequestLogs;
