import React, { useState, useEffect } from 'react';
import {
  Box,
  Typography,
  Paper,
  Tabs,
  Tab,
  Alert,
  CircularProgress,
  Button,
  TextField,
  Chip,
  Divider,
} from '@mui/material';
import { useParams, useNavigate } from 'react-router-dom';
import { tenantService, Tenant, TenantConfig } from '../services/tenantService';
import APIKeyManager from './APIKeyManager';
import QuotaManager from './QuotaManager';
import IPRuleManager from './IPRuleManager';

const TenantDetail: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [tenant, setTenant] = useState<Tenant | null>(null);
  const [config, setConfig] = useState<TenantConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState(0);
  const [isEditing, setIsEditing] = useState(false);
  const [editData, setEditData] = useState<Partial<Tenant>>({});

  useEffect(() => {
    if (id) {
      loadTenant();
    }
  }, [id]);

  const loadTenant = async () => {
    try {
      setLoading(true);
      const [tenantResponse, configResponse] = await Promise.all([
        tenantService.getTenant(id!),
        tenantService.getTenantConfig(id!),
      ]);
      setTenant(tenantResponse.tenant);
      setConfig(configResponse.config);
      setEditData(tenantResponse.tenant);
      setError(null);
    } catch (err) {
      setError('Failed to load tenant details');
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
      setIsEditing(false);
      loadTenant();
    } catch (err) {
      setError('Failed to update tenant');
    }
  };

  const getStatusChip = (status: string) => {
    const colorMap: { [key: string]: 'success' | 'error' | 'default' } = {
      active: 'success',
      suspended: 'error',
      deleted: 'default',
    };
    return <Chip label={status} color={colorMap[status] || 'default'} />;
  };

  if (loading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="400px">
        <CircularProgress />
      </Box>
    );
  }

  if (!tenant) {
    return (
      <Box>
        <Alert severity="error">Tenant not found</Alert>
        <Button onClick={() => navigate('/admin/tenants')} sx={{ mt: 2 }}>
          Back to Tenants
        </Button>
      </Box>
    );
  }

  return (
    <Box>
      <Box display="flex" justifyContent="space-between" alignItems="center" mb={3}>
        <Typography variant="h4">{tenant.name}</Typography>
        <Box display="flex" gap={1}>
          {isEditing ? (
            <>
              <Button variant="outlined" onClick={() => setIsEditing(false)}>
                Cancel
              </Button>
              <Button variant="contained" onClick={handleSave}>
                Save
              </Button>
            </>
          ) : (
            <Button variant="outlined" onClick={() => setIsEditing(true)}>
              Edit
            </Button>
          )}
          <Button variant="outlined" onClick={() => navigate('/admin/tenants')}>
            Back
          </Button>
        </Box>
      </Box>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}

      <Paper sx={{ mb: 3, p: 3 }}>
        {isEditing ? (
          <Box display="grid" gap={2}>
            <TextField
              label="Name"
              value={editData.name || ''}
              onChange={(e) => setEditData({ ...editData, name: e.target.value })}
              fullWidth
            />
            <TextField
              label="Description"
              value={editData.description || ''}
              onChange={(e) => setEditData({ ...editData, description: e.target.value })}
              fullWidth
              multiline
              rows={2}
            />
            <TextField
              label="Status"
              select
              value={editData.status || ''}
              onChange={(e) => setEditData({ ...editData, status: e.target.value })}
              fullWidth
            >
              <option value="active">Active</option>
              <option value="suspended">Suspended</option>
              <option value="deleted">Deleted</option>
            </TextField>
          </Box>
        ) : (
          <Box>
            <Box display="flex" justifyContent="space-between" mb={2}>
              <Typography variant="body1" color="text.secondary">
                Status
              </Typography>
              {getStatusChip(tenant.status)}
            </Box>
            <Divider sx={{ my: 1 }} />
            <Box mb={2}>
              <Typography variant="body2" color="text.secondary">
                Description
              </Typography>
              <Typography>{tenant.description || 'No description'}</Typography>
            </Box>
            <Box mb={2}>
              <Typography variant="body2" color="text.secondary">
                ID
              </Typography>
              <Typography fontFamily="monospace" variant="body2">
                {tenant.id}
              </Typography>
            </Box>
            <Box display="flex" gap={4}>
              <Box>
                <Typography variant="body2" color="text.secondary">
                  Created At
                </Typography>
                <Typography>
                  {new Date(tenant.created_at).toLocaleString()}
                </Typography>
              </Box>
              <Box>
                <Typography variant="body2" color="text.secondary">
                  Updated At
                </Typography>
                <Typography>
                  {new Date(tenant.updated_at).toLocaleString()}
                </Typography>
              </Box>
            </Box>
          </Box>
        )}
      </Paper>

      {config && (
        <Paper sx={{ mb: 3, p: 3 }}>
          <Typography variant="h6" mb={2}>
            Configuration
          </Typography>
          <Box display="grid" gridTemplateColumns="1fr 1fr" gap={2}>
            <Box>
              <Typography variant="body2" color="text.secondary">
                Default Model
              </Typography>
              <Typography>{config.default_model || 'Not set'}</Typography>
            </Box>
            <Box>
              <Typography variant="body2" color="text.secondary">
                Allowed Models
              </Typography>
              <Typography>
                {config.allowed_models?.length
                  ? config.allowed_models.join(', ')
                  : 'All models'}
              </Typography>
            </Box>
            <Box>
              <Typography variant="body2" color="text.secondary">
                Custom Rate Limits
              </Typography>
              <Chip
                label={config.custom_rate_limits ? 'Enabled' : 'Disabled'}
                color={config.custom_rate_limits ? 'success' : 'default'}
                size="small"
              />
            </Box>
            <Box>
              <Typography variant="body2" color="text.secondary">
                Require HMAC
              </Typography>
              <Chip
                label={config.require_hmac ? 'Required' : 'Optional'}
                color={config.require_hmac ? 'success' : 'default'}
                size="small"
              />
            </Box>
            {config.webhook_url && (
              <Box>
                <Typography variant="body2" color="text.secondary">
                  Webhook URL
                </Typography>
                <Typography variant="body2">{config.webhook_url}</Typography>
              </Box>
            )}
            {config.alert_email && (
              <Box>
                <Typography variant="body2" color="text.secondary">
                  Alert Email
                </Typography>
                <Typography variant="body2">{config.alert_email}</Typography>
              </Box>
            )}
          </Box>
        </Paper>
      )}

      <Paper sx={{ mb: 3 }}>
        <Tabs
          value={activeTab}
          onChange={(_, newValue) => setActiveTab(newValue)}
          sx={{ borderBottom: 1, borderColor: 'divider' }}
        >
          <Tab label="API Keys" />
          <Tab label="Quotas" />
          <Tab label="IP Rules" />
        </Tabs>
        <Box p={3}>
          {activeTab === 0 && <APIKeyManager tenantId={id!} />}
          {activeTab === 1 && <QuotaManager tenantId={id!} />}
          {activeTab === 2 && <IPRuleManager tenantId={id!} />}
        </Box>
      </Paper>
    </Box>
  );
};

export default TenantDetail;
