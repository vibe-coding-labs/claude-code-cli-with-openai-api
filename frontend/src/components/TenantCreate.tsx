import React, { useState } from 'react';
import {
  Card,
  Typography,
  Input,
  Button,
  Space,
  Steps,
  Select,
  message,
  Alert,
} from 'antd';
import {
  ArrowLeftOutlined,
  ArrowRightOutlined,
  CheckOutlined,
} from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { tenantService } from '../services/tenantService';
import './TenantCreate.css';

const { Title } = Typography;
const { TextArea } = Input;

const TenantCreate: React.FC = () => {
  const navigate = useNavigate();
  const [activeStep, setActiveStep] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [tenantData, setTenantData] = useState({
    name: '',
    description: '',
    status: 'active' as string,
    metadata: '',
  });
  const [configData, setConfigData] = useState({
    default_model: '',
    custom_rate_limits: false,
    require_hmac: false,
    webhook_url: '',
    alert_email: '',
  });

  const steps = [
    { title: 'Basic Info', description: 'Name and status' },
    { title: 'Configuration', description: 'Model and limits' },
    { title: 'Review', description: 'Confirm details' },
  ];

  const isStepValid = () => {
    if (activeStep === 0) return tenantData.name.length >= 3;
    return true;
  };

  const handleCreate = async () => {
    setLoading(true);
    setError(null);
    try {
      const response = await tenantService.createTenant({
        name: tenantData.name,
        description: tenantData.description,
        status: tenantData.status,
        metadata: tenantData.metadata,
      });
      if (response.tenant?.id) {
        await tenantService.updateTenantConfig(response.tenant.id, configData).catch(() => {});
      }
      message.success('Tenant created successfully');
      navigate(`/ui/tenants/${response.tenant.id}`);
    } catch {
      setError('Failed to create tenant');
      setLoading(false);
    }
  };

  const renderBasicInfo = () => (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16, maxWidth: 520 }}>
      <div>
        <label style={{ display: 'block', marginBottom: 6, fontWeight: 500, fontSize: 13, color: 'var(--color-text-primary)' }}>
          Tenant Name <span style={{ color: 'var(--color-error)' }}>*</span>
        </label>
        <Input
          placeholder="Enter tenant name"
          value={tenantData.name}
          onChange={(e) => setTenantData({ ...tenantData, name: e.target.value })}
          status={tenantData.name.length > 0 && tenantData.name.length < 3 ? 'error' : undefined}
        />
        {tenantData.name.length > 0 && tenantData.name.length < 3 && (
          <span style={{ fontSize: 12, color: 'var(--color-error)' }}>Minimum 3 characters</span>
        )}
      </div>
      <div>
        <label style={{ display: 'block', marginBottom: 6, fontWeight: 500, fontSize: 13, color: 'var(--color-text-primary)' }}>
          Description
        </label>
        <TextArea
          placeholder="Optional description"
          value={tenantData.description}
          onChange={(e) => setTenantData({ ...tenantData, description: e.target.value })}
          rows={3}
        />
      </div>
      <div>
        <label style={{ display: 'block', marginBottom: 6, fontWeight: 500, fontSize: 13, color: 'var(--color-text-primary)' }}>
          Status
        </label>
        <Select
          value={tenantData.status}
          onChange={(value) => setTenantData({ ...tenantData, status: value })}
          style={{ width: '100%' }}
          options={[
            { value: 'active', label: 'Active' },
            { value: 'suspended', label: 'Suspended' },
          ]}
        />
      </div>
      <div>
        <label style={{ display: 'block', marginBottom: 6, fontWeight: 500, fontSize: 13, color: 'var(--color-text-primary)' }}>
          Metadata (JSON)
        </label>
        <TextArea
          placeholder='{"key": "value"}'
          value={tenantData.metadata}
          onChange={(e) => setTenantData({ ...tenantData, metadata: e.target.value })}
          rows={3}
        />
      </div>
    </div>
  );

  const renderConfiguration = () => (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16, maxWidth: 520 }}>
      <div>
        <label style={{ display: 'block', marginBottom: 6, fontWeight: 500, fontSize: 13, color: 'var(--color-text-primary)' }}>
          Default Model
        </label>
        <Input
          placeholder="e.g., gpt-4o"
          value={configData.default_model}
          onChange={(e) => setConfigData({ ...configData, default_model: e.target.value })}
        />
      </div>
      <div>
        <label style={{ display: 'block', marginBottom: 6, fontWeight: 500, fontSize: 13, color: 'var(--color-text-primary)' }}>
          Custom Rate Limits
        </label>
        <Select
          value={configData.custom_rate_limits ? 'true' : 'false'}
          onChange={(v) => setConfigData({ ...configData, custom_rate_limits: v === 'true' })}
          style={{ width: '100%' }}
          options={[
            { value: 'false', label: 'Disabled' },
            { value: 'true', label: 'Enabled' },
          ]}
        />
      </div>
      <div>
        <label style={{ display: 'block', marginBottom: 6, fontWeight: 500, fontSize: 13, color: 'var(--color-text-primary)' }}>
          Require HMAC
        </label>
        <Select
          value={configData.require_hmac ? 'true' : 'false'}
          onChange={(v) => setConfigData({ ...configData, require_hmac: v === 'true' })}
          style={{ width: '100%' }}
          options={[
            { value: 'false', label: 'Optional' },
            { value: 'true', label: 'Required' },
          ]}
        />
      </div>
      <div>
        <label style={{ display: 'block', marginBottom: 6, fontWeight: 500, fontSize: 13, color: 'var(--color-text-primary)' }}>
          Webhook URL
        </label>
        <Input
          placeholder="https://example.com/webhook"
          value={configData.webhook_url}
          onChange={(e) => setConfigData({ ...configData, webhook_url: e.target.value })}
        />
      </div>
      <div>
        <label style={{ display: 'block', marginBottom: 6, fontWeight: 500, fontSize: 13, color: 'var(--color-text-primary)' }}>
          Alert Email
        </label>
        <Input
          placeholder="admin@example.com"
          value={configData.alert_email}
          onChange={(e) => setConfigData({ ...configData, alert_email: e.target.value })}
        />
      </div>
    </div>
  );

  const renderReview = () => (
    <div>
      <div className="tenant-create__review-section">
        <div className="tenant-create__review-label">Name</div>
        <div className="tenant-create__review-value">{tenantData.name}</div>
      </div>
      <div className="tenant-create__review-section">
        <div className="tenant-create__review-label">Description</div>
        <div className="tenant-create__review-value">{tenantData.description || '—'}</div>
      </div>
      <div className="tenant-create__review-section">
        <div className="tenant-create__review-label">Status</div>
        <div className="tenant-create__review-value">{tenantData.status}</div>
      </div>
      <div className="tenant-create__review-section">
        <div className="tenant-create__review-label">Default Model</div>
        <div className="tenant-create__review-value">{configData.default_model || 'Not set'}</div>
      </div>
      <div className="tenant-create__review-section">
        <div className="tenant-create__review-label">Custom Rate Limits</div>
        <div className="tenant-create__review-value">{configData.custom_rate_limits ? 'Enabled' : 'Disabled'}</div>
      </div>
      <div className="tenant-create__review-section">
        <div className="tenant-create__review-label">Require HMAC</div>
        <div className="tenant-create__review-value">{configData.require_hmac ? 'Required' : 'Optional'}</div>
      </div>
    </div>
  );

  return (
    <Card className="tenant-create">
      <div className="tenant-create__header">
        <div className="tenant-create__title-wrap">
          <Title level={4} className="tenant-create__title">Create Tenant</Title>
          <span className="tenant-create__subtitle">Set up a new tenant with configuration</span>
        </div>
      </div>

      {error && <Alert type="error" message={error} style={{ marginBottom: 16 }} closable onClose={() => setError(null)} />}

      <Card className="tenant-create__form-card">
        <Steps current={activeStep} items={steps} className="tenant-create__steps" size="small" />

        {activeStep === 0 && renderBasicInfo()}
        {activeStep === 1 && renderConfiguration()}
        {activeStep === 2 && renderReview()}

        <div className="tenant-create__actions">
          <Button
            icon={<ArrowLeftOutlined />}
            onClick={() => activeStep === 0 ? navigate('/ui/tenants') : setActiveStep(activeStep - 1)}
          >
            {activeStep === 0 ? 'Cancel' : 'Back'}
          </Button>
          <Button
            type="primary"
            icon={activeStep === steps.length - 1 ? <CheckOutlined /> : <ArrowRightOutlined />}
            onClick={activeStep === steps.length - 1 ? handleCreate : () => setActiveStep(activeStep + 1)}
            disabled={!isStepValid() || loading}
            loading={loading && activeStep === steps.length - 1}
          >
            {activeStep === steps.length - 1 ? 'Create' : 'Next'}
          </Button>
        </div>
      </Card>
    </Card>
  );
};

export default TenantCreate;
