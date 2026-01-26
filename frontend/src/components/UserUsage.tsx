import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Card, Col, Empty, Row, Select, Spin, Table, Tag, Typography, message, Input } from 'antd';
import { useParams } from 'react-router-dom';
import { userAPI } from '../services/api';
import { LogsResult, UserLog, UserTokenStats } from '../types/user';
import './UserManagement.css';

const { Title, Text } = Typography;
const { Option } = Select;

const UserUsage: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const userId = Number(id);
  const [stats, setStats] = useState<UserTokenStats[]>([]);
  const [logs, setLogs] = useState<UserLog[]>([]);
  const [loadingStats, setLoadingStats] = useState(false);
  const [loadingLogs, setLoadingLogs] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [total, setTotal] = useState(0);
  const [status, setStatus] = useState<string>('');
  const [model, setModel] = useState<string>('');
  const [search, setSearch] = useState('');

  const availableModels = useMemo(() => {
    const modelSet = new Set<string>();
    stats.forEach((item) => modelSet.add(item.model));
    return Array.from(modelSet);
  }, [stats]);

  const fetchStats = useCallback(async () => {
    if (!userId) return;
    setLoadingStats(true);
    try {
      const data = await userAPI.getUserStats(userId, 30);
      setStats(data);
    } catch (error: any) {
      message.error(error.response?.data?.error || '获取用量统计失败');
    } finally {
      setLoadingStats(false);
    }
  }, [userId]);

  const fetchLogs = useCallback(async () => {
    if (!userId) return;
    setLoadingLogs(true);
    try {
      const result: LogsResult = await userAPI.getUserLogs(userId, {
        page,
        page_size: pageSize,
        status: status || undefined,
        model: model || undefined,
        search: search || undefined,
        sort_by: 'created_at',
        sort_order: 'desc',
      });
      setLogs(result.logs || []);
      setTotal(result.total || 0);
    } catch (error: any) {
      message.error(error.response?.data?.error || '获取日志失败');
    } finally {
      setLoadingLogs(false);
    }
  }, [model, page, pageSize, search, status, userId]);

  useEffect(() => {
    fetchStats();
  }, [fetchStats]);

  useEffect(() => {
    fetchLogs();
  }, [fetchLogs]);

  if (!userId) {
    return <Empty description="无效的用户" />;
  }

  return (
    <div className="user-management">
      <Card>
        <Title level={4}>用户用量概览</Title>
        {loadingStats ? (
          <Spin />
        ) : stats.length === 0 ? (
          <Empty description="暂无用量数据" />
        ) : (
          <Row gutter={[16, 16]}>
            {stats.map((item) => (
              <Col span={12} key={item.model}>
                <Card size="small" title={item.model}>
                  <Row gutter={[8, 8]}>
                    <Col span={12}><Text>请求数：{item.total_requests}</Text></Col>
                    <Col span={12}><Text>错误数：{item.error_count}</Text></Col>
                    <Col span={12}><Text>输入 Tokens：{item.input_tokens}</Text></Col>
                    <Col span={12}><Text>输出 Tokens：{item.output_tokens}</Text></Col>
                    <Col span={24}><Text strong>总 Tokens：{item.total_tokens}</Text></Col>
                  </Row>
                </Card>
              </Col>
            ))}
          </Row>
        )}
      </Card>

      <Card
        title="请求日志"
        extra={
          <div className="user-management__filter-row">
            <Select
              allowClear
              placeholder="状态"
              style={{ width: 120 }}
              value={status || undefined}
              onChange={(value) => { setStatus(value || ''); setPage(1); }}
            >
              <Option value="success">成功</Option>
              <Option value="error">失败</Option>
            </Select>
            <Select
              allowClear
              placeholder="模型"
              style={{ width: 160 }}
              value={model || undefined}
              onChange={(value) => { setModel(value || ''); setPage(1); }}
            >
              {availableModels.map((item) => (
                <Option key={item} value={item}>{item}</Option>
              ))}
            </Select>
            <Input.Search
              allowClear
              placeholder="搜索摘要"
              style={{ width: 200 }}
              onSearch={(value) => { setSearch(value); setPage(1); }}
            />
          </div>
        }
      >
        <Table
          rowKey="id"
          loading={loadingLogs}
          dataSource={logs}
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            onChange: (nextPage, nextSize) => {
              setPage(nextPage);
              setPageSize(nextSize || 20);
            },
          }}
          columns={[
            { title: '模型', dataIndex: 'model' },
            { title: '总 Tokens', dataIndex: 'total_tokens' },
            { title: '输入 Tokens', dataIndex: 'input_tokens' },
            { title: '输出 Tokens', dataIndex: 'output_tokens' },
            { title: '耗时(ms)', dataIndex: 'duration_ms' },
            {
              title: '状态',
              dataIndex: 'status',
              render: (value: string) => <Tag color={value === 'success' ? 'green' : 'red'}>{value}</Tag>,
            },
            { title: '摘要', dataIndex: 'request_summary' },
            { title: '创建时间', dataIndex: 'created_at' },
          ]}
        />
      </Card>
    </div>
  );
};

export default UserUsage;
