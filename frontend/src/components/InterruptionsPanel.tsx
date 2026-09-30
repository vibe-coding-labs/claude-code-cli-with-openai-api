/* eslint-disable react-hooks/exhaustive-deps */
import React, { useEffect, useMemo, useState } from 'react';
import { Card, Col, Row, Select, Table, Tag, Typography, Alert, Spin, Statistic, Empty } from 'antd';
import {
  interruptionAPI,
  InterruptionEvent,
  InterruptionStat,
  FlappingSession,
} from '../services/api';

const { Text, Title } = Typography;

// 中断原因分类 -> 中文标签 + 颜色
const CAUSE_META: Record<string, { label: string; color: string }> = {
  client_disconnect: { label: '客户端断开', color: 'default' },
  client_timeout: { label: '客户端超时', color: 'default' },
  upstream_stall: { label: '上游无数据', color: 'orange' },
  upstream_error: { label: '上游错误', color: 'red' },
  rate_limit: { label: '限流 429', color: 'volcano' },
  auth_error: { label: '鉴权失败', color: 'magenta' },
  conversion_error: { label: '转换错误', color: 'geekblue' },
  server_termination: { label: '服务器停机', color: 'purple' },
  unknown: { label: '未知', color: 'gray' },
};

const causeMeta = (cause: string) =>
  CAUSE_META[cause] ?? { label: cause || 'unknown', color: 'gray' };

// 维度 -> 中文标签
const DIMENSION_META: Record<string, { label: string; color: string }> = {
  subjective: { label: '主观停止', color: 'default' },
  infrastructure: { label: '基础设施故障', color: 'red' },
};

const dimensionMeta = (dim: string) =>
  DIMENSION_META[dim] ?? { label: dim || 'unknown', color: 'default' };

const WINDOWS = [
  { value: 15, label: '最近 15 分钟' },
  { value: 60, label: '最近 1 小时' },
  { value: 360, label: '最近 6 小时' },
  { value: 1440, label: '最近 24 小时' },
];

const formatTime = (iso: string) =>
  new Date(iso).toLocaleString('zh-CN', { hour12: false });

