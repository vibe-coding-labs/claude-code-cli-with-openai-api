import React, { useState } from 'react';
import {
  Box,
  Paper,
  Typography,
  TextField,
  Button,
  Alert,
  Stepper,
  Step,
  StepLabel,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
} from '@mui/material';
import { useNavigate } from 'react-router-dom';
import { tenantService } from '../services/tenantService';

const steps = ['Basic Info', 'Configuration', 'Review'];

const TenantCreate: React.FC = () => {
  const navigate = useNavigate();
  const [activeStep, setActiveStep] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [tenantData, setTenantData] = useState({
    name: '',
    description: '',
    status: 'active',
    metadata: '',
  });
  const [configData, setConfigData] = useState({
    default_model: '',
    custom_rate_limits: false,
    require_hmac: false,
    webhook_url: '',
    alert_email: '',
  });

  const handleNext = () => {
    setActiveStep((prev) => prev + 1);
  };

  const handleBack = () => {
    setActiveStep((prev) => prev - 1);
  };

  const handleCreate = async () => {
    try {
      setLoading(true);
      const response = await tenantService.createTenant({
        name: tenantData.name,
        description: tenantData.description,
        status: tenantData.status,
        metadata: tenantData.metadata,
      });

      // Update tenant config if provided
      if (response.tenant.id) {
        await tenantService.updateTenantConfig(response.tenant.id, configData);
      }

      navigate(`/admin/tenants/${response.tenant.id}`);
    } catch (err) {
      setError('Failed to create tenant');
      setLoading(false);
    }
  };

  const isStepValid = () => {
    switch (activeStep) {
      case 0:
        return tenantData.name.length >= 3;
      case 1:
        return true;
      default:
        return true;
    }
  };

  const renderStepContent = () => {
    switch (activeStep) {
      case 0:
        return (
          <Box display="grid" gap={3}>
            <TextField
              label="Tenant Name"
              required
              value={tenantData.name}
              onChange={(e) =>
                setTenantData({ ...tenantData, name: e.target.value })
              }
              helperText="Minimum 3 characters"
              fullWidth
            />
            <TextField
              label="Description"
              value={tenantData.description}
              onChange={(e) =>
                setTenantData({ ...tenantData, description: e.target.value })
              }
              multiline
              rows={3}
              fullWidth
            />
            <FormControl fullWidth>
              <InputLabel>Status</InputLabel>
              <Select
                value={tenantData.status}
                onChange={(e) =>
                  setTenantData({ ...tenantData, status: e.target.value })
                }
                label="Status"
              >
                <MenuItem value="active">Active</MenuItem>
                <MenuItem value="suspended">Suspended</MenuItem>
              </Select>
            </FormControl>
            <TextField
              label="Metadata (JSON)"
              value={tenantData.metadata}
              onChange={(e) =>
                setTenantData({ ...tenantData, metadata: e.target.value })
              }
              multiline
              rows={3}
              helperText="Optional JSON metadata"
              fullWidth
            />
          </Box>
        );
      case 1:
        return (
          <Box display="grid" gap={3}>
            <TextField
              label="Default Model"
              value={configData.default_model}
              onChange={(e) =>
                setConfigData({ ...configData, default_model: e.target.value })
              }
              helperText="Default model for this tenant"
              fullWidth
            />
            <FormControl fullWidth>
              <InputLabel>Custom Rate Limits</InputLabel>
              <Select
                value={configData.custom_rate_limits.toString()}
                onChange={(e) =>
                  setConfigData({
                    ...configData,
                    custom_rate_limits: e.target.value === 'true',
                  })
                }
                label="Custom Rate Limits"
              >
                <MenuItem value="false">Disabled</MenuItem>
                <MenuItem value="true">Enabled</MenuItem>
              </Select>
            </FormControl>
            <FormControl fullWidth>
              <InputLabel>Require HMAC</InputLabel>
              <Select
                value={configData.require_hmac.toString()}
                onChange={(e) =>
                  setConfigData({
                    ...configData,
                    require_hmac: e.target.value === 'true',
                  })
                }
                label="Require HMAC"
              >
                <MenuItem value="false">Optional</MenuItem>
                <MenuItem value="true">Required</MenuItem>
              </Select>
            </FormControl>
            <TextField
              label="Webhook URL"
              value={configData.webhook_url}
              onChange={(e) =>
                setConfigData({ ...configData, webhook_url: e.target.value })
              }
              helperText="URL for webhook notifications"
              fullWidth
            />
            <TextField
              label="Alert Email"
              value={configData.alert_email}
              onChange={(e) =>
                setConfigData({ ...configData, alert_email: e.target.value })
              }
              helperText="Email for alerts and notifications"
              fullWidth
            />
          </Box>
        );
      case 2:
        return (
          <Box>
            <Typography variant="h6" mb={2}>
              Review
            </Typography>
            <Paper sx={{ p: 2, mb: 2 }}>
              <Typography variant="subtitle2" color="text.secondary">
                Name
              </Typography>
              <Typography mb={1}>{tenantData.name}</Typography>
              <Typography variant="subtitle2" color="text.secondary">
                Description
              </Typography>
              <Typography mb={1}>
                {tenantData.description || 'None'}
              </Typography>
              <Typography variant="subtitle2" color="text.secondary">
                Status
              </Typography>
              <Typography mb={1}>{tenantData.status}</Typography>
            </Paper>
            <Paper sx={{ p: 2 }}>
              <Typography variant="subtitle2" color="text.secondary">
                Default Model
              </Typography>
              <Typography mb={1}>
                {configData.default_model || 'Not set'}
              </Typography>
              <Typography variant="subtitle2" color="text.secondary">
                Custom Rate Limits
              </Typography>
              <Typography mb={1}>
                {configData.custom_rate_limits ? 'Enabled' : 'Disabled'}
              </Typography>
              <Typography variant="subtitle2" color="text.secondary">
                Require HMAC
              </Typography>
              <Typography mb={1}>
                {configData.require_hmac ? 'Required' : 'Optional'}
              </Typography>
            </Paper>
          </Box>
        );
      default:
        return null;
    }
  };

  return (
    <Box>
      <Typography variant="h4" mb={3}>
        Create Tenant
      </Typography>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}

      <Paper sx={{ p: 3 }}>
        <Stepper activeStep={activeStep} sx={{ mb: 4 }}>
          {steps.map((label) => (
            <Step key={label}>
              <StepLabel>{label}</StepLabel>
            </Step>
          ))}
        </Stepper>

        {renderStepContent()}

        <Box display="flex" justifyContent="space-between" mt={4}>
          <Button
            variant="outlined"
            onClick={() =>
              activeStep === 0 ? navigate('/admin/tenants') : handleBack()
            }
          >
            {activeStep === 0 ? 'Cancel' : 'Back'}
          </Button>
          <Button
            variant="contained"
            onClick={activeStep === steps.length - 1 ? handleCreate : handleNext}
            disabled={!isStepValid() || loading}
          >
            {activeStep === steps.length - 1
              ? loading
                ? 'Creating...'
                : 'Create'
              : 'Next'}
          </Button>
        </Box>
      </Paper>
    </Box>
  );
};

export default TenantCreate;
