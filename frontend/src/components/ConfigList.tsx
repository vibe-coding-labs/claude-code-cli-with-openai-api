import React, { useState, useEffect } from 'react';
import {
  Table,
  Button,
  Space,
  Tag,
  Popconfirm,
  message,
  Card,
  Typography,
  Tooltip,
  Switch,
} from 'antd';
import {
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  ThunderboltOutlined,
  CopyOutlined,
  StarOutlined,
  StarFilled,
} from '@ant-design/icons';
import { configAPI } from '../services/api';
import { APIConfig } from '../types/api';
import ConfigModal from './ConfigModal';

const { Title } = Typography;

const ConfigList: React.FC = () => {
  const [configs, setConfigs] = useState<APIConfig[]>([]);
  const [defaultConfigId, setDefaultConfigId] = useState<string>('');
  const [loading, setLoading] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [editingConfig, setEditingConfig] = useState<APIConfig | null>(null);

  const loadConfigs = async () => {
    setLoading(true);
    try {
      const response = await configAPI.listConfigs();
      setConfigs(response.configs);
      setDefaultConfigId(response.default_config_id || '');
    } catch (error: any) {
      message.error('加载配置失败: ' + (error.response?.data?.error?.message || error.message));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadConfigs();
  }, []);

  const handleCreate = () => {
    setEditingConfig(null);
    setModalVisible(true);
  };

  const handleEdit = (config: APIConfig) => {
    setEditingConfig(config);
    setModalVisible(true);
  };

  const handleDelete = async (id: string) => {
    try {
      await configAPI.deleteConfig(id);
      message.success('配置已删除');
      loadConfigs();
    } catch (error: any) {
      message.error('删除配置失败: ' + (error.response?.data?.error?.message || error.message));
    }
  };

  const handleTest = async (id: string) => {
    try {
      const response = await configAPI.testConfig(id);
      if (response.status === 'success') {
        message.success('测试成功: ' + response.message);
      } else {
        message.error('测试失败: ' + (response.error || response.message));
      }
      loadConfigs();
    } catch (error: any) {
      message.error('测试失败: ' + (error.response?.data?.error?.message || error.message));
    }
  };

  const handleSetDefault = async (id: string) => {
    try {
      await configAPI.setDefaultConfig(id);
      message.success('默认配置已设置');
      loadConfigs();
    } catch (error: any) {
      message.error('设置默认配置失败: ' + (error.response?.data?.error?.message || error.message));
    }
  };

  const handleCopyClaudeConfig = async (id: string, name: string) => {
    try {
      const claudeConfig = await configAPI.getClaudeConfig(id);
      const configText = `ANTHROPIC_BASE_URL=${claudeConfig.ANTHROPIC_BASE_URL}\nANTHROPIC_API_KEY=${claudeConfig.ANTHROPIC_API_KEY}`;
      await navigator.clipboard.writeText(configText);
      message.success('Claude配置已复制到剪贴板');
    } catch (error: any) {
      message.error('获取Claude配置失败: ' + (error.response?.data?.error?.message || error.message));
    }
  };

  const handleToggleEnabled = async (config: APIConfig, enabled: boolean) => {
    try {
      await configAPI.updateConfig(config.id, {
        ...config,
        enabled,
      });
      message.success(enabled ? '配置已启用' : '配置已禁用');
      loadConfigs();
    } catch (error: any) {
      message.error('更新配置失败: ' + (error.response?.data?.error?.message || error.message));
    }
  };

  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      render: (text: string, record: APIConfig) => (
        <Space>
          {text}
          {record.id === defaultConfigId && (
            <Tag color="gold" icon={<StarFilled />}>
              默认
            </Tag>
          )}
          {!record.enabled && <Tag color="default">已禁用</Tag>}
        </Space>
      ),
    },
    {
      title: '描述',
      dataIndex: 'description',
      key: 'description',
      ellipsis: true,
    },
    {
      title: '状态',
      key: 'status',
      render: (_: any, record: APIConfig) => {
        if (record.last_test_status === 'success') {
          return (
            <Tag color="success" icon={<CheckCircleOutlined />}>
              测试通过
            </Tag>
          );
        } else if (record.last_test_status === 'failed') {
          return (
            <Tooltip title={record.last_test_error}>
              <Tag color="error" icon={<CloseCircleOutlined />}>
                测试失败
              </Tag>
            </Tooltip>
          );
        }
        return <Tag>未测试</Tag>;
      },
    },
    {
      title: '模型',
      key: 'models',
      render: (_: any, record: APIConfig) => (
        <Space direction="vertical" size="small">
          <Tag>大: {record.big_model}</Tag>
          <Tag>中: {record.middle_model}</Tag>
          <Tag>小: {record.small_model}</Tag>
        </Space>
      ),
    },
    {
      title: '操作',
      key: 'action',
      width: 300,
      render: (_: any, record: APIConfig) => (
        <Space>
          <Tooltip title="编辑">
            <Button
              type="link"
              icon={<EditOutlined />}
              onClick={() => handleEdit(record)}
            />
          </Tooltip>
          <Tooltip title="测试">
            <Button
              type="link"
              icon={<ThunderboltOutlined />}
              onClick={() => handleTest(record.id)}
            />
          </Tooltip>
          <Tooltip title="复制Claude配置">
            <Button
              type="link"
              icon={<CopyOutlined />}
              onClick={() => handleCopyClaudeConfig(record.id, record.name)}
            />
          </Tooltip>
          {record.id !== defaultConfigId && (
            <Tooltip title="设为默认">
              <Button
                type="link"
                icon={<StarOutlined />}
                onClick={() => handleSetDefault(record.id)}
              />
            </Tooltip>
          )}
          <Switch
            checked={record.enabled}
            onChange={(checked) => handleToggleEnabled(record, checked)}
            size="small"
          />
          <Popconfirm
            title="确定要删除这个配置吗？"
            onConfirm={() => handleDelete(record.id)}
            okText="确定"
            cancelText="取消"
          >
            <Tooltip title="删除">
              <Button
                type="link"
                danger
                icon={<DeleteOutlined />}
              />
            </Tooltip>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <Card>
      <Space style={{ marginBottom: 16, width: '100%', justifyContent: 'space-between' }}>
        <Title level={4} style={{ margin: 0 }}>
          配置管理
        </Title>
        <Button
          type="primary"
          icon={<PlusOutlined />}
          onClick={handleCreate}
        >
          新建配置
        </Button>
      </Space>
      <Table
        columns={columns}
        dataSource={configs}
        rowKey="id"
        loading={loading}
        pagination={{ pageSize: 10 }}
      />
      <ConfigModal
        visible={modalVisible}
        config={editingConfig}
        onCancel={() => {
          setModalVisible(false);
          setEditingConfig(null);
        }}
        onSuccess={() => {
          setModalVisible(false);
          setEditingConfig(null);
          loadConfigs();
        }}
      />
    </Card>
  );
};

export default ConfigList;