const InterruptionsPanel: React.FC = () => {
  const [windowMin, setWindowMin] = useState(60);
  const [events, setEvents] = useState<InterruptionEvent[]>([]);
  const [stats, setStats] = useState<InterruptionStat[]>([]);
  const [flapping, setFlapping] = useState<FlappingSession[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = async () => {
    try {
      setLoading(true);
      const [evt, st] = await Promise.all([
        interruptionAPI.getInterruptions({ window: windowMin, limit: 100 }),
        interruptionAPI.getInterruptionStats({ window: windowMin, by_config: true }),
      ]);
      setEvents(evt.interruptions);
      setStats(st.stats);
      setFlapping(st.flapping);
      setError(null);
    } catch (err: any) {
      setError(err.message || '加载中断数据失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
  }, [windowMin]);

  const totals = useMemo(() => {
    const infra = stats
      .filter((s) => s.dimension === 'infrastructure')
      .reduce((a, s) => a + s.count, 0);
    const subjective = stats
      .filter((s) => s.dimension === 'subjective')
      .reduce((a, s) => a + s.count, 0);
    return { infra, subjective, total: infra + subjective, flapping: flapping.length };
  }, [stats, flapping]);

  const statColumns = [
    {
      title: '原因',
      dataIndex: 'interruption_cause',
      key: 'cause',
      render: (cause: string, _: InterruptionStat) => {
        const m = causeMeta(cause);
        return <Tag color={m.color}>{m.label}</Tag>;
      },
    },
    {
      title: '维度',
      dataIndex: 'dimension',
      key: 'dimension',
      render: (dim: string) => {
        const m = dimensionMeta(dim);
        return <Tag color={m.color}>{m.label}</Tag>;
      },
    },
    {
      title: '配置',
      key: 'config',
      render: (_: string, s: InterruptionStat) =>
        s.config_name || s.config_id || '全部',
    },
    {
      title: '次数',
      dataIndex: 'count',
      key: 'count',
      sorter: (a: InterruptionStat, b: InterruptionStat) => a.count - b.count,
      defaultSortOrder: 'descend' as const,
    },
  ];

  const eventColumns = [
    {
      title: '时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 170,
      render: (t: string) => formatTime(t),
    },
    {
      title: '原因',
      dataIndex: 'interruption_cause',
      key: 'cause',
      render: (cause: string) => {
        const m = causeMeta(cause);
        return <Tag color={m.color}>{m.label}</Tag>;
      },
    },
    {
      title: '维度',
      dataIndex: 'dimension',
      key: 'dimension',
      render: (dim: string) => {
        const m = dimensionMeta(dim);
        return <Tag color={m.color}>{m.label}</Tag>;
      },
    },
    { title: '阶段', dataIndex: 'stage', key: 'stage', width: 100 },
    {
      title: '配置',
      dataIndex: 'config_name',
      key: 'config_name',
      render: (v: string, e: InterruptionEvent) => e.config_name || e.config_id || '-',
    },
    {
      title: '模型',
      dataIndex: 'model',
      key: 'model',
      render: (v?: string) => v || '-',
    },
    {
      title: '会话',
      dataIndex: 'session_id',
      key: 'session_id',
      render: (v?: string) => (v ? <Text code>{v.slice(0, 12)}</Text> : '-'),
    },
    {
      title: '耗时(ms)',
      dataIndex: 'duration_ms',
      key: 'duration_ms',
      render: (v?: number) => (v ? v.toLocaleString() : '-'),
    },
    {
      title: '详情',
      dataIndex: 'detail',
      key: 'detail',
      ellipsis: true,
      render: (v?: string) => v || '-',
    },
  ];

  const flappingColumns = [
    {
      title: '会话',
      dataIndex: 'session_id',
      key: 'session_id',
      render: (v: string) => <Text code>{v.slice(0, 16)}</Text>,
    },
    {
      title: '配置',
      dataIndex: 'config_name',
      key: 'config_name',
      render: (_: string, f: FlappingSession) => f.config_name || f.config_id || '-',
    },
    {
      title: '中断次数',
      dataIndex: 'count',
      key: 'count',
      sorter: (a: FlappingSession, b: FlappingSession) => a.count - b.count,
      defaultSortOrder: 'descend' as const,
    },
    { title: '最后中断', dataIndex: 'last_at', key: 'last_at', render: (t?: string) => (t ? formatTime(t) : '-') },
  ];

  return (
    <div>
      <Title level={4}>会话中断监控</Title>
      <Row gutter={[12, 12]} align="middle" style={{ marginBottom: 16 }}>
        <Col>
          <Select
            value={windowMin}
            onChange={setWindowMin}
            options={WINDOWS}
            style={{ width: 160 }}
          />
        </Col>
        <Col flex="auto">
          <Text type="secondary">
            主观停止(用户主动 Esc/断网) 仅统计不告警；基础设施故障参与告警与自动自愈。
          </Text>
        </Col>
      </Row>

      {error && (
        <Alert type="error" showIcon message="加载失败" description={error} style={{ marginBottom: 16 }} />
      )}

      {loading ? (
        <div style={{ textAlign: 'center', padding: 48 }}>
          <Spin />
        </div>
      ) : (
        <>
          <Row gutter={16} style={{ marginBottom: 16 }}>
            <Col span={6}>
              <Card>
                <Statistic title="全部中断" value={totals.total} />
              </Card>
            </Col>
            <Col span={6}>
              <Card>
                <Statistic
                  title="基础设施故障"
                  value={totals.infra}
                  valueStyle={{ color: totals.infra > 0 ? '#cf1322' : undefined }}
                />
              </Card>
            </Col>
            <Col span={6}>
              <Card>
                <Statistic title="主观停止" value={totals.subjective} valueStyle={{ color: '#8c8c8c' }} />
              </Card>
            </Col>
            <Col span={6}>
              <Card>
                <Statistic title="抖动会话(flapping)" value={totals.flapping} valueStyle={{ color: totals.flapping > 0 ? '#fa8c16' : undefined }} />
              </Card>
            </Col>
          </Row>

          <Row gutter={16}>
            <Col span={12}>
              <Card title="按原因分布" size="small" style={{ marginBottom: 16 }}>
                {stats.length === 0 ? (
                  <Empty description="无中断事件" />
                ) : (
                  <Table
                    rowKey={(s) => `${s.interruption_cause}-${s.dimension}-${s.config_id || ''}-${s.config_name || ''}`}
                    columns={statColumns}
                    dataSource={stats}
                    size="small"
                    pagination={false}
                  />
                )}
              </Card>
            </Col>
            <Col span={12}>
              <Card title={`抖动会话（≥3 次中断）`} size="small" style={{ marginBottom: 16 }}>
                {flapping.length === 0 ? (
                  <Empty description="无抖动会话" />
                ) : (
                  <Table
                    rowKey={(f) => f.session_id}
                    columns={flappingColumns}
                    dataSource={flapping}
                    size="small"
                    pagination={false}
                  />
                )}
              </Card>
            </Col>
          </Row>

          <Card title="最近中断事件" size="small">
            <Table
              rowKey={(e) => String(e.id)}
              columns={eventColumns}
              dataSource={events}
              size="small"
              pagination={{ pageSize: 20, showSizeChanger: false }}
            />
          </Card>
        </>
      )}
    </div>
  );
};

export default InterruptionsPanel;