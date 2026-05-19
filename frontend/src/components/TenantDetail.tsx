import React, { useState, useEffect } from 'react';
import {
  Card,
  Typography,
  Tag,
  Button,
  Space,
  Tabs,
  Input,
  Select,
  message,
  Descriptions,
  Spin,
  Result,
} from 'antd';
import {
  ArrowLeftOutlined,
  EditOutlined,
  SaveOutlined,
  CloseOutlined,
} from '@ant-design/icons';
import { useParams, useNavigate } from 'react-router-dom';
import { tenantService, Tenant, TenantConfig } from '../services/tenantService';
import APIKeyManager from './APIKeyManager';
import QuotaManager from './QuotaManager';
import IPRuleManager from './IPRuleManager';
import './TenantDetail.css';

const { Title } = Typography;

const statusColors: Record<string, string> = {
  active: 'success',
  suspended: 'error',
  deleted: 'default',
};

const TenantDetail: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [tenant, setTenant] = useState<Tenant | null>(null);
  const [config, setConfig] = useState<TenantConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [isEditing, setIsEditing] = useState(false);
  const [editData, setEditData] = useState<Partial<Tenant>>({});

  useEffect(() => {
    if (id) loadTenant();
  }, [id]);

  const loadTenant = async () => {
    setLoading(true);
    try {
      const [tenantRes, configRes] = await Promise.all([
        tenantService.getTenant(id!),
        tenantService.getTenantConfig(id!),
      ]);
      setTenant(tenantRes.tenant);
      setConfig(configRes.config);
      setEditData(tenantRes.tenant);
    } catch {
      message.error('Failed to load tenant details');
    } finally {
      setLoading(false);
    }
  };

  const handleSave = async () => {
    if (!tenant || !id) return;
    try {
      await tenantService.updateTenant(id, {
        name: editData.name || tenant.name,
        description: editData.description || '',
        status: editData.status || tenant.status,
        metadata: editData.metadata || '',
      });
      message.success('Tenant updated');
      setIsEditing(false);
      loadTenant();
    } catch {
      message.error('Failed to update tenant');
    }
  };

  if (loading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', padding: 120 }}>
        <Spin size="large" />
      </div>
    );
  }

  if (!tenant) {
    return (
      <Card style={{ borderRadius: 'var(--radius-lg)' }}>
        <Result
          status="404"
          title="Tenant not found"
          extra={
            <Button type="primary" onClick={() => navigate('/ui/tenants')}>
              Back to Tenants
            </Button>
          }
        />
      </Card>
    );
  }

  const renderEditForm = () => (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16, maxWidth: 480 }}>
      <div>
        <label style={{ display: 'block', marginBottom: 6, fontWeight: 500, fontSize: 13 }}>Name</label>
        <Input
          value={editData.name || ''}
          onChange={(e) => setEditData({ ...editData, name: e.target.value })}
        />
      </div>
      <div>
        <label style={{ display: 'block', marginBottom: 6, fontWeight: 500, fontSize: 13 }}>Description</label>
        <Input.TextArea
          value={editData.description || ''}
          onChange={(e) => setEditData({ ...editData, description: e.target.value })}
          rows={2}
        />
      </div>
      <div>
        <label style={{ display: 'block', marginBottom: 6, fontWeight: 500, fontSize: 13 }}>Status</label>
        <Select
          value={editData.status || tenant.status}
          onChange={(value) => setEditData({ ...editData, status: value })}
          style={{ width: '100%' }}
          options={[
            { value: 'active', label: 'Active' },
            { value: 'suspended', label: 'Suspended' },
            { value: 'deleted', label: 'Deleted' },
          ]}
        />
      </div>
    </div>
  );

  const renderInfo = () => (
    <Descriptions column={{ xs: 1, sm: 2 }} size="small" colon={false}>
      <Descriptions.Item label="Status">
        <Tag color={statusColors[tenant.status] || 'default'}>{tenant.status}</Tag>
      </Descriptions.Item>
      <Descriptions.Item label="ID">
        <span style={{ fontFamily: 'monospace', fontSize: 12, color: 'var(--color-text-tertiary)' }}>
          {tenant.id}
        </span>
      </Descriptions.Item>
      <Descriptions.Item label="Description" span={2}>
        {tenant.description || '—'}
      </Descriptions.Item>
      <Descriptions.Item label="Created">
        {new Date(tenant.created_at).toLocaleString()}
      </Descriptions.Item>
      <Descriptions.Item label="Updated">
        {new Date(tenant.updated_at).toLocaleString()}
      </Descriptions.Item>
    </Descriptions>
  );

  return (
    <div className="tenant-detail">
      <div className="tenant-detail__header">
        <div className="tenant-detail__title-wrap">
          <Title level={4} className="tenant-detail__title">{tenant.name}</Title>
          <span className="tenant-detail__id">{tenant.id}</span>
        </div>
        <Space>
          {isEditing ? (
            <>
              <Button icon={<CloseOutlined />} onClick={() => { setIsEditing(false); setEditData(tenant); }}>
                Cancel
              </Button>
              <Button type="primary" icon={<SaveOutlined />} onClick={handleSave}>
                Save
              </Button>
            </>
          ) : (
            <Button icon={<EditOutlined />} onClick={() => setIsEditing(true)}>
              Edit
            </Button>
          )}
          <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/ui/tenants')}>
            Back
          </Button>
        </Space>
      </div>

      <Card className="tenant-detail__card">
        <div className="tenant-detail__card-title">Details</div>
        {isEditing ? renderEditForm() : renderInfo()}
      </Card>

      {config && (
        <Card className="tenant-detail__card">
          <div className="tenant-detail__card-title">Configuration</div>
          <div className="tenant-detail__config-grid">
            <div className="tenant-detail__config-item">
              <div className="tenant-detail__config-label">Default Model</div>
              <div className="tenant-detail__config-value">{config.default_model || 'Not set'}</div>
            </div>
            <div className="tenant-detail__config-item">
              <div className="tenant-detail__config-label">Allowed Models</div>
              <div className="tenant-detail__config-value">
                {config.allowed_models?.length ? config.allowed_models.join(', ') : 'All models'}
              </div>
            </div>
            <div className="tenant-detail__config-item">
              <div className="tenant-detail__config-label">Custom Rate Limits</div>
              <div className="tenant-detail__config-value">
                <Tag color={config.custom_rate_limits ? 'success' : 'default'}>
                  {config.custom_rate_limits ? 'Enabled' : 'Disabled'}
                </Tag>
              </div>
            </div>
            <div className="tenant-detail__config-item">
              <div className="tenant-detail__config-label">Require HMAC</div>
              <div className="tenant-detail__config-value">
                <Tag color={config.require_hmac ? 'success' : 'default'}>
                  {config.require_hmac ? 'Required' : 'Optional'}
                </Tag>
              </div>
            </div>
            {config.webhook_url && (
              <div className="tenant-detail__config-item">
                <div className="tenant-detail__config-label">Webhook URL</div>
                <div className="tenant-detail__config-value" style={{ fontSize: 12, fontFamily: 'monospace' }}>
                  {config.webhook_url}
                </div>
              </div>
            )}
            {config.alert_email && (
              <div className="tenant-detail__config-item">
                <div className="tenant-detail__config-label">Alert Email</div>
                <div className="tenant-detail__config-value">{config.alert_email}</div>
              </div>
            )}
          </div>
        </Card>
      )}

      <Card className="tenant-detail__tabs-card">
        <Tabs
          defaultActiveKey="api-keys"
          items={[
            { key: 'api-keys', label: 'API Keys', children: <APIKeyManager tenantId={id!} /> },
            { key: 'quotas', label: 'Quotas', children: <QuotaManager tenantId={id!} /> },
            { key: 'ip-rules', label: 'IP Rules', children: <IPRuleManager tenantId={id!} /> },
          ]}
        />
      </Card>
    </div>
  );
};

export default TenantDetail;
