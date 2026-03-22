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
  Chip,
  IconButton,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Alert,
  CircularProgress,
} from '@mui/material';
import { Add as AddIcon, Delete as DeleteIcon } from '@mui/icons-material';
import { ipRuleService, IPRule } from '../services/ipRuleService';

interface IPRuleManagerProps {
  tenantId: string;
}

const IPRuleManager: React.FC<IPRuleManagerProps> = ({ tenantId }) => {
  const [rules, setRules] = useState<IPRule[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [newRule, setNewRule] = useState({
    rule_type: 'whitelist',
    ip_address: '',
    description: '',
  });
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [selectedRule, setSelectedRule] = useState<IPRule | null>(null);

  useEffect(() => {
    loadRules();
  }, [tenantId]);

  const loadRules = async () => {
    try {
      setLoading(true);
      const response = await ipRuleService.listRules(tenantId);
      setRules(response.ip_rules);
      setError(null);
    } catch (err) {
      setError('Failed to load IP rules');
    } finally {
      setLoading(false);
    }
  };

  const handleCreateRule = async () => {
    try {
      await ipRuleService.createRule(tenantId, newRule);
      setCreateDialogOpen(false);
      setNewRule({ rule_type: 'whitelist', ip_address: '', description: '' });
      loadRules();
    } catch (err) {
      setError('Failed to create IP rule');
    }
  };

  const handleDeleteRule = async () => {
    if (!selectedRule) return;
    try {
      await ipRuleService.deleteRule(selectedRule.id);
      setDeleteDialogOpen(false);
      setSelectedRule(null);
      loadRules();
    } catch (err) {
      setError('Failed to delete IP rule');
    }
  };

  const getRuleTypeChip = (ruleType: string) => {
    if (ruleType === 'whitelist') {
      return <Chip label="Whitelist" color="success" size="small" />;
    }
    return <Chip label="Blacklist" color="error" size="small" />;
  };

  if (loading && rules.length === 0) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="200px">
        <CircularProgress />
      </Box>
    );
  }

  return (
    <Box>
      <Box display="flex" justifyContent="space-between" alignItems="center" mb={2}>
        <Typography variant="h6">IP Rules</Typography>
        <Button
          variant="contained"
          startIcon={<AddIcon />}
          onClick={() => setCreateDialogOpen(true)}
        >
          Add IP Rule
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
              <TableCell>IP Address</TableCell>
              <TableCell>Description</TableCell>
              <TableCell>Created At</TableCell>
              <TableCell align="right">Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {rules.map((rule) => (
              <TableRow key={rule.id}>
                <TableCell>{getRuleTypeChip(rule.rule_type)}</TableCell>
                <TableCell>
                  <Typography fontFamily="monospace">{rule.ip_address}</Typography>
                </TableCell>
                <TableCell>{rule.description}</TableCell>
                <TableCell>{new Date(rule.created_at).toLocaleDateString()}</TableCell>
                <TableCell align="right">
                  <IconButton
                    onClick={() => {
                      setSelectedRule(rule);
                      setDeleteDialogOpen(true);
                    }}
                    color="error"
                  >
                    <DeleteIcon />
                  </IconButton>
                </TableCell>
              </TableRow>
            ))}
            {rules.length === 0 && (
              <TableRow>
                <TableCell colSpan={5} align="center">
                  <Typography color="text.secondary" py={2}>
                    No IP rules configured for this tenant
                  </Typography>
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </TableContainer>

      {/* Create Dialog */}
      <Dialog open={createDialogOpen} onClose={() => setCreateDialogOpen(false)}>
        <DialogTitle>Add IP Rule</DialogTitle>
        <DialogContent>
          <FormControl fullWidth margin="normal">
            <InputLabel>Rule Type</InputLabel>
            <Select
              value={newRule.rule_type}
              onChange={(e) =>
                setNewRule({ ...newRule, rule_type: e.target.value })
              }
              label="Rule Type"
            >
              <MenuItem value="whitelist">Whitelist</MenuItem>
              <MenuItem value="blacklist">Blacklist</MenuItem>
            </Select>
          </FormControl>

          <TextField
            label="IP Address / CIDR"
            fullWidth
            margin="normal"
            value={newRule.ip_address}
            onChange={(e) =>
              setNewRule({ ...newRule, ip_address: e.target.value })
            }
            placeholder="e.g., 192.168.1.1 or 10.0.0.0/24"
            helperText="Supports IPv4/IPv6 addresses and CIDR notation"
          />

          <TextField
            label="Description"
            fullWidth
            margin="normal"
            value={newRule.description}
            onChange={(e) =>
              setNewRule({ ...newRule, description: e.target.value })
            }
            placeholder="Optional description for this rule"
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setCreateDialogOpen(false)}>Cancel</Button>
          <Button
            onClick={handleCreateRule}
            variant="contained"
            disabled={!newRule.ip_address}
          >
            Add Rule
          </Button>
        </DialogActions>
      </Dialog>

      {/* Delete Dialog */}
      <Dialog open={deleteDialogOpen} onClose={() => setDeleteDialogOpen(false)}>
        <DialogTitle>Delete IP Rule</DialogTitle>
        <DialogContent>
          Are you sure you want to delete this IP rule?
          <Box mt={2}>
            <Typography variant="body2" color="text.secondary">
              Type: {selectedRule?.rule_type}
            </Typography>
            <Typography variant="body2" color="text.secondary">
              IP Address: {selectedRule?.ip_address}
            </Typography>
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeleteDialogOpen(false)}>Cancel</Button>
          <Button onClick={handleDeleteRule} color="error" variant="contained">
            Delete
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

export default IPRuleManager;
