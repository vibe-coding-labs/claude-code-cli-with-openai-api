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
  Alert,
  CircularProgress,
  Tooltip,
} from '@mui/material';
import {
  Add as AddIcon,
  Refresh as RefreshIcon,
  Block as BlockIcon,
  ContentCopy as CopyIcon,
} from '@mui/icons-material';
import { apiKeyService, APIKey } from '../services/apiKeyService';

interface APIKeyManagerProps {
  tenantId: string;
}

const APIKeyManager: React.FC<APIKeyManagerProps> = ({ tenantId }) => {
  const [apiKeys, setApiKeys] = useState<APIKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [newKeyData, setNewKeyData] = useState({ name: '', expires_at: '' });
  const [createdKey, setCreatedKey] = useState<string | null>(null);
  const [revokeDialogOpen, setRevokeDialogOpen] = useState(false);
  const [selectedKey, setSelectedKey] = useState<APIKey | null>(null);

  useEffect(() => {
    loadApiKeys();
  }, [tenantId]);

  const loadApiKeys = async () => {
    try {
      setLoading(true);
      const response = await apiKeyService.listApiKeys(tenantId);
      setApiKeys(response.keys);
      setError(null);
    } catch (err) {
      setError('Failed to load API keys');
    } finally {
      setLoading(false);
    }
  };

  const handleCreateKey = async () => {
    try {
      const expiresAt = newKeyData.expires_at ? new Date(newKeyData.expires_at) : undefined;
      const response = await apiKeyService.createApiKey(tenantId, {
        name: newKeyData.name,
        expires_at: expiresAt,
      });
      setCreatedKey(response.key.plain_key);
      setNewKeyData({ name: '', expires_at: '' });
      loadApiKeys();
    } catch (err) {
      setError('Failed to create API key');
    }
  };

  const handleRevokeKey = async () => {
    if (!selectedKey) return;
    try {
      await apiKeyService.revokeApiKey(selectedKey.id);
      setRevokeDialogOpen(false);
      setSelectedKey(null);
      loadApiKeys();
    } catch (err) {
      setError('Failed to revoke API key');
    }
  };

  const handleRotateKey = async (keyId: string) => {
    try {
      await apiKeyService.rotateApiKey(keyId, 86400); // 24 hours grace period
      loadApiKeys();
    } catch (err) {
      setError('Failed to rotate API key');
    }
  };

  const getStatusChip = (status: string, expiresAt?: string) => {
    if (status === 'revoked') {
      return <Chip label="Revoked" color="error" size="small" />;
    }
    if (expiresAt && new Date(expiresAt) < new Date()) {
      return <Chip label="Expired" color="warning" size="small" />;
    }
    return <Chip label="Active" color="success" size="small" />;
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
  };

  if (loading && apiKeys.length === 0) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="200px">
        <CircularProgress />
      </Box>
    );
  }

  return (
    <Box>
      <Box display="flex" justifyContent="space-between" alignItems="center" mb={2}>
        <Typography variant="h6">API Keys</Typography>
        <Button
          variant="contained"
          startIcon={<AddIcon />}
          onClick={() => setCreateDialogOpen(true)}
        >
          Create API Key
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
              <TableCell>Name</TableCell>
              <TableCell>Status</TableCell>
              <TableCell>Created At</TableCell>
              <TableCell>Expires At</TableCell>
              <TableCell>Last Used</TableCell>
              <TableCell align="right">Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {apiKeys.map((key) => (
              <TableRow key={key.id}>
                <TableCell>{key.name}</TableCell>
                <TableCell>{getStatusChip(key.status, key.expires_at)}</TableCell>
                <TableCell>{new Date(key.created_at).toLocaleDateString()}</TableCell>
                <TableCell>
                  {key.expires_at
                    ? new Date(key.expires_at).toLocaleDateString()
                    : 'Never'}
                </TableCell>
                <TableCell>
                  {key.last_used_at
                    ? new Date(key.last_used_at).toLocaleDateString()
                    : 'Never'}
                </TableCell>
                <TableCell align="right">
                  <Tooltip title="Rotate Key">
                    <IconButton
                      onClick={() => handleRotateKey(key.id)}
                      disabled={key.status !== 'active'}
                    >
                      <RefreshIcon />
                    </IconButton>
                  </Tooltip>
                  <Tooltip title="Revoke Key">
                    <IconButton
                      onClick={() => {
                        setSelectedKey(key);
                        setRevokeDialogOpen(true);
                      }}
                      disabled={key.status !== 'active'}
                      color="error"
                    >
                      <BlockIcon />
                    </IconButton>
                  </Tooltip>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>

      {/* Create Key Dialog */}
      <Dialog
        open={createDialogOpen}
        onClose={() => {
          setCreateDialogOpen(false);
          setCreatedKey(null);
        }}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Create API Key</DialogTitle>
        <DialogContent>
          {createdKey ? (
            <Box>
              <Alert severity="success" sx={{ mb: 2 }}>
                API Key created successfully! Copy this key now as it will not be shown again.
              </Alert>
              <Paper sx={{ p: 2, bgcolor: 'grey.100' }}>
                <Typography
                  variant="body2"
                  component="code"
                  sx={{ wordBreak: 'break-all', fontFamily: 'monospace' }}
                >
                  {createdKey}
                </Typography>
              </Paper>
              <Button
                fullWidth
                variant="outlined"
                startIcon={<CopyIcon />}
                onClick={() => copyToClipboard(createdKey)}
                sx={{ mt: 2 }}
              >
                Copy to Clipboard
              </Button>
            </Box>
          ) : (
            <Box>
              <TextField
                label="Key Name"
                fullWidth
                margin="normal"
                value={newKeyData.name}
                onChange={(e) => setNewKeyData({ ...newKeyData, name: e.target.value })}
              />
              <TextField
                label="Expiration Date (Optional)"
                type="datetime-local"
                fullWidth
                margin="normal"
                value={newKeyData.expires_at}
                onChange={(e) => setNewKeyData({ ...newKeyData, expires_at: e.target.value })}
                InputLabelProps={{ shrink: true }}
              />
            </Box>
          )}
        </DialogContent>
        <DialogActions>
          {!createdKey ? (
            <>
              <Button onClick={() => setCreateDialogOpen(false)}>Cancel</Button>
              <Button
                onClick={handleCreateKey}
                variant="contained"
                disabled={!newKeyData.name}
              >
                Create
              </Button>
            </>
          ) : (
            <Button
              onClick={() => {
                setCreateDialogOpen(false);
                setCreatedKey(null);
              }}
              variant="contained"
            >
              Done
            </Button>
          )}
        </DialogActions>
      </Dialog>

      {/* Revoke Dialog */}
      <Dialog open={revokeDialogOpen} onClose={() => setRevokeDialogOpen(false)}>
        <DialogTitle>Revoke API Key</DialogTitle>
        <DialogContent>
          Are you sure you want to revoke the API key &quot;{selectedKey?.name}&quot;? This action cannot be undone.
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setRevokeDialogOpen(false)}>Cancel</Button>
          <Button onClick={handleRevokeKey} color="error" variant="contained">
            Revoke
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

export default APIKeyManager;
