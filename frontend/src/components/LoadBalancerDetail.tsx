import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Card,
  Descriptions,
  Button,
  Space,
  Tag,
  Table,
  Tabs,
  Statistic,
  Row,
  Col,
  message,
  Modal,
  Input,
  Popconfirm,
  Spin,
} from 'antd';
import {
  EditOutlined,
  DeleteOutlined,
  ReloadOutlined,
  KeyOutlined,
  PlayCircleOutlined,
  ApiOutlined,
} from '@ant-design/icons';
import { loadBalancerApi, LoadBalancer, LoadBalancerStats } from '../services/loadBalancerApi';
import api from '../services/api';
import HealthStatusPanel from './HealthStatusPanel';
import CircuitBreakerPanel from './CircuitBreakerPanel';
import AlertsPanel from './AlertsPanel';
import EnhancedStatsPanel from './EnhancedStatsPanel';
import RequestLogsPanel from './RequestLogsPanel';
import './LoadBalancerDetail.css';

const { TabPane } = Tabs;

interface APIConfig {
  id: string;
  name: string;
  description: string;
}

const LoadBalancerDetail: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();

  const [loadBalancer, setLoadBalancer] = useState<LoadBalancer | null>(null);
  const [stats, setStats] = useState<LoadBalancerStats | null>(null);
  const [configs, setConfigs] = useState<APIConfig[]>([]);
  const [loading, setLoading] = useState(true);
  const [renewKeyModalVisible, setRenewKeyModalVisible] = useState(false);
  const [customToken, setCustomToken] = useState('');
  const [renewingKey, setRenewingKey] = useState(false);

  useEffect(() => {
    if (id) {
      loadData();
    }
  }, [id]);

  const loadData = async () => {
    if (!id) return;

    setLoading(true);
    try {
      const [lbData, statsData, configsData] = await Promise.all([
        loadBalancerApi.get(id),
        loadBalancerApi.getStats(id),
        api.get('/api/configs'),
      ]);

      setLoadBalancer(lbData);
      setStats(statsData);
      setConfigs(configsData.data.configs || []);
    } catch (error) {
      message.error('加载数据失败');
      console.error('Failed to load data:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleDelete = async () => {
    if (!id) return;

    try {
      await loadBalancerApi.delete(id);
      message.success('负载均衡器已删除');
      navigate('/ui/load-balancers');
    } catch (error) {
      message.error('删除失败');
      console.error('Failed to delete load balancer:', error);
    }
  };

  const handleTest = async () => {
    if (!id) return;

    try {
      const result = await loadBalancerApi.test(id);
      Modal.success({
        title: '测试成功',
        content: (
          <div>
            <p>选中的配置：{result.selected_config}</p>
            <p>配置ID：{result.config_id}</p>
            <p>策略：{result.strategy}</p>
            <p>可用配置数：{result.available_configs}</p>
          </div>
        ),
      });
    } catch (error: any) {
      message.error(error.response?.data?.error || '测试失败');
      console.error('Failed to test load balancer:', error);
    }
  };

  const handleRenewKey = async () => {
    if (!id) return;

    setRenewingKey(true);
    try {
      const result = await loadBalancerApi.renewKey(id, customToken);
      message.success('API Key已更新');
      Modal.info({
        title: '新的API Key',
        content: (
          <div>
            <p>请妥善保存以下API Key：</p>
            <Input.TextArea
              value={result.anthropic_api_key}
              readOnly
              autoSize
              style={{ fontFamily: 'monospace' }}
            />
          </div>
        ),
      });
      setRenewKeyModalVisible(false);
      setCustomToken('');
      loadData();
    } catch (error: any) {
      message.error(error.response?.data?.error || '更新失败');
      console.error('Failed to renew key:', error);
    } finally {
      setRenewingKey(false);
    }
  };

  const getStrategyLabel = (strategy: string) => {
    const labels: Record<string, string> = {
      round_robin: '轮询',
      random: '随机',
      weighted: '权重',
      least_connections: '最少连接',
    };
    return labels[strategy] || strategy;
  };

  const getConfigName = (configId: string) => {
    const config = configs.find(c => c.id === configId);
    return config?.name || configId;
  };

  const configColumns = [
    {
      title: '配置名称',
      dataIndex: 'config_id',
      key: 'config_id',
      render: (configId: string) => (
        <Space>
          <ApiOutlined />
          {getConfigName(configId)}
        </Space>
      ),
    },
    {
      title: '权重',
      dataIndex: 'weight',
      key: 'weight',
      render: (weight: number) => <Tag color="blue">{weight}</Tag>,
    },
    {
      title: '状态',
      dataIndex: 'enabled',
      key: 'enabled',
      render: (enabled: boolean) => (
        <Tag color={enabled ? 'success' : 'default'}>
          {enabled ? '启用' : '禁用'}
        </Tag>
      ),
    },
  ];

  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: '100px 0' }}>
        <Spin size="large" />
      </div>
    );
  }

  if (!loadBalancer) {
    return <div>负载均衡器不存在</div>;
  }

  return (
    <div className="load-balancer-detail">
      <Card>
        <Space direction="vertical" style={{ width: '100%' }} size="large">
          {/* 头部操作栏 */}
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <h2>{loadBalancer.name}</h2>
            <Space>
              <Button icon={<PlayCircleOutlined />} onClick={handleTest}>
                测试
              </Button>
              <Button icon={<KeyOutlined />} onClick={() => setRenewKeyModalVisible(true)}>
                更新API Key
              </Button>
              <Button
                icon={<EditOutlined />}
                type="primary"
                onClick={() => navigate(`/ui/load-balancers/${id}/edit`)}
              >
                编辑
              </Button>
              <Popconfirm
                title="确定要删除这个负载均衡器吗？"
                onConfirm={handleDelete}
                okText="确定"
                cancelText="取消"
              >
                <Button icon={<DeleteOutlined />} danger>
                  删除
                </Button>
              </Popconfirm>
              <Button icon={<ReloadOutlined />} onClick={loadData}>
                刷新
              </Button>
            </Space>
          </div>

          {/* 标签页 */}
          <Tabs defaultActiveKey="overview">
            <TabPane tab="概览" key="overview">
              <Space direction="vertical" style={{ width: '100%' }} size="large">
                <Descriptions bordered column={2}>
                  <Descriptions.Item label="ID">{loadBalancer.id}</Descriptions.Item>
                  <Descriptions.Item label="名称">{loadBalancer.name}</Descriptions.Item>
                  <Descriptions.Item label="描述" span={2}>
                    {loadBalancer.description || '-'}
                  </Descriptions.Item>
                  <Descriptions.Item label="策略">
                    <Tag color="blue">{getStrategyLabel(loadBalancer.strategy)}</Tag>
                  </Descriptions.Item>
                  <Descriptions.Item label="状态">
                    <Tag color={loadBalancer.enabled ? 'success' : 'default'}>
                      {loadBalancer.enabled ? '启用' : '禁用'}
                    </Tag>
                  </Descriptions.Item>
                  <Descriptions.Item label="API Key" span={2}>
                    <Input.Password
                      value={loadBalancer.anthropic_api_key}
                      readOnly
                      style={{ fontFamily: 'monospace' }}
                    />
                  </Descriptions.Item>
                  <Descriptions.Item label="创建时间">
                    {new Date(loadBalancer.created_at).toLocaleString('zh-CN')}
                  </Descriptions.Item>
                  <Descriptions.Item label="更新时间">
                    {new Date(loadBalancer.updated_at).toLocaleString('zh-CN')}
                  </Descriptions.Item>
                </Descriptions>

                {/* 健康检查配置 */}
                <Card title="健康检查配置" size="small">
                  <Descriptions bordered column={2} size="small">
                    <Descriptions.Item label="启用状态">
                      <Tag color={loadBalancer.health_check_enabled ? 'success' : 'default'}>
                        {loadBalancer.health_check_enabled ? '启用' : '禁用'}
                      </Tag>
                    </Descriptions.Item>
                    <Descriptions.Item label="检查间隔">
                      {loadBalancer.health_check_interval || 30} 秒
                    </Descriptions.Item>
                    <Descriptions.Item label="失败阈值">
                      {loadBalancer.failure_threshold || 3} 次
                    </Descriptions.Item>
                    <Descriptions.Item label="恢复阈值">
                      {loadBalancer.recovery_threshold || 2} 次
                    </Descriptions.Item>
                    <Descriptions.Item label="超时时间" span={2}>
                      {loadBalancer.health_check_timeout || 5} 秒
                    </Descriptions.Item>
                  </Descriptions>
                </Card>

                {/* 重试配置 */}
                <Card title="重试配置" size="small">
                  <Descriptions bordered column={2} size="small">
                    <Descriptions.Item label="最大重试次数">
                      {loadBalancer.max_retries || 3} 次
                    </Descriptions.Item>
                    <Descriptions.Item label="初始延迟">
                      {loadBalancer.initial_retry_delay || 100} 毫秒
                    </Descriptions.Item>
                    <Descriptions.Item label="最大延迟" span={2}>
                      {loadBalancer.max_retry_delay || 5000} 毫秒
                    </Descriptions.Item>
                  </Descriptions>
                </Card>

                {/* 熔断器配置 */}
                <Card title="熔断器配置" size="small">
                  <Descriptions bordered column={2} size="small">
                    <Descriptions.Item label="启用状态">
                      <Tag color={loadBalancer.circuit_breaker_enabled ? 'success' : 'default'}>
                        {loadBalancer.circuit_breaker_enabled ? '启用' : '禁用'}
                      </Tag>
                    </Descriptions.Item>
                    <Descriptions.Item label="错误率阈值">
                      {((loadBalancer.error_rate_threshold || 0.5) * 100).toFixed(0)}%
                    </Descriptions.Item>
                    <Descriptions.Item label="时间窗口">
                      {loadBalancer.circuit_breaker_window || 60} 秒
                    </Descriptions.Item>
                    <Descriptions.Item label="超时时间">
                      {loadBalancer.circuit_breaker_timeout || 30} 秒
                    </Descriptions.Item>
                    <Descriptions.Item label="半开测试请求数" span={2}>
                      {loadBalancer.half_open_requests || 3} 个
                    </Descriptions.Item>
                  </Descriptions>
                </Card>

                {/* 动态权重配置 */}
                <Card title="动态权重配置" size="small">
                  <Descriptions bordered column={2} size="small">
                    <Descriptions.Item label="启用状态">
                      <Tag color={loadBalancer.dynamic_weight_enabled ? 'success' : 'default'}>
                        {loadBalancer.dynamic_weight_enabled ? '启用' : '禁用'}
                      </Tag>
                    </Descriptions.Item>
                    <Descriptions.Item label="更新间隔">
                      {loadBalancer.weight_update_interval || 300} 秒
                    </Descriptions.Item>
                  </Descriptions>
                </Card>

                {/* 日志配置 */}
                <Card title="日志配置" size="small">
                  <Descriptions bordered column={1} size="small">
                    <Descriptions.Item label="日志级别">
                      <Tag color="blue">
                        {loadBalancer.log_level === 'minimal' && '最小'}
                        {loadBalancer.log_level === 'standard' && '标准'}
                        {loadBalancer.log_level === 'detailed' && '详细'}
                        {!loadBalancer.log_level && '标准'}
                      </Tag>
                    </Descriptions.Item>
                  </Descriptions>
                </Card>
              </Space>
            </TabPane>

            <TabPane tab="配置节点" key="configs">
              <Table
                columns={configColumns}
                dataSource={loadBalancer.config_nodes}
                rowKey="config_id"
                pagination={false}
              />
            </TabPane>

            <TabPane tab="健康状态" key="health">
              <HealthStatusPanel loadBalancerId={id!} />
            </TabPane>

            <TabPane tab="熔断器" key="circuit-breaker">
              <CircuitBreakerPanel loadBalancerId={id!} />
            </TabPane>

            <TabPane tab="告警" key="alerts">
              <AlertsPanel loadBalancerId={id!} />
            </TabPane>

            <TabPane tab="统计信息" key="stats">
              {stats && (
                <Space direction="vertical" style={{ width: '100%' }} size="large">
                  <Row gutter={16}>
                    <Col span={6}>
                      <Card>
                        <Statistic
                          title="总请求数"
                          value={stats.total_requests}
                          valueStyle={{ color: '#1890ff' }}
                        />
                      </Card>
                    </Col>
                    <Col span={6}>
                      <Card>
                        <Statistic
                          title="成功请求"
                          value={stats.success_requests}
                          valueStyle={{ color: '#52c41a' }}
                        />
                      </Card>
                    </Col>
                    <Col span={6}>
                      <Card>
                        <Statistic
                          title="失败请求"
                          value={stats.error_requests}
                          valueStyle={{ color: '#ff4d4f' }}
                        />
                      </Card>
                    </Col>
                    <Col span={6}>
                      <Card>
                        <Statistic
                          title="平均响应时间"
                          value={stats.avg_duration_ms.toFixed(2)}
                          suffix="ms"
                        />
                      </Card>
                    </Col>
                  </Row>

                  <Row gutter={16}>
                    <Col span={8}>
                      <Card>
                        <Statistic
                          title="输入Token"
                          value={stats.total_input_tokens}
                        />
                      </Card>
                    </Col>
                    <Col span={8}>
                      <Card>
                        <Statistic
                          title="输出Token"
                          value={stats.total_output_tokens}
                        />
                      </Card>
                    </Col>
                    <Col span={8}>
                      <Card>
                        <Statistic
                          title="总Token"
                          value={stats.total_tokens}
                        />
                      </Card>
                    </Col>
                  </Row>

                  <Card>
                    <Descriptions bordered>
                      <Descriptions.Item label="配置数量">
                        {stats.config_count}
                      </Descriptions.Item>
                      <Descriptions.Item label="成功率">
                        {stats.total_requests > 0
                          ? ((stats.success_requests / stats.total_requests) * 100).toFixed(2)
                          : 0}
                        %
                      </Descriptions.Item>
                    </Descriptions>
                  </Card>
                </Space>
              )}
            </TabPane>

            <TabPane tab="增强统计" key="enhanced-stats">
              <EnhancedStatsPanel loadBalancerId={id!} />
            </TabPane>

            <TabPane tab="请求日志" key="request-logs">
              <RequestLogsPanel loadBalancerId={id!} />
            </TabPane>
          </Tabs>
        </Space>
      </Card>

      {/* 更新API Key模态框 */}
      <Modal
        title="更新API Key"
        open={renewKeyModalVisible}
        onOk={handleRenewKey}
        onCancel={() => {
          setRenewKeyModalVisible(false);
          setCustomToken('');
        }}
        confirmLoading={renewingKey}
      >
        <Space direction="vertical" style={{ width: '100%' }}>
          <p>留空将自动生成新的API Key</p>
          <Input
            placeholder="自定义Token（可选）"
            value={customToken}
            onChange={(e) => setCustomToken(e.target.value)}
            maxLength={100}
          />
          <p style={{ fontSize: 12, color: '#8c8c8c' }}>
            自定义Token只能包含字母、数字和下划线，长度不超过100个字符
          </p>
        </Space>
      </Modal>
    </div>
  );
};

export default LoadBalancerDetail;
