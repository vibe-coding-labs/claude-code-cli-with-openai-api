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
} from 'antd';
import {
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  EyeOutlined,
  ApiOutlined,
} from '@ant-design/icons';
import { loadBalancerApi, LoadBalancer } from '../services/loadBalancerApi';

const { Title } = Typography;

const LoadBalancerList: React.FC = () => {
  const navigate = useNavigate();
  const [loadBalancers, setLoadBalancers] = useState<LoadBalancer[]>([]);
  const [loading, setLoading] = useState(false);

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

  const handleDelete = async (id: string) => {
    try {
      await loadBalancerApi.delete(id);
      message.success('负载均衡器已删除');
      loadData();
    } catch (error: any) {
      message.error('删除失败');
      console.error('Failed to delete load balancer:', error);
    }
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
    <Card>
      <Space style={{ marginBottom: 16, width: '100%', justifyContent: 'space-between' }}>
        <Title level={4} style={{ margin: 0 }}>
          负载均衡器
        </Title>
        <Button
          type="primary"
          icon={<PlusOutlined />}
          onClick={() => navigate('/ui/load-balancers/create')}
        >
          新建负载均衡器
        </Button>
      </Space>
      <Table
        columns={columns}
        dataSource={loadBalancers}
        rowKey="id"
        loading={loading}
        pagination={{ pageSize: 10 }}
      />
    </Card>
  );
};

export default LoadBalancerList;
