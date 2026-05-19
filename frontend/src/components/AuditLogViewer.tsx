import React, { useState, useEffect, useCallback } from 'react';
import {
  Table,
  Card,
  Typography,
  Tag,
  Select,
  Input,
  Space,
  Button,
  Modal,
  Empty,
  Tooltip,
} from 'antd';
import {
  SearchOutlined,
  ReloadOutlined,
  InfoCircleOutlined,
  SafetyCertificateOutlined,
  CloseOutlined,
} from '@ant-design/icons';
import { auditLogService, AuditEvent } from '../services/auditLogService';
import './AuditLogViewer.css';

const { Title } = Typography;

const eventTypeColors: Record<string, string> = {
  authentication: 'blue',
  ip_filter: 'orange',
  rate_limit: 'red',
  quota: 'purple',
  api_key: 'cyan',
};

const resultColors: Record<string, string> = {
  success: 'success',
  failure: 'error',
};

const AuditLogViewer: React.FC = () => {
  const [logs, setLogs] = useState<AuditEvent[]>([]);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(25);
  const [filters, setFilters] = useState({
    event_type: '',
    result: '',
    tenant_id: '',
  });
  const [selectedLog, setSelectedLog] = useState<AuditEvent | null>(null);

  const loadLogs = useCallback(async () => {
    setLoading(true);
    try {
      const response = await auditLogService.queryEvents({
        ...filters,
        limit: pageSize,
        offset: (page - 1) * pageSize,
      });
      setLogs(response.audit_logs || []);
    } catch {
      setLogs([]);
    } finally {
      setLoading(false);
    }
  }, [filters, page, pageSize]);

  useEffect(() => {
    loadLogs();
  }, [loadLogs]);

  const formatTimestamp = (timestamp: string) => {
    return new Date(timestamp).toLocaleString();
  };

  const columns = [
    {
      title: 'Timestamp',
      dataIndex: 'timestamp',
      key: 'timestamp',
      width: 180,
      render: (ts: string) => (
        <span style={{ color: 'var(--color-text-secondary)', fontSize: 13 }}>
          {formatTimestamp(ts)}
        </span>
      ),
    },
    {
      title: 'Event Type',
      dataIndex: 'event_type',
      key: 'event_type',
      width: 140,
      render: (type: string) => (
        <Tag color={eventTypeColors[type] || 'default'}>{type}</Tag>
      ),
    },
    {
      title: 'Actor',
      dataIndex: 'actor',
      key: 'actor',
      width: 140,
      ellipsis: true,
    },
    {
      title: 'Action',
      dataIndex: 'action',
      key: 'action',
      width: 130,
      ellipsis: true,
    },
    {
      title: 'Resource',
      dataIndex: 'resource',
      key: 'resource',
      width: 140,
      ellipsis: true,
    },
    {
      title: 'Result',
      dataIndex: 'result',
      key: 'result',
      width: 100,
      render: (result: string) => (
        <Tag color={resultColors[result] || 'default'}>{result}</Tag>
      ),
    },
    {
      title: 'IP Address',
      dataIndex: 'ip_address',
      key: 'ip_address',
      width: 140,
      render: (ip: string) => (
        <span style={{ fontFamily: 'monospace', fontSize: 12, color: 'var(--color-text-secondary)' }}>
          {ip}
        </span>
      ),
    },
    {
      title: '',
      key: 'actions',
      width: 48,
      render: (_: unknown, record: AuditEvent) => (
        <Tooltip title="View Details">
          <Button
            type="text"
            size="small"
            icon={<InfoCircleOutlined />}
            onClick={() => setSelectedLog(record)}
            style={{ color: 'var(--color-accent)' }}
          />
        </Tooltip>
      ),
    },
  ];

  return (
    <Card className="audit-log-viewer">
      <div className="audit-log-viewer__header">
        <div className="audit-log-viewer__title-wrap">
          <Title level={4} className="audit-log-viewer__title">
            <SafetyCertificateOutlined style={{ marginRight: 8, color: 'var(--color-accent)' }} />
            Audit Logs
          </Title>
          <span className="audit-log-viewer__subtitle">Security event history and audit trail</span>
        </div>
        <Button
          icon={<ReloadOutlined />}
          onClick={loadLogs}
          loading={loading}
        >
          Refresh
        </Button>
      </div>

      <div className="audit-log-viewer__filters">
        <Select
          placeholder="Event Type"
          allowClear
          style={{ width: 160 }}
          value={filters.event_type || undefined}
          onChange={(value) => {
            setFilters(prev => ({ ...prev, event_type: value || '' }));
            setPage(1);
          }}
          options={[
            { value: 'authentication', label: 'Authentication' },
            { value: 'ip_filter', label: 'IP Filter' },
            { value: 'rate_limit', label: 'Rate Limit' },
            { value: 'quota', label: 'Quota' },
            { value: 'api_key', label: 'API Key' },
          ]}
        />
        <Select
          placeholder="Result"
          allowClear
          style={{ width: 120 }}
          value={filters.result || undefined}
          onChange={(value) => {
            setFilters(prev => ({ ...prev, result: value || '' }));
            setPage(1);
          }}
          options={[
            { value: 'success', label: 'Success' },
            { value: 'failure', label: 'Failure' },
          ]}
        />
        <Input
          placeholder="Tenant ID"
          prefix={<SearchOutlined />}
          allowClear
          style={{ width: 200 }}
          value={filters.tenant_id}
          onChange={(e) => {
            setFilters(prev => ({ ...prev, tenant_id: e.target.value }));
            setPage(1);
          }}
        />
      </div>

      <Card className="audit-log-viewer__table-card">
        <Table
          dataSource={logs}
          columns={columns}
          rowKey="id"
          loading={loading}
          size="middle"
          pagination={{
            current: page,
            pageSize,
            showSizeChanger: true,
            pageSizeOptions: ['10', '25', '50', '100'],
            showTotal: (total) => `${total} events`,
            onChange: (newPage, newPageSize) => {
              setPage(newPage);
              setPageSize(newPageSize);
            },
          }}
          locale={{
            emptyText: (
              <div className="audit-log-viewer__empty">
                <Empty description="No audit logs found" />
              </div>
            ),
          }}
        />
      </Card>

      <Modal
        title="Audit Log Details"
        open={!!selectedLog}
        onCancel={() => setSelectedLog(null)}
        footer={
          <Button icon={<CloseOutlined />} onClick={() => setSelectedLog(null)}>
            Close
          </Button>
        }
        width={640}
        className="audit-log-viewer__detail-modal"
        destroyOnClose
      >
        {selectedLog && (
          <div>
            {[
              { label: 'ID', value: selectedLog.id },
              { label: 'Timestamp', value: formatTimestamp(selectedLog.timestamp) },
              { label: 'Event Type', value: <Tag color={eventTypeColors[selectedLog.event_type] || 'default'}>{selectedLog.event_type}</Tag> },
              { label: 'Actor', value: selectedLog.actor },
              { label: 'Action', value: selectedLog.action },
              { label: 'Resource', value: selectedLog.resource },
              { label: 'Result', value: <Tag color={resultColors[selectedLog.result] || 'default'}>{selectedLog.result}</Tag> },
              { label: 'IP Address', value: selectedLog.ip_address },
              { label: 'Tenant ID', value: selectedLog.tenant_id || 'N/A' },
            ].map((item) => (
              <div key={item.label} className="audit-log-viewer__detail-row">
                <span className="audit-log-viewer__detail-label">{item.label}</span>
                <span className="audit-log-viewer__detail-value">{item.value}</span>
              </div>
            ))}
            {selectedLog.details && (
              <div style={{ marginTop: 16 }}>
                <span className="audit-log-viewer__detail-label">Details</span>
                <div className="audit-log-viewer__detail-json">
                  {(() => {
                    try {
                      return JSON.stringify(JSON.parse(selectedLog.details), null, 2);
                    } catch {
                      return selectedLog.details;
                    }
                  })()}
                </div>
              </div>
            )}
          </div>
        )}
      </Modal>
    </Card>
  );
};

export default AuditLogViewer;
