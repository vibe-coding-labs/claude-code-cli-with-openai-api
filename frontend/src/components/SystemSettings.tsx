import React, { useState, useEffect } from 'react';
import { Card, Form, InputNumber, Button, message, Statistic, Row, Col, Space } from 'antd';
import { SettingOutlined, DatabaseOutlined } from '@ant-design/icons';
import api from '../services/api';

interface SystemSettingsData {
  log_retention_days?: string;
}

const SystemSettings: React.FC = () => {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [totalLogs, setTotalLogs] = useState<number>(0);

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
      setTotalLogs(resp.data.total_logs || 0);
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
      });
      message.success('Settings saved successfully');
      loadLogStats();
    } catch {
      message.error('Failed to save settings');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div>
      <h2 style={{ marginBottom: 24 }}><SettingOutlined /> System Settings</h2>
      <Row gutter={24}>
        <Col span={16}>
          <Card title="Log Retention" loading={loading}>
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
                <InputNumber min={1} max={365} style={{ width: '100%' }} />
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
        </Col>
        <Col span={8}>
          <Card title="Log Storage">
            <Statistic
              title="Total Request Logs"
              value={totalLogs}
              prefix={<DatabaseOutlined />}
            />
          </Card>
        </Col>
      </Row>
    </div>
  );
};

export default SystemSettings;
