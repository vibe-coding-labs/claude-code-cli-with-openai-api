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
  TablePagination,
  Typography,
  TextField,
  MenuItem,
  FormControl,
  InputLabel,
  Select,
  Chip,
  Alert,
  CircularProgress,
  IconButton,
  Tooltip,
  Dialog,
  DialogTitle,
  DialogContent,
} from '@mui/material';
import { Info as InfoIcon } from '@mui/icons-material';
import { auditLogService, AuditEvent } from '../services/auditLogService';

const AuditLogViewer: React.FC = () => {
  const [logs, setLogs] = useState<AuditEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(25);
  const [filters, setFilters] = useState({
    event_type: '',
    result: '',
    tenant_id: '',
  });
  const [selectedLog, setSelectedLog] = useState<AuditEvent | null>(null);

  useEffect(() => {
    loadLogs();
  }, [filters]);

  const loadLogs = async () => {
    try {
      setLoading(true);
      const response = await auditLogService.queryEvents({
        ...filters,
        limit: rowsPerPage,
        offset: page * rowsPerPage,
      });
      setLogs(response.audit_logs);
      setError(null);
    } catch (err) {
      setError('Failed to load audit logs');
    } finally {
      setLoading(false);
    }
  };

  const handleChangePage = (event: unknown, newPage: number) => {
    setPage(newPage);
    loadLogs();
  };

  const handleChangeRowsPerPage = (event: React.ChangeEvent<HTMLInputElement>) => {
    setRowsPerPage(parseInt(event.target.value, 10));
    setPage(0);
    loadLogs();
  };

  const getResultChip = (result: string) => {
    if (result === 'success') {
      return <Chip label="Success" color="success" size="small" />;
    }
    return <Chip label={result} color="error" size="small" />;
  };

  const formatTimestamp = (timestamp: string) => {
    return new Date(timestamp).toLocaleString();
  };

  if (loading && logs.length === 0) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="400px">
        <CircularProgress />
      </Box>
    );
  }

  return (
    <Box>
      <Typography variant="h4" mb={3}>
        Audit Logs
      </Typography>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}

      <Paper sx={{ mb: 2, p: 2 }}>
        <Box display="flex" gap={2}>
          <FormControl sx={{ minWidth: 150 }}>
            <InputLabel>Event Type</InputLabel>
            <Select
              value={filters.event_type}
              onChange={(e) => setFilters({ ...filters, event_type: e.target.value })}
              label="Event Type"
            >
              <MenuItem value="">All</MenuItem>
              <MenuItem value="authentication">Authentication</MenuItem>
              <MenuItem value="ip_filter">IP Filter</MenuItem>
              <MenuItem value="rate_limit">Rate Limit</MenuItem>
              <MenuItem value="quota">Quota</MenuItem>
              <MenuItem value="api_key">API Key</MenuItem>
            </Select>
          </FormControl>

          <FormControl sx={{ minWidth: 120 }}>
            <InputLabel>Result</InputLabel>
            <Select
              value={filters.result}
              onChange={(e) => setFilters({ ...filters, result: e.target.value })}
              label="Result"
            >
              <MenuItem value="">All</MenuItem>
              <MenuItem value="success">Success</MenuItem>
              <MenuItem value="failure">Failure</MenuItem>
            </Select>
          </FormControl>

          <TextField
            label="Tenant ID"
            value={filters.tenant_id}
            onChange={(e) => setFilters({ ...filters, tenant_id: e.target.value })}
          />
        </Box>
      </Paper>

      <TableContainer component={Paper}>
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>Timestamp</TableCell>
              <TableCell>Event Type</TableCell>
              <TableCell>Actor</TableCell>
              <TableCell>Action</TableCell>
              <TableCell>Resource</TableCell>
              <TableCell>Result</TableCell>
              <TableCell>IP Address</TableCell>
              <TableCell align="right">Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {logs.map((log) => (
              <TableRow key={log.id}>
                <TableCell>{formatTimestamp(log.timestamp)}</TableCell>
                <TableCell>
                  <Chip label={log.event_type} size="small" variant="outlined" />
                </TableCell>
                <TableCell>{log.actor}</TableCell>
                <TableCell>{log.action}</TableCell>
                <TableCell>{log.resource}</TableCell>
                <TableCell>{getResultChip(log.result)}</TableCell>
                <TableCell>{log.ip_address}</TableCell>
                <TableCell align="right">
                  <Tooltip title="View Details">
                    <IconButton onClick={() => setSelectedLog(log)}>
                      <InfoIcon />
                    </IconButton>
                  </Tooltip>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        <TablePagination
          component="div"
          count={-1}
          page={page}
          onPageChange={handleChangePage}
          rowsPerPage={rowsPerPage}
          onRowsPerPageChange={handleChangeRowsPerPage}
          rowsPerPageOptions={[10, 25, 50, 100]}
        />
      </TableContainer>

      {/* Detail Dialog */}
      <Dialog
        open={!!selectedLog}
        onClose={() => setSelectedLog(null)}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle>Audit Log Details</DialogTitle>
        <DialogContent>
          {selectedLog && (
            <Box>
              <Typography variant="body2" color="text.secondary">
                ID: {selectedLog.id}
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Timestamp: {formatTimestamp(selectedLog.timestamp)}
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Tenant ID: {selectedLog.tenant_id || 'N/A'}
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Event Type: {selectedLog.event_type}
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Actor: {selectedLog.actor}
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Action: {selectedLog.action}
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Resource: {selectedLog.resource}
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Result: {selectedLog.result}
              </Typography>
              <Typography variant="body2" color="text.secondary">
                IP Address: {selectedLog.ip_address}
              </Typography>
              {selectedLog.details && (
                <Box mt={2}>
                  <Typography variant="subtitle2">Details:</Typography>
                  <Paper sx={{ p: 2, bgcolor: 'grey.100' }}>
                    <pre style={{ margin: 0, overflow: 'auto' }}>
                      {JSON.stringify(JSON.parse(selectedLog.details), null, 2)}
                    </pre>
                  </Paper>
                </Box>
              )}
            </Box>
          )}
        </DialogContent>
      </Dialog>
    </Box>
  );
};

export default AuditLogViewer;
