import React, { useState, useEffect } from 'react';
import {
  Card, Form, InputNumber, Button, message, Statistic, Row, Col, Space,
  Progress, Tooltip, Select, Divider, Alert
} from 'antd';
import {
  SettingOutlined, DatabaseOutlined, DeleteOutlined,
  CompressOutlined, CloudUploadOutlined, HardDriveOutlined
} from '@ant-design/icons';
import api from '../services/api';
import type { SystemSettingsData, LogStorageStats } from '../types/settings';

const formatBytes = (bytes: number): string => {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
};

const SystemSettings: React.FC = () => {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [stats, setStats] = useState<LogStorageStats | null>(null);
  const [cleaning, setCleaning] = useState(false);
  const [vacuuming, setVacuuming] = useState(false);
  const [migrating, setMigrating] = useState(false);

  useEffect(() => {
    loadSettings();
    loadLogStats();
  }, []);

  const loadSettings = async () => {
    setLoading(true);
    try {
      const resp = await api.get('/api/settings');
      const data: SystemSettingsData = resp.data;
      form.setFieldsValue({
        log_retention_days: parseInt(data.log_retention_days || '30', 10),
        max_db_size_gb: parseInt(data.max_db_size_gb || '10', 10),
        log_body_storage: data.log_body_storage || 'file',
        proxy_error_retention_days: parseInt(data.proxy_error_retention_days || '30', 10),
      });
    } catch {
      message.error('Failed to load settings');
    } finally {
      setLoading(false);
    }
  };

  const loadLogStats = async () => {
    try {
      const resp = await api.get('/api/log-stats');
      setStats(resp.data);
    } catch {
      // ignore
    }
  };

  const handleSave = async () => {
    try {
      const values = await form.validateFields();
      setSaving(true);
      await api.put('/api/settings', {
        log_retention_days: String(values.log_retention_days),
        max_db_size_gb: String(values.max_db_size_gb),
        log_body_storage: values.log_body_storage,
        proxy_error_retention_days: String(values.proxy_error_retention_days),
      });
      message.success('Settings saved successfully');
      loadLogStats();
    } catch {
      message.error('Failed to save settings');
    } finally {
      setSaving(false);
    }
  };

  const handleCleanup = async () => {
    setCleaning(true);
    try {
      await api.post('/api/cleanup');
      message.success('Cleanup triggered successfully');
      setTimeout(loadLogStats, 3000);
    } catch {
      message.error('Failed to trigger cleanup');
    } finally {
      setCleaning(false);
    }
  };

  const handleVacuum = async () => {
    setVacuuming(true);
    try {
      const resp = await api.post('/api/vacuum');
      message.success(`VACUUM completed: ${formatBytes(resp.data.new_size_bytes)}`);
      loadLogStats();
    } catch {
      message.error('Failed to run VACUUM');
    } finally {
      setVacuuming(false);
    }
  };

  const handleMigrate = async () => {
    setMigrating(true);
    try {
      const resp = await api.post('/api/migrate-bodies');
      message.success(`Migrated ${resp.data.migrated} bodies to file storage`);
      loadLogStats();
    } catch {
      message.error('Failed to migrate bodies');
    } finally {
      setMigrating(false);
    }
  };

  const maxDbSizeGB = form.getFieldValue('max_db_size_gb') || 10;
  const dbUsagePercent = stats && stats.total_size_bytes !== undefined
    ? Math.min(100, Math.round((stats.total_size_bytes) / (maxDbSizeGB * 1024 * 1024 * 1024) * 100))
    : 0;

  return (
    <div>
      <h2 style={{ marginBottom: 24 }}><SettingOutlined /> System Settings</h2>
      <Row gutter={24}>
        <Col span={16}>
          <Card title="Log Retention & Storage" loading={loading}>
            <Form form={form} layout="vertical">
              <Form.Item
                name="log_retention_days"
                label="Request Log Retention (days)"
                rules={[
                  { required: true, message: 'Please enter retention days' },
                  { type: 'number', min: 1, max: 365, message: 'Must be between 1-365 days' },
                ]}
                extra="Logs older than this will be automatically deleted daily at 2:00 AM"
              >
                <InputNumber min={1} max={365} style={{ width: '100%' }} addonAfter="days" />
              </Form.Item>

              <Form.Item
                name="proxy_error_retention_days"
                label="Proxy Error Retention (days)"
                rules={[
                  { required: true, message: 'Please enter retention days' },
                  { type: 'number', min: 1, max: 365, message: 'Must be between 1-365 days' },
                ]}
                extra="Proxy error records older than this will be automatically deleted"
              >
                <InputNumber min={1} max={365} style={{ width: '100%' }} addonAfter="days" />
              </Form.Item>

              <Form.Item
                name="max_db_size_gb"
                label="Max Database Size (GB)"
                rules={[
                  { required: true, message: 'Please enter max database size' },
                  { type: 'number', min: 1, max: 1000, message: 'Must be between 1-1000 GB' },
                ]}
                extra="When the database exceeds this size, the oldest logs will be automatically deleted"
              >
                <InputNumber min={1} max={1000} style={{ width: '100%' }} addonAfter="GB" />
              </Form.Item>

              <Form.Item
                name="log_body_storage"
                label="Body Storage Mode"
                extra="'file' stores request/response bodies in separate compressed files (recommended, keeps DB small). 'inline' stores bodies directly in the database (legacy, causes DB bloat)."
              >
                <Select>
                  <Select.Option value="file">File Storage (Recommended)</Select.Option>
                  <Select.Option value="inline">Inline Storage (Legacy)</Select.Option>
                </Select>
              </Form.Item>

              <Form.Item>
                <Space>
                  <Button type="primary" onClick={handleSave} loading={saving}>
                    Save Settings
                  </Button>
                </Space>
              </Form.Item>
            </Form>
          </Card>

          <Card title="Maintenance Actions" style={{ marginTop: 24 }}>
            <Space direction="vertical" style={{ width: '100%' }} size="middle">
              <Alert
                message="These actions run in the background and may take a few minutes for large databases."
                type="info"
                showIcon
              />
              <Space wrap>
                <Tooltip title="Delete old logs and reclaim space">
                  <Button
                    icon={<DeleteOutlined />}
                    onClick={handleCleanup}
                    loading={cleaning}
                  >
                    Run Cleanup Now
                  </Button>
                </Tooltip>
                <Tooltip title="Compact the database file to reclaim disk space (safe, no data loss)">
                  <Button
                    icon={<CompressOutlined />}
                    onClick={handleVacuum}
                    loading={vacuuming}
                  >
                    VACUUM Database
                  </Button>
                </Tooltip>
                <Tooltip title="Migrate inline bodies to file storage (run after switching to file mode)">
                  <Button
                    icon={<CloudUploadOutlined />}
                    onClick={handleMigrate}
                    loading={migrating}
                  >
                    Migrate Bodies to Files
                  </Button>
                </Tooltip>
              </Space>
            </Space>
          </Card>
        </Col>

        <Col span={8}>
          <Card title="Storage Overview">
            <Space direction="vertical" style={{ width: '100%' }} size="large">
              <Statistic
                title="Total Request Logs"
                value={stats?.total_logs || 0}
                prefix={<DatabaseOutlined />}
              />
              <Divider style={{ margin: '8px 0' }} />
              <Statistic
                title="Database Size"
                value={formatBytes(stats?.total_size_bytes || 0)}
                prefix={<HardDriveOutlined />}
              />
              <Statistic
                title="Body Files Size"
                value={formatBytes(stats?.body_size_bytes || 0)}
                prefix={<CloudUploadOutlined />}
              />
              {stats && stats.total_size_bytes !== undefined && (
                <>
                  <div>
                    <div style={{ marginBottom: 4, fontSize: 12, color: '#888' }}>
                      DB Usage: {dbUsagePercent}% of {maxDbSizeGB} GB limit
                    </div>
                    <Progress
                      percent={dbUsagePercent}
                      status={dbUsagePercent > 90 ? 'exception' : dbUsagePercent > 70 ? 'active' : 'normal'}
                      size="small"
                    />
                  </div>
                  {stats.free_pages !== undefined && stats.total_pages !== undefined && stats.total_pages > 0 && (
                    <div style={{ fontSize: 12, color: '#888' }}>
                      Free pages: {stats.free_pages} / {stats.total_pages}
                      ({Math.round(stats.free_pages / stats.total_pages * 100)}% reclaimable via VACUUM)
                    </div>
                  )}
                </>
              )}
              {stats?.db_path && (
                <div style={{ fontSize: 11, color: '#aaa', wordBreak: 'break-all' }}>
                  Path: {stats.db_path}
                </div>
              )}
            </Space>
          </Card>
        </Col>
      </Row>
    </div>
  );
};

export default SystemSettings;
