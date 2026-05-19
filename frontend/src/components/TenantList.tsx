import React, { useState, useEffect, useCallback } from 'react';
import {
  Table,
  Card,
  Typography,
  Tag,
  Select,
  Button,
  Space,
  Popconfirm,
  message,
  Empty,
  Tooltip,
} from 'antd';
import {
  PlusOutlined,
  EyeOutlined,
  DeleteOutlined,
  ReloadOutlined,
  TeamOutlined,
} from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { tenantService, Tenant } from '../services/tenantService';
import './TenantList.css';

const { Title } = Typography;

const statusColors: Record<string, string> = {
  active: 'success',
  suspended: 'error',
  deleted: 'default',
};

const TenantList: React.FC = () => {
  const navigate = useNavigate();
  const [tenants, setTenants] = useState<Tenant[]>([]);
  const [loading, setLoading] = useState(false);
  const [statusFilter, setStatusFilter] = useState<string>('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  const loadTenants = useCallback(async () => {
    setLoading(true);
    try {
      const filters = statusFilter ? { status: statusFilter } : {};
      const response = await tenantService.listTenants(filters);
      setTenants(response.tenants || []);
    } catch {
      message.error('Failed to load tenants');
    } finally {
      setLoading(false);
    }
  }, [statusFilter]);

  useEffect(() => {
    loadTenants();
  }, [loadTenants]);

  const handleDelete = async (tenant: Tenant) => {
    try {
      await tenantService.deleteTenant(tenant.id);
      message.success(`Tenant "${tenant.name}" deleted`);
      loadTenants();
    } catch {
      message.error('Failed to delete tenant');
    }
  };

  const columns = [
    {
      title: 'Name',
      dataIndex: 'name',
      key: 'name',
      render: (name: string) => (
        <span style={{ fontWeight: 500, color: 'var(--color-text-primary)' }}>{name}</span>
      ),
    },
    {
      title: 'Description',
      dataIndex: 'description',
      key: 'description',
      ellipsis: true,
      render: (desc: string) => (
        <span style={{ color: 'var(--color-text-secondary)' }}>{desc || '—'}</span>
      ),
    },
    {
      title: 'Status',
      dataIndex: 'status',
      key: 'status',
      width: 120,
      render: (status: string) => (
        <Tag color={statusColors[status] || 'default'}>{status}</Tag>
      ),
    },
    {
      title: 'Created',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 140,
      render: (date: string) => (
        <span style={{ color: 'var(--color-text-secondary)', fontSize: 13 }}>
          {new Date(date).toLocaleDateString()}
        </span>
      ),
    },
    {
      title: '',
      key: 'actions',
      width: 100,
      render: (_: unknown, record: Tenant) => (
        <Space size={4}>
          <Tooltip title="View Details">
            <Button
              type="text"
              size="small"
              icon={<EyeOutlined />}
              onClick={() => navigate(`/ui/tenants/${record.id}`)}
              className="tenant-list__action-btn"
            />
          </Tooltip>
          <Popconfirm
            title={`Delete "${record.name}"?`}
            description="This action cannot be undone."
            onConfirm={() => handleDelete(record)}
            okText="Delete"
            cancelText="Cancel"
            okButtonProps={{ danger: true }}
          >
            <Tooltip title="Delete">
              <Button
                type="text"
                size="small"
                icon={<DeleteOutlined />}
                className="tenant-list__action-btn tenant-list__action-btn--danger"
              />
            </Tooltip>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <Card className="tenant-list">
      <div className="tenant-list__header">
        <div className="tenant-list__title-wrap">
          <Title level={4} className="tenant-list__title">
            <TeamOutlined style={{ marginRight: 8, color: 'var(--color-accent)' }} />
            Tenants
          </Title>
          <span className="tenant-list__subtitle">
            Manage multi-tenant configurations and access
          </span>
        </div>
        <Space>
          <Button icon={<ReloadOutlined />} onClick={loadTenants} loading={loading}>
            Refresh
          </Button>
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={() => navigate('/ui/tenants/create')}
          >
            Create Tenant
          </Button>
        </Space>
      </div>

      <div className="tenant-list__filters">
        <Select
          placeholder="Status"
          allowClear
          style={{ width: 140 }}
          value={statusFilter || undefined}
          onChange={(value) => {
            setStatusFilter(value || '');
            setPage(1);
          }}
          options={[
            { value: 'active', label: 'Active' },
            { value: 'suspended', label: 'Suspended' },
            { value: 'deleted', label: 'Deleted' },
          ]}
        />
      </div>

      <Card className="tenant-list__table-card">
        <Table
          dataSource={tenants}
          columns={columns}
          rowKey="id"
          loading={loading}
          size="middle"
          pagination={{
            current: page,
            pageSize,
            showSizeChanger: true,
            pageSizeOptions: ['5', '10', '25', '50'],
            showTotal: (total) => `${total} tenants`,
            onChange: (newPage, newPageSize) => {
              setPage(newPage);
              setPageSize(newPageSize);
            },
          }}
          locale={{
            emptyText: (
              <div style={{ padding: '40px 20px' }}>
                <Empty description="No tenants found" />
              </div>
            ),
          }}
        />
      </Card>
    </Card>
  );
};

export default TenantList;
