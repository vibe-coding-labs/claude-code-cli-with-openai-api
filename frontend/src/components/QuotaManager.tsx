import React, { useState, useEffect } from 'react';
import {
  Box,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
  Button,
  LinearProgress,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  MenuItem,
  FormControl,
  InputLabel,
  Select,
  Alert,
  CircularProgress,
  Chip,
} from '@mui/material';
import { Add as AddIcon, Refresh as RefreshIcon } from '@mui/icons-material';
import { quotaService, Quota } from '../services/quotaService';

interface QuotaManagerProps {
  tenantId: string;
}

const QuotaManager: React.FC<QuotaManagerProps> = ({ tenantId }) => {
  const [quotas, setQuotas] = useState<Quota[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [newQuota, setNewQuota] = useState({
    quota_type: 'requests',
    period: 'daily',
    limit: 1000,
  });

  useEffect(() => {
    loadQuotas();
  }, [tenantId]);

  const loadQuotas = async () => {
    try {
      setLoading(true);
      const response = await quotaService.getQuotas(tenantId);
      setQuotas(response.quotas);
      setError(null);
    } catch (err) {
      setError('Failed to load quotas');
    } finally {
      setLoading(false);
    }
  };

  const handleCreateQuota = async () => {
    try {
      await quotaService.setQuota(tenantId, newQuota);
      setCreateDialogOpen(false);
      setNewQuota({ quota_type: 'requests', period: 'daily', limit: 1000 });
      loadQuotas();
    } catch (err) {
      setError('Failed to create quota');
    }
  };

  const handleResetQuota = async (quotaType: string) => {
    try {
      await quotaService.resetQuota(tenantId, quotaType);
      loadQuotas();
    } catch (err) {
      setError('Failed to reset quota');
    }
  };

  const getUsagePercentage = (usage: number, limit: number) => {
    return Math.min((usage / limit) * 100, 100);
  };

  const getProgressColor = (percentage: number) => {
    if (percentage >= 90) return 'error';
    if (percentage >= 75) return 'warning';
    return 'success';
  };

  const formatNumber = (num: number) => {
    return new Intl.NumberFormat().format(num);
  };

  if (loading && quotas.length === 0) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="200px">
        <CircularProgress />
      </Box>
    );
  }

  return (
    <Box>
      <Box display="flex" justifyContent="space-between" alignItems="center" mb={2}>
        <Typography variant="h6">Quotas</Typography>
        <Button
          variant="contained"
          startIcon={<AddIcon />}
          onClick={() => setCreateDialogOpen(true)}
        >
          Set Quota
        </Button>
      </Box>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}

      <TableContainer component={Paper}>
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>Type</TableCell>
              <TableCell>Period</TableCell>
              <TableCell>Usage / Limit</TableCell>
              <TableCell>Progress</TableCell>
              <TableCell>Reset At</TableCell>
              <TableCell align="right">Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {quotas.map((quota) => {
              const percentage = getUsagePercentage(
                quota.current_usage,
                quota.limit
              );
              return (
                <TableRow key={`${quota.quota_type}-${quota.period}`}>
                  <TableCell>
                    <Chip label={quota.quota_type} size="small" />
                  </TableCell>
                  <TableCell>{quota.period}</TableCell>
                  <TableCell>
                    {formatNumber(quota.current_usage)} / {formatNumber(quota.limit)}
                  </TableCell>
                  <TableCell sx={{ minWidth: 150 }}>
                    <Box display="flex" alignItems="center" gap={1}>
                      <LinearProgress
                        variant="determinate"
                        value={percentage}
                        color={getProgressColor(percentage)}
                        sx={{ flexGrow: 1 }}
                      />
                      <Typography variant="caption">{percentage.toFixed(1)}%</Typography>
                    </Box>
                  </TableCell>
                  <TableCell>
                    {new Date(quota.reset_at).toLocaleDateString()}
                  </TableCell>
                  <TableCell align="right">
                    <Button
                      size="small"
                      startIcon={<RefreshIcon />}
                      onClick={() => handleResetQuota(quota.quota_type)}
                    >
                      Reset
                    </Button>
                  </TableCell>
                </TableRow>
              );
            })}
            {quotas.length === 0 && (
              <TableRow>
                <TableCell colSpan={6} align="center">
                  <Typography color="text.secondary" py={2}>
                    No quotas configured for this tenant
                  </Typography>
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </TableContainer>

      {/* Create Quota Dialog */}
      <Dialog open={createDialogOpen} onClose={() => setCreateDialogOpen(false)}>
        <DialogTitle>Set Quota</DialogTitle>
        <DialogContent>
          <FormControl fullWidth margin="normal">
            <InputLabel>Quota Type</InputLabel>
            <Select
              value={newQuota.quota_type}
              onChange={(e) =>
                setNewQuota({ ...newQuota, quota_type: e.target.value })
              }
              label="Quota Type"
            >
              <MenuItem value="requests">Requests</MenuItem>
              <MenuItem value="tokens">Tokens</MenuItem>
              <MenuItem value="cost">Cost</MenuItem>
            </Select>
          </FormControl>

          <FormControl fullWidth margin="normal">
            <InputLabel>Period</InputLabel>
            <Select
              value={newQuota.period}
              onChange={(e) =>
                setNewQuota({ ...newQuota, period: e.target.value })
              }
              label="Period"
            >
              <MenuItem value="daily">Daily</MenuItem>
              <MenuItem value="monthly">Monthly</MenuItem>
            </Select>
          </FormControl>

          <TextField
            label="Limit"
            type="number"
            fullWidth
            margin="normal"
            value={newQuota.limit}
            onChange={(e) =>
              setNewQuota({ ...newQuota, limit: parseInt(e.target.value) || 0 })
            }
            helperText={
              newQuota.quota_type === 'cost'
                ? 'Cost limit in cents (e.g., 1000 = $10.00)'
                : 'Maximum allowed value'
            }
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setCreateDialogOpen(false)}>Cancel</Button>
          <Button
            onClick={handleCreateQuota}
            variant="contained"
            disabled={newQuota.limit <= 0}
          >
            Set Quota
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

export default QuotaManager;
