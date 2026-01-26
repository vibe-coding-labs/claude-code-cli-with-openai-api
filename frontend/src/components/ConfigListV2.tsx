/* eslint-disable @typescript-eslint/no-unused-vars */
import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { usePageTitle } from '../utils/pageTitle';
import { getPreference, setPreference, PREFERENCE_KEYS } from '../utils/storage';
import {
  Table,
  Button,
  Space,
  Tag,
  Popconfirm,
  message,
  Card,
  Typography,
  Select,
  Row,
  Col,
  Input,
  Tooltip,
  Switch,
} from 'antd';
import {
  PlusOutlined,
  DeleteOutlined,
  EyeOutlined,
  SearchOutlined,
  ReloadOutlined,
  FilterOutlined,
  AppstoreOutlined,
  UnorderedListOutlined,
  EditOutlined,
} from '@ant-design/icons';
import axios from 'axios';

const { Title, Text } = Typography;
const { Option } = Select;

interface Config {
  id: string;
  name: string;
  description: string;
  openai_api_key_masked: string;
  openai_base_url: string;
  big_model: string;
  middle_model: string;
  small_model: string;
  supported_models?: string[];
  max_tokens_limit: number;
  request_timeout: number;
  anthropic_api_key?: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

const ConfigListV2: React.FC = () => {
  usePageTitle('配置列表');
  const navigate = useNavigate();
  const [configs, setConfigs] = useState<Config[]>([]);
  const [filteredConfigs, setFilteredConfigs] = useState<Config[]>([]);
  const [loading, setLoading] = useState(false);
  
  // View mode: 'card' or 'list'
  const [viewMode, setViewMode] = useState<'card' | 'list'>('list');
  
  // Filter states
  const [searchText, setSearchText] = useState('');
  const [statusFilter, setStatusFilter] = useState<string>('');
  const [modelFilter, setModelFilter] = useState<string>('');
  const [sortField, setSortField] = useState<string>('created_at');
  const [sortOrder, setSortOrder] = useState<'ascend' | 'descend'>('descend');

  const loadConfigs = async () => {
    setLoading(true);
    try {
      const response = await axios.get('/api/configs');
      setConfigs(response.data.configs || []);
    } catch (error: any) {
      message.error('加载配置失败');
    } finally {
      setLoading(false);
    }
  };

  // Load user preferences on mount
  useEffect(() => {
    const loadPreferences = async () => {
      try {
        const savedViewMode = await getPreference<'card' | 'list'>(
          PREFERENCE_KEYS.CONFIG_LIST_VIEW_MODE,
          'list'
        );
        const savedSortField = await getPreference<string>(
          PREFERENCE_KEYS.CONFIG_LIST_SORT_FIELD,
          'created_at'
        );
        const savedSortOrder = await getPreference<'ascend' | 'descend'>(
          PREFERENCE_KEYS.CONFIG_LIST_SORT_ORDER,
          'descend'
        );

        if (savedViewMode) {
          setViewMode(savedViewMode);
        }
        if (savedSortField) {
          setSortField(savedSortField);
        }
        if (savedSortOrder) {
          setSortOrder(savedSortOrder);
        }
      } catch (error) {
        console.error('Failed to load preferences:', error);
      } finally {
        // preferences loaded
      }
    };

    loadPreferences();
    loadConfigs();
  }, []);

  // Apply filters
  useEffect(() => {
    let result = [...configs];

    // Search filter
    if (searchText) {
      result = result.filter(config =>
        config.name.toLowerCase().includes(searchText.toLowerCase()) ||
        config.description?.toLowerCase().includes(searchText.toLowerCase()) ||
        config.openai_base_url.toLowerCase().includes(searchText.toLowerCase()) ||
        config.big_model.toLowerCase().includes(searchText.toLowerCase()) ||
        config.middle_model.toLowerCase().includes(searchText.toLowerCase()) ||
        config.small_model.toLowerCase().includes(searchText.toLowerCase())
      );
    }

    // Status filter
    if (statusFilter === 'enabled') {
      result = result.filter(config => config.enabled);
    } else if (statusFilter === 'disabled') {
      result = result.filter(config => !config.enabled);
    }

    // Model filter
    if (modelFilter) {
      result = result.filter(config =>
        config.big_model.toLowerCase().includes(modelFilter.toLowerCase()) ||
        config.middle_model.toLowerCase().includes(modelFilter.toLowerCase()) ||
        config.small_model.toLowerCase().includes(modelFilter.toLowerCase())
      );
    }

    // Sorting
    result.sort((a, b) => {
      let aVal: any = a[sortField as keyof Config];
      let bVal: any = b[sortField as keyof Config];

      if (sortField === 'created_at') {
        aVal = new Date(aVal).getTime();
        bVal = new Date(bVal).getTime();
      } else if (typeof aVal === 'string') {
        aVal = aVal.toLowerCase();
        bVal = (bVal as string).toLowerCase();
      }

      if (sortOrder === 'ascend') {
        return aVal > bVal ? 1 : -1;
      } else {
        return aVal < bVal ? 1 : -1;
      }
    });

    setFilteredConfigs(result);
  }, [configs, searchText, statusFilter, modelFilter, sortField, sortOrder]);

  const handleCreate = () => {
    navigate('/ui/configs/create');
  };

  const handleDelete = async (id: string, event?: React.MouseEvent) => {
    // 阻止事件冒泡，避免触发卡片点击
    event?.stopPropagation();
    try {
      await axios.delete(`/api/configs/${id}`);
      message.success('配置已删除');
      loadConfigs();
    } catch (error: any) {
      message.error('删除配置失败');
    }
  };

  const handleViewDetail = (id: string) => {
    navigate(`/ui/configs/${id}`);
  };

  const handleEdit = (id: string, event?: React.MouseEvent) => {
    event?.stopPropagation();
    navigate(`/ui/configs/${id}/edit`);
  };

  const handleToggleEnabled = async (id: string, enabled: boolean, event?: React.MouseEvent) => {
    event?.stopPropagation();
    try {
      await axios.put(`/api/configs/${id}`, { enabled });
      message.success(enabled ? '配置已启用' : '配置已禁用');
      loadConfigs();
    } catch (error: any) {
      message.error('更新状态失败');
    }
  };

  const handleResetFilters = async () => {
    setSearchText('');
    setStatusFilter('');
    setModelFilter('');
    setSortField('created_at');
    setSortOrder('descend');
    
    // Save reset preferences
    try {
      await setPreference(PREFERENCE_KEYS.CONFIG_LIST_SORT_FIELD, 'created_at');
      await setPreference(PREFERENCE_KEYS.CONFIG_LIST_SORT_ORDER, 'descend');
    } catch (error) {
      console.error('Failed to save preferences:', error);
    }
  };

  // Handle view mode change with preference saving
  const handleViewModeChange = async (mode: 'card' | 'list') => {
    setViewMode(mode);
    try {
      await setPreference(PREFERENCE_KEYS.CONFIG_LIST_VIEW_MODE, mode);
    } catch (error) {
      console.error('Failed to save view mode preference:', error);
    }
  };

  // Handle sort field change with preference saving
  const handleSortFieldChange = async (field: string) => {
    setSortField(field);
    try {
      await setPreference(PREFERENCE_KEYS.CONFIG_LIST_SORT_FIELD, field);
    } catch (error) {
      console.error('Failed to save sort field preference:', error);
    }
  };

  // Handle sort order change with preference saving
  const handleSortOrderChange = async (order: 'ascend' | 'descend') => {
    setSortOrder(order);
    try {
      await setPreference(PREFERENCE_KEYS.CONFIG_LIST_SORT_ORDER, order);
    } catch (error) {
      console.error('Failed to save sort order preference:', error);
    }
  };

  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      width: 200,
      render: (text: string, record: Config) => (
        <Space>
          <span style={{ fontWeight: 500 }}>{text}</span>
          {!record.enabled && <Tag color="default">已禁用</Tag>}
        </Space>
      ),
    },
    {
      title: '描述',
      dataIndex: 'description',
      key: 'description',
      ellipsis: true,
      render: (text: string) => text || '-',
    },
    {
      title: 'OpenAI API Key',
      dataIndex: 'openai_api_key_masked',
      key: 'openai_api_key_masked',
      width: 150,
      ellipsis: true,
      render: (text: string) => (
        <code style={{ fontSize: 11, background: '#f5f5f5', padding: '2px 6px', borderRadius: 3 }}>
          {text || '-'}
        </code>
      ),
    },
    {
      title: 'Base URL',
      dataIndex: 'openai_base_url',
      key: 'openai_base_url',
      ellipsis: true,
      width: 250,
    },
    {
      title: '大模型',
      dataIndex: 'big_model',
      key: 'big_model',
      width: 150,
      ellipsis: true,
      render: (text: string) => (
        <Tag color="purple" style={{ fontSize: 11 }}>{text}</Tag>
      ),
    },
    {
      title: '中模型',
      dataIndex: 'middle_model',
      key: 'middle_model',
      width: 150,
      ellipsis: true,
      render: (text: string) => (
        <Tag color="blue" style={{ fontSize: 11 }}>{text}</Tag>
      ),
    },
    {
      title: '小模型',
      dataIndex: 'small_model',
      key: 'small_model',
      width: 150,
      ellipsis: true,
      render: (text: string) => (
        <Tag color="cyan" style={{ fontSize: 11 }}>{text}</Tag>
      ),
    },
    {
      title: '支持的模型',
      dataIndex: 'supported_models',
      key: 'supported_models',
      width: 120,
      align: 'center' as const,
      render: (models: string[]) => (
        <Tooltip title={models && models.length > 0 ? models.join(', ') : '使用默认映射模型'}>
          <Tag color={models && models.length > 0 ? 'green' : 'default'}>
            {models && models.length > 0 ? `${models.length} 个` : '默认'}
          </Tag>
        </Tooltip>
      ),
    },
    {
      title: 'Token限制',
      dataIndex: 'max_tokens_limit',
      key: 'max_tokens_limit',
      width: 100,
      align: 'center' as const,
      render: (value: number) => (
        <span style={{ fontSize: 12 }}>{value?.toLocaleString() || '-'}</span>
      ),
    },
    {
      title: '超时(秒)',
      dataIndex: 'request_timeout',
      key: 'request_timeout',
      width: 90,
      align: 'center' as const,
    },
    {
      title: 'Anthropic Token',
      dataIndex: 'anthropic_api_key',
      key: 'anthropic_api_key',
      width: 150,
      ellipsis: true,
      render: (text: string) => (
        <code style={{ fontSize: 11, background: '#f5f5f5', padding: '2px 6px', borderRadius: 3 }}>
          {text || '-'}
        </code>
      ),
    },
    {
      title: '状态',
      dataIndex: 'enabled',
      key: 'enabled',
      width: 100,
      align: 'center' as const,
      render: (enabled: boolean, record: Config) => (
        <Switch
          checked={enabled}
          onChange={(checked, e) => handleToggleEnabled(record.id, checked, e as any)}
          checkedChildren="启用"
          unCheckedChildren="禁用"
          onClick={(checked, e) => e.stopPropagation()}
        />
      ),
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 160,
      render: (text: string) => (
        <span style={{ fontSize: 12 }}>{new Date(text).toLocaleString('zh-CN', { 
          year: 'numeric', 
          month: '2-digit', 
          day: '2-digit',
          hour: '2-digit',
          minute: '2-digit'
        })}</span>
      ),
    },
    {
      title: '更新时间',
      dataIndex: 'updated_at',
      key: 'updated_at',
      width: 160,
      render: (text: string) => (
        <span style={{ fontSize: 12 }}>{new Date(text).toLocaleString('zh-CN', { 
          year: 'numeric', 
          month: '2-digit', 
          day: '2-digit',
          hour: '2-digit',
          minute: '2-digit'
        })}</span>
      ),
    },
    {
      title: '操作',
      key: 'action',
      width: 150,
      fixed: 'right' as const,
      render: (_: any, record: Config) => (
        <Space>
          <Button
            type="link"
            icon={<EyeOutlined />}
            onClick={() => navigate(`/ui/configs/${record.id}`)}
          >
            详情
          </Button>
          <Popconfirm
            title="确定要删除这个配置吗？"
            onConfirm={() => handleDelete(record.id)}
            okText="确定"
            cancelText="取消"
          >
            <Button
              type="link"
              danger
              icon={<DeleteOutlined />}
            />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <Card>
      <Space style={{ marginBottom: 16, width: '100%', justifyContent: 'space-between' }}>
        <Title level={4} style={{ margin: 0 }}>
          OpenAI API配置
        </Title>
        <Space>
          <Space.Compact>
          <Tooltip title="卡片视图">
            <Button
              icon={<AppstoreOutlined />}
              type={viewMode === 'card' ? 'primary' : 'default'}
              onClick={() => handleViewModeChange('card')}
            />
          </Tooltip>
          <Tooltip title="列表视图">
            <Button
              icon={<UnorderedListOutlined />}
              type={viewMode === 'list' ? 'primary' : 'default'}
              onClick={() => handleViewModeChange('list')}
            />
          </Tooltip>
        </Space.Compact>
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={handleCreate}
          >
            新建配置
          </Button>
        </Space>
      </Space>

      {/* Filters */}
      <Card title="筛选和排序" size="small" style={{ marginBottom: 16 }}>
        <Row gutter={[16, 16]}>
          {/* 第一行：搜索和筛选 */}
          <Col xs={24} sm={12} md={8} lg={8}>
            <Space direction="vertical" size={4} style={{ width: '100%' }}>
              <Text strong style={{ fontSize: 12 }}>关键词搜索</Text>
              <Input
                placeholder="搜索名称、描述、URL、模型..."
                prefix={<SearchOutlined />}
                value={searchText}
                onChange={(e) => setSearchText(e.target.value)}
                allowClear
              />
            </Space>
          </Col>
          
          <Col xs={12} sm={6} md={4} lg={3}>
            <Space direction="vertical" size={4} style={{ width: '100%' }}>
              <Text strong style={{ fontSize: 12 }}>状态筛选</Text>
              <Select
                placeholder="全部状态"
                value={statusFilter}
                onChange={setStatusFilter}
                style={{ width: '100%' }}
                allowClear
              >
                <Option value="enabled">✓ 仅启用</Option>
                <Option value="disabled">✗ 仅禁用</Option>
              </Select>
            </Space>
          </Col>

          <Col xs={12} sm={6} md={5} lg={4}>
            <Space direction="vertical" size={4} style={{ width: '100%' }}>
              <Text strong style={{ fontSize: 12 }}>模型筛选</Text>
              <Input
                placeholder="输入模型名称"
                value={modelFilter}
                onChange={(e) => setModelFilter(e.target.value)}
                allowClear
              />
            </Space>
          </Col>

          <Col xs={12} sm={6} md={4} lg={3}>
            <Space direction="vertical" size={4} style={{ width: '100%' }}>
              <Text strong style={{ fontSize: 12 }}>排序字段</Text>
              <Select
                value={sortField}
                onChange={handleSortFieldChange}
                style={{ width: '100%' }}
              >
                <Option value="created_at">⏰ 创建时间</Option>
                <Option value="name">📝 配置名称</Option>
                <Option value="openai_base_url">🔗 Base URL</Option>
              </Select>
            </Space>
          </Col>

          <Col xs={12} sm={6} md={3} lg={2}>
            <Space direction="vertical" size={4} style={{ width: '100%' }}>
              <Text strong style={{ fontSize: 12 }}>排序方式</Text>
              <Select
                value={sortOrder}
                onChange={handleSortOrderChange}
                style={{ width: '100%' }}
              >
                <Option value="descend">↓ 降序</Option>
                <Option value="ascend">↑ 升序</Option>
              </Select>
            </Space>
          </Col>

          <Col xs={24} sm={12} md={7} lg={4}>
            <Space direction="vertical" size={4} style={{ width: '100%' }}>
              <Text strong style={{ fontSize: 12, opacity: 0 }}>操作</Text>
              <Space style={{ width: '100%' }}>
                <Button icon={<FilterOutlined />} onClick={handleResetFilters} style={{ flex: 1 }}>
                  重置
                </Button>
                <Button icon={<ReloadOutlined />} onClick={loadConfigs} type="primary" style={{ flex: 1 }}>
                  刷新
                </Button>
              </Space>
            </Space>
          </Col>
        </Row>
      </Card>

      {viewMode === 'list' ? (
        <Table
          columns={columns}
          dataSource={filteredConfigs}
          rowKey="id"
          loading={loading}
          pagination={{
            pageSize: 10,
            showSizeChanger: true,
            showTotal: (total) => `共 ${total} 条记录`,
          }}
          scroll={{ x: 2200 }}
          onRow={(record) => ({
            onClick: () => handleViewDetail(record.id),
            style: { cursor: 'pointer' },
          })}
        />
      ) : (
        <Row gutter={[16, 16]}>
          {filteredConfigs.map((config) => (
            <Col xs={24} sm={12} md={8} lg={6} key={config.id}>
              <Card
                hoverable
                onClick={() => handleViewDetail(config.id)}
                style={{ height: '100%' }}
                extra={
                  <Switch
                    checked={config.enabled}
                    onChange={(checked, e) => handleToggleEnabled(config.id, checked, e as any)}
                    size="small"
                    onClick={(checked, e) => e.stopPropagation()}
                  />
                }
                actions={[
                  <Tooltip title="查看详情" key="view">
                    <EyeOutlined onClick={(e) => { e.stopPropagation(); handleViewDetail(config.id); }} />
                  </Tooltip>,
                  <Tooltip title="编辑" key="edit">
                    <EditOutlined onClick={(e) => handleEdit(config.id, e)} />
                  </Tooltip>,
                  <Popconfirm
                    title="确定要删除这个配置吗？"
                    onConfirm={(e) => handleDelete(config.id, e)}
                    onCancel={(e) => e?.stopPropagation()}
                    okText="确定"
                    cancelText="取消"
                    key="delete"
                  >
                    <DeleteOutlined onClick={(e) => e.stopPropagation()} />
                  </Popconfirm>,
                ]}
              >
                <Card.Meta
                  title={
                    <Space>
                      <span>{config.name}</span>
                      {!config.enabled && <Tag color="default">已禁用</Tag>}
                    </Space>
                  }
                  description={
                    <div style={{ minHeight: 120 }}>
                      <div style={{ 
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        display: '-webkit-box',
                        WebkitLineClamp: 2,
                        WebkitBoxOrient: 'vertical',
                        marginBottom: 8,
                        color: '#666',
                        fontSize: 12
                      }}>
                        {config.description || '暂无描述'}
                      </div>
                      <div style={{ fontSize: 11, color: '#999', marginBottom: 3, lineHeight: '1.5' }}>
                        <strong>URL:</strong> {config.openai_base_url.length > 30 ? config.openai_base_url.substring(0, 30) + '...' : config.openai_base_url}
                      </div>
                      <div style={{ fontSize: 11, color: '#999', marginBottom: 3 }}>
                        <strong>模型:</strong> <Tag color="purple" style={{ fontSize: 10, padding: '0 4px', margin: 0 }}>{config.big_model}</Tag>
                        {config.supported_models && config.supported_models.length > 0 && (
                          <Tag color="green" style={{ fontSize: 10, padding: '0 4px', marginLeft: 4 }}>
                            +{config.supported_models.length}个
                          </Tag>
                        )}
                      </div>
                      <div style={{ fontSize: 11, color: '#999', marginBottom: 3 }}>
                        <strong>限制:</strong> {config.max_tokens_limit?.toLocaleString()} tokens / {config.request_timeout}s
                      </div>
                      <div style={{ fontSize: 11, color: '#999' }}>
                        <strong>Token:</strong> <code style={{ fontSize: 10, background: '#f0f0f0', padding: '1px 4px' }}>{config.anthropic_api_key || '-'}</code>
                      </div>
                    </div>
                  }
                />
                <div style={{ marginTop: 12, paddingTop: 12, borderTop: '1px solid #f0f0f0' }}>
                  <Space size={4} wrap>
                    <Tag color={config.enabled ? 'success' : 'default'}>
                      {config.enabled ? '启用' : '禁用'}
                    </Tag>
                    <Tooltip title={`创建: ${new Date(config.created_at).toLocaleString('zh-CN')}`}>
                      <Tag color="blue" style={{ fontSize: 10 }}>
                        创建 {new Date(config.created_at).toLocaleDateString('zh-CN')}
                      </Tag>
                    </Tooltip>
                    <Tooltip title={`更新: ${new Date(config.updated_at).toLocaleString('zh-CN')}`}>
                      <Tag color="orange" style={{ fontSize: 10 }}>
                        更新 {new Date(config.updated_at).toLocaleDateString('zh-CN')}
                      </Tag>
                    </Tooltip>
                  </Space>
                </div>
              </Card>
            </Col>
          ))}
          {filteredConfigs.length === 0 && !loading && (
            <Col span={24}>
              <Card style={{ textAlign: 'center', padding: '40px 0' }}>
                <Typography.Text type="secondary">暂无配置数据</Typography.Text>
              </Card>
            </Col>
          )}
        </Row>
      )}
    </Card>
  );
};

export default ConfigListV2;
