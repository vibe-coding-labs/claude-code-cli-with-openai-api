import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Table,
  Button,
  Space,
  Tag,
  Popconfirm,
  message,
  Card,
  Typography,
  Switch,
  Row,
  Col,
  Tooltip,
} from 'antd';
import {
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  EyeOutlined,
  ApiOutlined,
  AppstoreOutlined,
  UnorderedListOutlined,
} from '@ant-design/icons';
import { loadBalancerApi, LoadBalancer } from '../services/loadBalancerApi';
import './LoadBalancerList.css';

const { Title } = Typography;

const LoadBalancerList: React.FC = () => {
  const navigate = useNavigate();
  const [loadBalancers, setLoadBalancers] = useState<LoadBalancer[]>([]);
  const [loading, setLoading] = useState(false);
  const [viewMode, setViewMode] = useState<'card' | 'list'>('list');

  const loadData = async () => {
    setLoading(true);
    try {
      const data = await loadBalancerApi.getAll();
      setLoadBalancers(data);
    } catch (error: any) {
      message.error('加载负载均衡器失败');
      console.error('Failed to load load balancers:', error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
  }, []);

  const handleDelete = async (id: string, event?: React.MouseEvent) => {
    event?.stopPropagation();
    try {
      await loadBalancerApi.delete(id);
      message.success('负载均衡器已删除');
      loadData();
    } catch (error: any) {
      message.error('删除失败');
      console.error('Failed to delete load balancer:', error);
    }
  };

  const handleViewDetail = (id: string) => {
    navigate(`/ui/load-balancers/${id}`);
  };

  const handleEdit = (id: string, event?: React.MouseEvent) => {
    event?.stopPropagation();
    navigate(`/ui/load-balancers/${id}/edit`);
  };

  const handleViewModeChange = (mode: 'card' | 'list') => {
    setViewMode(mode);
  };

  const handleToggleEnabled = async (lb: LoadBalancer) => {
    try {
      await loadBalancerApi.update(lb.id, {
        name: lb.name,
        description: lb.description,
        strategy: lb.strategy,
        config_nodes: lb.config_nodes,
        enabled: !lb.enabled,
      });
      message.success(lb.enabled ? '已禁用' : '已启用');
      loadData();
    } catch (error: any) {
      message.error('操作失败');
      console.error('Failed to toggle enabled:', error);
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

  const formatDateTime = (value?: string) => {
    if (!value) {
      return '暂无';
    }
    return new Date(value).toLocaleString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    });
  };

  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      render: (text: string, record: LoadBalancer) => (
        <Space>
          <ApiOutlined />
          {text}
          {!record.enabled && <Tag color="default">已禁用</Tag>}
        </Space>
      ),
    },
    {
      title: '描述',
      dataIndex: 'description',
      key: 'description',
      ellipsis: true,
    },
    {
      title: '策略',
      dataIndex: 'strategy',
      key: 'strategy',
      render: (strategy: string) => (
        <Tag color="blue">{getStrategyLabel(strategy)}</Tag>
      ),
    },
    {
      title: '配置数量',
      key: 'config_count',
      render: (_: any, record: LoadBalancer) => (
        <Tag color="green">{record.config_nodes?.length || 0} 个配置</Tag>
      ),
    },
    {
      title: '状态',
      key: 'enabled',
      render: (_: any, record: LoadBalancer) => (
        <Switch
          checked={record.enabled}
          onChange={() => handleToggleEnabled(record)}
          checkedChildren="启用"
          unCheckedChildren="禁用"
        />
      ),
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (text: string) => new Date(text).toLocaleString('zh-CN'),
    },
    {
      title: '操作',
      key: 'action',
      width: 200,
      render: (_: any, record: LoadBalancer) => (
        <Space>
          <Button
            type="link"
            icon={<EyeOutlined />}
            onClick={() => navigate(`/ui/load-balancers/${record.id}`)}
          >
            详情
          </Button>
          <Button
            type="link"
            icon={<EditOutlined />}
            onClick={() => navigate(`/ui/load-balancers/${record.id}/edit`)}
          >
            编辑
          </Button>
          <Popconfirm
            title="确定要删除这个负载均衡器吗？"
            onConfirm={() => handleDelete(record.id)}
            okText="确定"
            cancelText="取消"
          >
            <Button
              type="link"
              danger
              icon={<DeleteOutlined />}
            />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <Card className="load-balancer-list">
      <div className="load-balancer-list__header">
        <div className="load-balancer-list__title-wrap">
          <Title level={4} className="load-balancer-list__title">
            负载均衡器
          </Title>
          <span className="load-balancer-list__count">共 {loadBalancers.length} 个</span>
        </div>
        <Space className="load-balancer-list__actions">
          <Space.Compact>
            <Tooltip title="卡片视图">
              <Button
                icon={<AppstoreOutlined />}
                type={viewMode === 'card' ? 'primary' : 'default'}
                onClick={() => handleViewModeChange('card')}
              />
            </Tooltip>
            <Tooltip title="列表视图">
              <Button
                icon={<UnorderedListOutlined />}
                type={viewMode === 'list' ? 'primary' : 'default'}
                onClick={() => handleViewModeChange('list')}
              />
            </Tooltip>
          </Space.Compact>
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={() => navigate('/ui/load-balancers/create')}
          >
            新建负载均衡器
          </Button>
        </Space>
      </div>

      {viewMode === 'list' ? (
        <Table
          columns={columns}
          dataSource={loadBalancers}
          rowKey="id"
          loading={loading}
          pagination={{ pageSize: 10 }}
          onRow={(record) => ({
            onClick: () => handleViewDetail(record.id),
            style: { cursor: 'pointer' },
          })}
        />
      ) : (
        <Row gutter={[16, 16]} className="load-balancer-card-grid">
          {loadBalancers.map((lb) => (
            <Col xs={24} sm={12} md={8} lg={6} key={lb.id}>
              <Card
                hoverable
                className="load-balancer-card"
                onClick={() => handleViewDetail(lb.id)}
                extra={
                  <Switch
                    checked={lb.enabled}
                    onChange={(checked, e) => {
                      e.stopPropagation();
                      handleToggleEnabled(lb);
                    }}
                    size="small"
                  />
                }
                actions={[
                  <Tooltip title="查看详情" key="view">
                    <EyeOutlined
                      className="load-balancer-card__action"
                      onClick={(e) => {
                        e.stopPropagation();
                        handleViewDetail(lb.id);
                      }}
                    />
                  </Tooltip>,
                  <Tooltip title="编辑" key="edit">
                    <EditOutlined
                      className="load-balancer-card__action"
                      onClick={(e) => handleEdit(lb.id, e)}
                    />
                  </Tooltip>,
                  <Popconfirm
                    title="确定要删除这个负载均衡器吗？"
                    onConfirm={(e) => handleDelete(lb.id, e)}
                    onCancel={(e) => e?.stopPropagation()}
                    okText="确定"
                    cancelText="取消"
                    key="delete"
                  >
                    <DeleteOutlined
                      className="load-balancer-card__action load-balancer-card__action--danger"
                      onClick={(e) => e.stopPropagation()}
                    />
                  </Popconfirm>,
                ]}
              >
                <Card.Meta
                  title={
                    <div className="load-balancer-card__title">
                      <div className="load-balancer-card__title-main">
                        <span className="load-balancer-card__icon">
                          <ApiOutlined />
                        </span>
                        <span className="load-balancer-card__name">{lb.name}</span>
                      </div>
                      <div className="load-balancer-card__title-tags">
                        {!lb.enabled && <Tag color="default">已禁用</Tag>}
                        <Tag color="blue">{getStrategyLabel(lb.strategy)}</Tag>
                      </div>
                    </div>
                  }
                  description={
                    <div className="load-balancer-card__content">
                      <div className="load-balancer-card__description">
                        {lb.description || '暂无描述'}
                      </div>
                      <div className="load-balancer-card__stats">
                        <div className="load-balancer-card__stat">
                          <span className="load-balancer-card__stat-label">配置节点</span>
                          <span className="load-balancer-card__stat-value">{lb.config_nodes?.length || 0}</span>
                        </div>
                        <div className="load-balancer-card__stat">
                          <span className="load-balancer-card__stat-label">最近更新</span>
                          <span className="load-balancer-card__stat-value">{formatDateTime(lb.updated_at || lb.created_at)}</span>
                        </div>
                      </div>
                      <div className="load-balancer-card__chips">
                        <Tag className="load-balancer-card__chip" color={lb.health_check_enabled ? 'green' : 'default'}>
                          健康检查{lb.health_check_interval ? ` ${lb.health_check_interval}s` : ''}
                        </Tag>
                        <Tag className="load-balancer-card__chip" color={lb.circuit_breaker_enabled ? 'geekblue' : 'default'}>
                          熔断{lb.circuit_breaker_timeout ? ` ${lb.circuit_breaker_timeout}s` : ''}
                        </Tag>
                        <Tag className="load-balancer-card__chip" color={lb.dynamic_weight_enabled ? 'purple' : 'default'}>
                          动态权重{lb.weight_update_interval ? ` ${lb.weight_update_interval}s` : ''}
                        </Tag>
                        <Tag className="load-balancer-card__chip" color={lb.max_retries ? 'gold' : 'default'}>
                          重试 {lb.max_retries ?? 0}
                        </Tag>
                      </div>
                      <div className="load-balancer-card__footer">
                        <span>日志级别: {lb.log_level || '默认'}</span>
                        <span>状态: {lb.enabled ? '启用' : '禁用'}</span>
                      </div>
                    </div>
                  }
                />
              </Card>
            </Col>
          ))}
        </Row>
      )}
    </Card>
  );
};

export default LoadBalancerList;
