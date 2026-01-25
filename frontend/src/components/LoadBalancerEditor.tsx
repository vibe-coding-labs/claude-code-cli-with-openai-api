import React, { useState, useCallback, useEffect, useRef } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import ReactFlow, {
  Node,
  Edge,
  Controls,
  Background,
  useNodesState,
  useEdgesState,
  addEdge,
  Connection,
  MarkerType,
  Panel,
  SelectionMode,
} from 'reactflow';
import 'reactflow/dist/style.css';
import {
  Button,
  Select,
  InputNumber,
  message,
  Card,
  Space,
  Typography,
  Input,
  Drawer,
  Form,
  Switch,
  Spin,
  Dropdown,
  Menu,
} from 'antd';
import {
  SaveOutlined,
  PlusOutlined,
  DeleteOutlined,
  SettingOutlined,
  CopyOutlined,
  ScissorOutlined,
  SnippetsOutlined,
  LockOutlined,
  UnlockOutlined,
  EyeInvisibleOutlined,
  EyeOutlined,
  FolderOutlined,
  FolderOpenOutlined,
  EditOutlined,
} from '@ant-design/icons';
import { loadBalancerApi, LoadBalancer } from '../services/loadBalancerApi';
import api from '../services/api';
import './LoadBalancerEditor.css';

const { Title, Text } = Typography;
const { Option } = Select;

interface APIConfig {
  id: string;
  name: string;
  description: string;
  enabled: boolean;
}

interface NodeData {
  label: string;
  configId?: string;
  weight?: number;
  enabled?: boolean;
  type?: 'start' | 'config';
}

const LoadBalancerEditor: React.FC = () => {
  const navigate = useNavigate();
  const { id } = useParams<{ id: string }>();
  const isEditMode = !!id;

  const [nodes, setNodes, onNodesChange] = useNodesState([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState([]);
  const [configs, setConfigs] = useState<APIConfig[]>([]);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  
  // 表单数据
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [strategy, setStrategy] = useState<string>('round_robin');
  const [enabled, setEnabled] = useState(true);
  
  // 健康检查配置
  const [healthCheckEnabled, setHealthCheckEnabled] = useState(true);
  const [healthCheckInterval, setHealthCheckInterval] = useState(30);
  const [failureThreshold, setFailureThreshold] = useState(3);
  const [recoveryThreshold, setRecoveryThreshold] = useState(2);
  const [healthCheckTimeout, setHealthCheckTimeout] = useState(5);
  
  // 重试配置
  const [maxRetries, setMaxRetries] = useState(3);
  const [initialRetryDelay, setInitialRetryDelay] = useState(100);
  const [maxRetryDelay, setMaxRetryDelay] = useState(5000);
  
  // 熔断器配置
  const [circuitBreakerEnabled, setCircuitBreakerEnabled] = useState(true);
  const [errorRateThreshold, setErrorRateThreshold] = useState(0.5);
  const [circuitBreakerWindow, setCircuitBreakerWindow] = useState(60);
  const [circuitBreakerTimeout, setCircuitBreakerTimeout] = useState(30);
  const [halfOpenRequests, setHalfOpenRequests] = useState(3);
  
  // 动态权重配置
  const [dynamicWeightEnabled, setDynamicWeightEnabled] = useState(false);
  const [weightUpdateInterval, setWeightUpdateInterval] = useState(300);
  
  // 日志配置
  const [logLevel, setLogLevel] = useState('standard');
  
  // 节点编辑抽屉
  const [selectedNode, setSelectedNode] = useState<Node<NodeData> | null>(null);
  const [drawerVisible, setDrawerVisible] = useState(false);
  const [nodeForm] = Form.useForm();

  // 剪贴板状态
  const [clipboardNode, setClipboardNode] = useState<Node<NodeData> | null>(null);
  const [clipboardAction, setClipboardAction] = useState<'copy' | 'cut'>('copy');

  // 右键菜单状态
  const [contextMenuVisible, setContextMenuVisible] = useState(false);
  const [contextMenuPosition, setContextMenuPosition] = useState({ x: 0, y: 0 });
  const [contextMenuNode, setContextMenuNode] = useState<Node<NodeData> | null>(null);

  // 节点分组状态
  const [nodeGroups, setNodeGroups] = useState<Record<string, string>>({});
  const [selectedGroupId, setSelectedGroupId] = useState<string | null>(null);

  // 键盘快捷键处理
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      // Delete键：删除选中的节点
      if (event.key === 'Delete' || event.key === 'Backspace') {
        const selectedNodes = nodes.filter(n => n.selected);
        if (selectedNodes.length > 0) {
          selectedNodes.forEach(node => {
            if (node.data.type === 'config') {
              handleDeleteNode(node.id);
            }
          });
          event.preventDefault();
        }
      }

      // Ctrl+C：复制节点
      if (event.ctrlKey && event.key === 'c') {
        const selectedNodes = nodes.filter(n => n.selected);
        if (selectedNodes.length === 1 && selectedNodes[0].data.type === 'config') {
          setClipboardNode(selectedNodes[0]);
          setClipboardAction('copy');
          message.success('节点已复制');
          event.preventDefault();
        }
      }

      // Ctrl+V：粘贴节点
      if (event.ctrlKey && event.key === 'v') {
        if (clipboardNode) {
          handlePasteNode();
          event.preventDefault();
        }
      }

      // Ctrl+X：剪切节点
      if (event.ctrlKey && event.key === 'x') {
        const selectedNodes = nodes.filter(n => n.selected);
        if (selectedNodes.length === 1 && selectedNodes[0].data.type === 'config') {
          setClipboardNode(selectedNodes[0]);
          setClipboardAction('cut');
          message.success('节点已剪切');
          event.preventDefault();
        }
      }

      // Ctrl+A：全选节点
      if (event.ctrlKey && event.key === 'a') {
        const configNodes = nodes.filter(n => n.data.type === 'config');
        setNodes((nds) => nds.map(n => {
          if (n.data.type === 'config') {
            return { ...n, selected: true };
          }
          return n;
        }));
        event.preventDefault();
      }

      // Ctrl+D：取消选择
      if (event.ctrlKey && event.key === 'd') {
        setNodes((nds) => nds.map(n => ({ ...n, selected: false })));
        event.preventDefault();
      }

      // Ctrl+I：反选节点
      if (event.ctrlKey && event.key === 'i') {
        setNodes((nds) => nds.map(n => {
          if (n.data.type === 'config') {
            return { ...n, selected: !n.selected };
          }
          return n;
        }));
        message.success('已反选节点');
        event.preventDefault();
      }

      // Escape键：关闭右键菜单
      if (event.key === 'Escape') {
        handleContextMenuClose();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => {
      window.removeEventListener('keydown', handleKeyDown);
    };
  }, [nodes, clipboardNode, clipboardAction, setNodes]);

  // 加载API配置列表
  useEffect(() => {
    loadConfigs();
    if (isEditMode) {
      loadLoadBalancer();
    } else {
      // 新建模式：初始化起始节点
      initializeStartNode();
    }
  }, [id]);

  const loadConfigs = async () => {
    try {
      const response = await api.get('/api/configs');
      setConfigs(response.data.configs || []);
    } catch (error) {
      message.error('加载配置列表失败');
      console.error('Failed to load configs:', error);
    }
  };

  const loadLoadBalancer = async () => {
    if (!id) return;
    
    setLoading(true);
    try {
      const lb = await loadBalancerApi.get(id);
      setName(lb.name);
      setDescription(lb.description);
      setStrategy(lb.strategy);
      setEnabled(lb.enabled);
      
      // 健康检查配置
      setHealthCheckEnabled(lb.health_check_enabled ?? true);
      setHealthCheckInterval(lb.health_check_interval ?? 30);
      setFailureThreshold(lb.failure_threshold ?? 3);
      setRecoveryThreshold(lb.recovery_threshold ?? 2);
      setHealthCheckTimeout(lb.health_check_timeout ?? 5);
      
      // 重试配置
      setMaxRetries(lb.max_retries ?? 3);
      setInitialRetryDelay(lb.initial_retry_delay ?? 100);
      setMaxRetryDelay(lb.max_retry_delay ?? 5000);
      
      // 熔断器配置
      setCircuitBreakerEnabled(lb.circuit_breaker_enabled ?? true);
      setErrorRateThreshold(lb.error_rate_threshold ?? 0.5);
      setCircuitBreakerWindow(lb.circuit_breaker_window ?? 60);
      setCircuitBreakerTimeout(lb.circuit_breaker_timeout ?? 30);
      setHalfOpenRequests(lb.half_open_requests ?? 3);
      
      // 动态权重配置
      setDynamicWeightEnabled(lb.dynamic_weight_enabled ?? false);
      setWeightUpdateInterval(lb.weight_update_interval ?? 300);
      
      // 日志配置
      setLogLevel(lb.log_level ?? 'standard');
      
      // 转换为节点和边
      convertToNodesAndEdges(lb);
    } catch (error) {
      message.error('加载负载均衡器失败');
      console.error('Failed to load load balancer:', error);
    } finally {
      setLoading(false);
    }
  };

  const initializeStartNode = () => {
    const startNode: Node<NodeData> = {
      id: 'start',
      type: 'input',
      data: { label: '负载均衡器入口', type: 'start' },
      position: { x: 250, y: 50 },
      draggable: false,
      style: {
        background: '#1890ff',
        color: 'white',
        border: '2px solid #096dd9',
        borderRadius: '8px',
        padding: '10px 20px',
        fontSize: '14px',
        fontWeight: 'bold',
      },
    };
    setNodes([startNode]);
  };

  const convertToNodesAndEdges = (lb: LoadBalancer) => {
    const newNodes: Node<NodeData>[] = [];
    const newEdges: Edge[] = [];

    // 起始节点
    const startNode: Node<NodeData> = {
      id: 'start',
      type: 'input',
      data: { label: '负载均衡器入口', type: 'start' },
      position: { x: 250, y: 50 },
      draggable: false,
      style: {
        background: '#1890ff',
        color: 'white',
        border: '2px solid #096dd9',
        borderRadius: '8px',
        padding: '10px 20px',
        fontSize: '14px',
        fontWeight: 'bold',
      },
    };
    newNodes.push(startNode);

    // 配置节点
    lb.config_nodes.forEach((node, index) => {
      const config = configs.find(c => c.id === node.config_id);
      const configName = config?.name || node.config_id;
      
      const configNode: Node<NodeData> = {
        id: node.config_id,
        type: 'default',
        data: {
          label: `${configName}\n权重: ${node.weight}`,
          configId: node.config_id,
          weight: node.weight,
          enabled: node.enabled,
          type: 'config',
        },
        position: { x: 100 + index * 200, y: 200 },
        style: {
          background: node.enabled ? '#52c41a' : '#d9d9d9',
          color: 'white',
          border: node.enabled ? '2px solid #389e0d' : '2px solid #8c8c8c',
          borderRadius: '8px',
          padding: '10px',
          fontSize: '12px',
        },
      };
      newNodes.push(configNode);

      // 连接线
      const edge: Edge = {
        id: `start-${node.config_id}`,
        source: 'start',
        target: node.config_id,
        type: 'smoothstep',
        animated: true,
        markerEnd: {
          type: MarkerType.ArrowClosed,
        },
      };
      newEdges.push(edge);
    });

    setNodes(newNodes);
    setEdges(newEdges);
  };

  const onConnect = useCallback(
    (params: Connection) => setEdges((eds) => addEdge(params, eds)),
    [setEdges]
  );

  const onNodeClick = useCallback((event: React.MouseEvent, node: Node<NodeData>) => {
    // Shift+点击：多选节点
    if (event.shiftKey && node.data.type === 'config') {
      setNodes((nds) => nds.map(n => {
        if (n.id === node.id) {
          return { ...n, selected: !n.selected };
        }
        return n;
      }));
      return;
    }

    // 普通点击：单选节点
    if (node.data.type === 'config') {
      setSelectedNode(node);
      nodeForm.setFieldsValue({
        weight: node.data.weight || 1,
        enabled: node.data.enabled !== false,
      });
      setDrawerVisible(true);
    }
  }, [nodeForm, setNodes]);

  const onNodeContextMenu = useCallback((event: React.MouseEvent, node: Node<NodeData>) => {
    event.preventDefault();
    event.stopPropagation();
    
    if (node.data.type === 'config') {
      setContextMenuNode(node);
      setContextMenuPosition({ x: event.clientX, y: event.clientY });
      setContextMenuVisible(true);
    }
  }, []);

  const handleContextMenuClose = useCallback(() => {
    setContextMenuVisible(false);
    setContextMenuNode(null);
  }, []);

  const handleCopyNode = useCallback(() => {
    if (!contextMenuNode) return;
    setClipboardNode(contextMenuNode);
    setClipboardAction('copy');
    message.success('节点已复制');
    handleContextMenuClose();
  }, [contextMenuNode]);

  const handleCutNode = useCallback(() => {
    if (!contextMenuNode) return;
    setClipboardNode(contextMenuNode);
    setClipboardAction('cut');
    message.success('节点已剪切');
    handleContextMenuClose();
  }, [contextMenuNode]);

  const handlePasteNode = useCallback(() => {
    if (!clipboardNode) {
      message.warning('剪贴板为空');
      return;
    }
    
    const config = configs.find(c => c.id === clipboardNode.id);
    if (!config) {
      message.error('配置不存在');
      return;
    }

    const newNodeId = `copy-${clipboardNode.id}-${Date.now()}`;
    const newNode: Node<NodeData> = {
      ...clipboardNode,
      id: newNodeId,
      data: {
        ...clipboardNode.data,
        configId: config.id,
        label: `${config.name}\n权重: ${clipboardNode.data.weight || 1}`,
      },
      position: {
        x: clipboardNode.position.x + 50,
        y: clipboardNode.position.y + 50,
      },
    };

    setNodes((nds) => [...nds, newNode]);
    
    if (clipboardAction === 'cut') {
      setNodes((nds) => nds.filter(n => n.id !== clipboardNode.id));
      setEdges((eds) => eds.filter(e => e.source !== clipboardNode.id && e.target !== clipboardNode.id));
      setClipboardNode(null);
      message.success('节点已粘贴');
    } else {
      message.success('节点已复制');
    }
    
    handleContextMenuClose();
  }, [clipboardNode, clipboardAction, configs, setNodes, setEdges]);

  const handleLockNode = useCallback(() => {
    if (!contextMenuNode) return;
    const updatedNode = {
      ...contextMenuNode,
      draggable: false,
    };
    setNodes((nds) => nds.map(n => n.id === contextMenuNode.id ? updatedNode : n));
    message.success('节点已锁定');
    handleContextMenuClose();
  }, [contextMenuNode, setNodes]);

  const handleUnlockNode = useCallback(() => {
    if (!contextMenuNode) return;
    const updatedNode = {
      ...contextMenuNode,
      draggable: true,
    };
    setNodes((nds) => nds.map(n => n.id === contextMenuNode.id ? updatedNode : n));
    message.success('节点已解锁');
    handleContextMenuClose();
  }, [contextMenuNode, setNodes]);

  const handleHideNode = useCallback(() => {
    if (!contextMenuNode) return;
    const updatedNode = {
      ...contextMenuNode,
      hidden: true,
    };
    setNodes((nds) => nds.map(n => n.id === contextMenuNode.id ? updatedNode : n));
    message.success('节点已隐藏');
    handleContextMenuClose();
  }, [contextMenuNode, setNodes]);

  const handleShowNode = useCallback(() => {
    if (!contextMenuNode) return;
    const updatedNode = {
      ...contextMenuNode,
      hidden: false,
    };
    setNodes((nds) => nds.map(n => n.id === contextMenuNode.id ? updatedNode : n));
    message.success('节点已显示');
    handleContextMenuClose();
  }, [contextMenuNode, setNodes]);

  const handleDeleteNodeFromMenu = useCallback(() => {
    if (!contextMenuNode) return;
    handleDeleteNode(contextMenuNode.id);
    handleContextMenuClose();
  }, [contextMenuNode]);

  const handleAlignNodes = useCallback((alignment: 'left' | 'center' | 'right' | 'top' | 'middle' | 'bottom') => {
    const selectedNodes = nodes.filter(n => n.selected && n.data.type === 'config');
    if (selectedNodes.length < 2) {
      message.warning('请至少选择2个节点');
      return;
    }

    let targetX: number | null = null;
    let targetY: number | null = null;

    if (alignment === 'left') {
      const minX = Math.min(...selectedNodes.map(n => n.position.x));
      setNodes((nds) => nds.map(n => {
        if (n.selected && n.data.type === 'config') {
          return { ...n, position: { ...n.position, x: minX } };
        }
        return n;
      }));
      message.success('已左对齐');
    } else if (alignment === 'center') {
      const centerX = selectedNodes.reduce((sum, n) => sum + n.position.x, 0) / selectedNodes.length;
      setNodes((nds) => nds.map(n => {
        if (n.selected && n.data.type === 'config') {
          return { ...n, position: { ...n.position, x: centerX } };
        }
        return n;
      }));
      message.success('已水平居中');
    } else if (alignment === 'right') {
      const maxX = Math.max(...selectedNodes.map(n => n.position.x));
      setNodes((nds) => nds.map(n => {
        if (n.selected && n.data.type === 'config') {
          return { ...n, position: { ...n.position, x: maxX } };
        }
        return n;
      }));
      message.success('已右对齐');
    } else if (alignment === 'top') {
      const minY = Math.min(...selectedNodes.map(n => n.position.y));
      setNodes((nds) => nds.map(n => {
        if (n.selected && n.data.type === 'config') {
          return { ...n, position: { ...n.position, y: minY } };
        }
        return n;
      }));
      message.success('已顶对齐');
    } else if (alignment === 'middle') {
      const centerY = selectedNodes.reduce((sum, n) => sum + n.position.y, 0) / selectedNodes.length;
      setNodes((nds) => nds.map(n => {
        if (n.selected && n.data.type === 'config') {
          return { ...n, position: { ...n.position, y: centerY } };
        }
        return n;
      }));
      message.success('已垂直居中');
    } else if (alignment === 'bottom') {
      const maxY = Math.max(...selectedNodes.map(n => n.position.y));
      setNodes((nds) => nds.map(n => {
        if (n.selected && n.data.type === 'config') {
          return { ...n, position: { ...n.position, y: maxY } };
        }
        return n;
      }));
      message.success('已底对齐');
    }
  }, [nodes, setNodes]);

  const handleDistributeNodes = useCallback((direction: 'horizontal' | 'vertical') => {
    const selectedNodes = nodes.filter(n => n.selected && n.data.type === 'config');
    if (selectedNodes.length < 2) {
      message.warning('请至少选择2个节点');
      return;
    }

    const sortedNodes = [...selectedNodes].sort((a, b) => {
      if (direction === 'horizontal') {
        return a.position.x - b.position.x;
      } else {
        return a.position.y - b.position.y;
      }
    });

    if (direction === 'horizontal') {
      const firstX = sortedNodes[0].position.x;
      const lastX = sortedNodes[sortedNodes.length - 1].position.x;
      const spacing = (lastX - firstX) / (sortedNodes.length - 1);
      
      setNodes((nds) => nds.map(n => {
        const index = sortedNodes.findIndex(sn => sn.id === n.id);
        if (index !== -1) {
          return { ...n, position: { ...n.position, x: firstX + index * spacing } };
        }
        return n;
      }));
      message.success('已水平分布');
    } else {
      const firstY = sortedNodes[0].position.y;
      const lastY = sortedNodes[sortedNodes.length - 1].position.y;
      const spacing = (lastY - firstY) / (sortedNodes.length - 1);
      
      setNodes((nds) => nds.map(n => {
        const index = sortedNodes.findIndex(sn => sn.id === n.id);
        if (index !== -1) {
          return { ...n, position: { ...n.position, y: firstY + index * spacing } };
        }
        return n;
      }));
      message.success('已垂直分布');
    }
  }, [nodes, setNodes]);

  const handleGroupNodes = useCallback(() => {
    const selectedNodes = nodes.filter(n => n.selected && n.data.type === 'config');
    if (selectedNodes.length < 2) {
      message.warning('请至少选择2个节点进行分组');
      return;
    }

    const groupId = `group-${Date.now()}`;
    const newGroups = { ...nodeGroups };
    selectedNodes.forEach(node => {
      newGroups[node.id] = groupId;
    });
    setNodeGroups(newGroups);
    message.success(`已创建分组，包含${selectedNodes.length}个节点`);
  }, [nodes, nodeGroups]);

  const handleUngroupNodes = useCallback(() => {
    const selectedNodes = nodes.filter(n => n.selected && n.data.type === 'config');
    if (selectedNodes.length === 0) {
      message.warning('请选择要取消分组的节点');
      return;
    }

    const newGroups = { ...nodeGroups };
    let ungroupedCount = 0;
    selectedNodes.forEach(node => {
      if (newGroups[node.id]) {
        delete newGroups[node.id];
        ungroupedCount++;
      }
    });
    setNodeGroups(newGroups);
    message.success(`已取消${ungroupedCount}个节点的分组`);
  }, [nodes, nodeGroups]);

  const handleAddConfig = (configId: string) => {
    const config = configs.find(c => c.id === configId);
    if (!config) return;

    // 检查是否已添加
    if (nodes.some(n => n.id === configId)) {
      message.warning('该配置已添加');
      return;
    }

    const newNode: Node<NodeData> = {
      id: configId,
      type: 'default',
      data: {
        label: `${config.name}\n权重: 1`,
        configId: configId,
        weight: 1,
        enabled: true,
        type: 'config',
      },
      position: { x: 100 + (nodes.length - 1) * 200, y: 200 },
      style: {
        background: '#52c41a',
        color: 'white',
        border: '2px solid #389e0d',
        borderRadius: '8px',
        padding: '10px',
        fontSize: '12px',
      },
    };

    const newEdge: Edge = {
      id: `start-${configId}`,
      source: 'start',
      target: configId,
      type: 'smoothstep',
      animated: true,
      markerEnd: {
        type: MarkerType.ArrowClosed,
      },
    };

    setNodes((nds) => [...nds, newNode]);
    setEdges((eds) => [...eds, newEdge]);
    message.success('配置已添加');
  };

  const handleDeleteNode = (nodeId: string) => {
    setNodes((nds) => nds.filter(n => n.id !== nodeId));
    setEdges((eds) => eds.filter(e => e.source !== nodeId && e.target !== nodeId));
    message.success('配置已删除');
  };

  const handleUpdateNode = () => {
    if (!selectedNode) return;

    nodeForm.validateFields().then((values) => {
      const config = configs.find(c => c.id === selectedNode.id);
      const updatedNode: Node<NodeData> = {
        ...selectedNode,
        data: {
          ...selectedNode.data,
          label: `${config?.name || selectedNode.id}\n权重: ${values.weight}`,
          weight: values.weight,
          enabled: values.enabled,
        },
        style: {
          ...selectedNode.style,
          background: values.enabled ? '#52c41a' : '#d9d9d9',
          border: values.enabled ? '2px solid #389e0d' : '2px solid #8c8c8c',
        },
      };

      setNodes((nds) => nds.map(n => n.id === selectedNode.id ? updatedNode : n));
      setDrawerVisible(false);
      message.success('节点已更新');
    });
  };

  const handleSave = async () => {
    if (!name.trim()) {
      message.error('请输入负载均衡器名称');
      return;
    }

    const configNodes = nodes
      .filter(n => n.data.type === 'config')
      .map(n => ({
        config_id: n.id,
        weight: n.data.weight || 1,
        enabled: n.data.enabled !== false,
      }));

    if (configNodes.length === 0) {
      message.error('请至少添加一个配置节点');
      return;
    }

    setSaving(true);
    try {
      const data = {
        name,
        description,
        strategy,
        config_nodes: configNodes,
        enabled,
        // 健康检查配置
        health_check_enabled: healthCheckEnabled,
        health_check_interval: healthCheckInterval,
        failure_threshold: failureThreshold,
        recovery_threshold: recoveryThreshold,
        health_check_timeout: healthCheckTimeout,
        // 重试配置
        max_retries: maxRetries,
        initial_retry_delay: initialRetryDelay,
        max_retry_delay: maxRetryDelay,
        // 熔断器配置
        circuit_breaker_enabled: circuitBreakerEnabled,
        error_rate_threshold: errorRateThreshold,
        circuit_breaker_window: circuitBreakerWindow,
        circuit_breaker_timeout: circuitBreakerTimeout,
        half_open_requests: halfOpenRequests,
        // 动态权重配置
        dynamic_weight_enabled: dynamicWeightEnabled,
        weight_update_interval: weightUpdateInterval,
        // 日志配置
        log_level: logLevel,
      };

      if (isEditMode && id) {
        await loadBalancerApi.update(id, data);
        message.success('负载均衡器已更新');
      } else {
        await loadBalancerApi.create(data);
        message.success('负载均衡器已创建');
      }

      navigate('/ui/load-balancers');
    } catch (error: any) {
      message.error(error.response?.data?.error || '保存失败');
      console.error('Failed to save load balancer:', error);
    } finally {
      setSaving(false);
    }
  };

  const handleAutoLayout = () => {
    const configNodesCount = nodes.filter(n => n.data.type === 'config').length;
    const spacing = 200;
    const startX = 250 - (configNodesCount * spacing) / 2;

    setNodes((nds) => nds.map((node, index) => {
      if (node.data.type === 'start') {
        return { ...node, position: { x: 250, y: 50 } };
      }
      const configIndex = nds.filter(n => n.data.type === 'config').indexOf(node);
      return {
        ...node,
        position: { x: startX + configIndex * spacing, y: 200 },
      };
    }));
    message.success('已自动布局');
  };

  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: '100px 0' }}>
        <Spin size="large" />
      </div>
    );
  }

  return (
    <div className="load-balancer-editor">
      <Card>
        <Space direction="vertical" style={{ width: '100%' }} size="large">
          {/* 顶部表单 */}
          <div>
            <Title level={4}>{isEditMode ? '编辑负载均衡器' : '创建负载均衡器'}</Title>
            <Space direction="vertical" style={{ width: '100%' }}>
              <div>
                <Text strong>名称：</Text>
                <Input
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="请输入负载均衡器名称"
                  style={{ width: 300, marginLeft: 8 }}
                />
              </div>
              <div>
                <Text strong>描述：</Text>
                <Input
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder="请输入描述（可选）"
                  style={{ width: 300, marginLeft: 8 }}
                />
              </div>
              <div>
                <Text strong>策略：</Text>
                <Select
                  value={strategy}
                  onChange={setStrategy}
                  style={{ width: 200, marginLeft: 8 }}
                >
                  <Option value="round_robin">轮询</Option>
                  <Option value="random">随机</Option>
                  <Option value="weighted">权重</Option>
                  <Option value="least_connections">最少连接</Option>
                </Select>
              </div>
              <div>
                <Text strong>状态：</Text>
                <Switch
                  checked={enabled}
                  onChange={setEnabled}
                  checkedChildren="启用"
                  unCheckedChildren="禁用"
                  style={{ marginLeft: 8 }}
                />
              </div>
            </Space>
          </div>

          {/* 高级配置 */}
          <Card title="健康检查配置" size="small" style={{ marginTop: 16 }}>
            <Space direction="vertical" style={{ width: '100%' }}>
              <div>
                <Text strong>启用健康检查：</Text>
                <Switch
                  checked={healthCheckEnabled}
                  onChange={setHealthCheckEnabled}
                  checkedChildren="启用"
                  unCheckedChildren="禁用"
                  style={{ marginLeft: 8 }}
                />
              </div>
              {healthCheckEnabled && (
                <>
                  <div>
                    <Text strong>检查间隔（秒）：</Text>
                    <InputNumber
                      value={healthCheckInterval}
                      onChange={(val) => setHealthCheckInterval(val || 30)}
                      min={10}
                      max={300}
                      style={{ width: 150, marginLeft: 8 }}
                    />
                    <Text type="secondary" style={{ marginLeft: 8 }}>范围：10-300秒</Text>
                  </div>
                  <div>
                    <Text strong>失败阈值：</Text>
                    <InputNumber
                      value={failureThreshold}
                      onChange={(val) => setFailureThreshold(val || 3)}
                      min={1}
                      max={10}
                      style={{ width: 150, marginLeft: 8 }}
                    />
                    <Text type="secondary" style={{ marginLeft: 8 }}>连续失败次数</Text>
                  </div>
                  <div>
                    <Text strong>恢复阈值：</Text>
                    <InputNumber
                      value={recoveryThreshold}
                      onChange={(val) => setRecoveryThreshold(val || 2)}
                      min={1}
                      max={10}
                      style={{ width: 150, marginLeft: 8 }}
                    />
                    <Text type="secondary" style={{ marginLeft: 8 }}>连续成功次数</Text>
                  </div>
                  <div>
                    <Text strong>超时时间（秒）：</Text>
                    <InputNumber
                      value={healthCheckTimeout}
                      onChange={(val) => setHealthCheckTimeout(val || 5)}
                      min={1}
                      max={30}
                      style={{ width: 150, marginLeft: 8 }}
                    />
                  </div>
                </>
              )}
            </Space>
          </Card>

          <Card title="重试配置" size="small" style={{ marginTop: 16 }}>
            <Space direction="vertical" style={{ width: '100%' }}>
              <div>
                <Text strong>最大重试次数：</Text>
                <InputNumber
                  value={maxRetries}
                  onChange={(val) => setMaxRetries(val || 3)}
                  min={0}
                  max={10}
                  style={{ width: 150, marginLeft: 8 }}
                />
                <Text type="secondary" style={{ marginLeft: 8 }}>范围：0-10次</Text>
              </div>
              <div>
                <Text strong>初始延迟（毫秒）：</Text>
                <InputNumber
                  value={initialRetryDelay}
                  onChange={(val) => setInitialRetryDelay(val || 100)}
                  min={10}
                  max={5000}
                  style={{ width: 150, marginLeft: 8 }}
                />
              </div>
              <div>
                <Text strong>最大延迟（毫秒）：</Text>
                <InputNumber
                  value={maxRetryDelay}
                  onChange={(val) => setMaxRetryDelay(val || 5000)}
                  min={100}
                  max={30000}
                  style={{ width: 150, marginLeft: 8 }}
                />
              </div>
            </Space>
          </Card>

          <Card title="熔断器配置" size="small" style={{ marginTop: 16 }}>
            <Space direction="vertical" style={{ width: '100%' }}>
              <div>
                <Text strong>启用熔断器：</Text>
                <Switch
                  checked={circuitBreakerEnabled}
                  onChange={setCircuitBreakerEnabled}
                  checkedChildren="启用"
                  unCheckedChildren="禁用"
                  style={{ marginLeft: 8 }}
                />
              </div>
              {circuitBreakerEnabled && (
                <>
                  <div>
                    <Text strong>错误率阈值：</Text>
                    <InputNumber
                      value={errorRateThreshold}
                      onChange={(val) => setErrorRateThreshold(val || 0.5)}
                      min={0}
                      max={1}
                      step={0.1}
                      style={{ width: 150, marginLeft: 8 }}
                    />
                    <Text type="secondary" style={{ marginLeft: 8 }}>0.0-1.0</Text>
                  </div>
                  <div>
                    <Text strong>时间窗口（秒）：</Text>
                    <InputNumber
                      value={circuitBreakerWindow}
                      onChange={(val) => setCircuitBreakerWindow(val || 60)}
                      min={10}
                      max={300}
                      style={{ width: 150, marginLeft: 8 }}
                    />
                  </div>
                  <div>
                    <Text strong>超时时间（秒）：</Text>
                    <InputNumber
                      value={circuitBreakerTimeout}
                      onChange={(val) => setCircuitBreakerTimeout(val || 30)}
                      min={5}
                      max={300}
                      style={{ width: 150, marginLeft: 8 }}
                    />
                  </div>
                  <div>
                    <Text strong>半开测试请求数：</Text>
                    <InputNumber
                      value={halfOpenRequests}
                      onChange={(val) => setHalfOpenRequests(val || 3)}
                      min={1}
                      max={10}
                      style={{ width: 150, marginLeft: 8 }}
                    />
                  </div>
                </>
              )}
            </Space>
          </Card>

          <Card title="动态权重配置" size="small" style={{ marginTop: 16 }}>
            <Space direction="vertical" style={{ width: '100%' }}>
              <div>
                <Text strong>启用动态权重：</Text>
                <Switch
                  checked={dynamicWeightEnabled}
                  onChange={setDynamicWeightEnabled}
                  checkedChildren="启用"
                  unCheckedChildren="禁用"
                  style={{ marginLeft: 8 }}
                />
                <Text type="secondary" style={{ marginLeft: 8 }}>仅在权重策略下生效</Text>
              </div>
              {dynamicWeightEnabled && (
                <div>
                  <Text strong>更新间隔（秒）：</Text>
                  <InputNumber
                    value={weightUpdateInterval}
                    onChange={(val) => setWeightUpdateInterval(val || 300)}
                    min={60}
                    max={3600}
                    style={{ width: 150, marginLeft: 8 }}
                  />
                  <Text type="secondary" style={{ marginLeft: 8 }}>范围：60-3600秒</Text>
                </div>
              )}
            </Space>
          </Card>

          <Card title="日志配置" size="small" style={{ marginTop: 16 }}>
            <div>
              <Text strong>日志级别：</Text>
              <Select
                value={logLevel}
                onChange={setLogLevel}
                style={{ width: 200, marginLeft: 8 }}
              >
                <Option value="minimal">最小（Minimal）</Option>
                <Option value="standard">标准（Standard）</Option>
                <Option value="detailed">详细（Detailed）</Option>
              </Select>
            </div>
          </Card>

          {/* 工具栏 */}
          <Space>
            <Text strong>添加配置：</Text>
            <Select
              placeholder="选择要添加的配置"
              style={{ width: 300 }}
              onChange={handleAddConfig}
              value={undefined}
            >
              {configs.filter(c => c.enabled).map(config => (
                <Option key={config.id} value={config.id}>
                  {config.name}
                </Option>
              ))}
            </Select>
            <Button icon={<SettingOutlined />} onClick={handleAutoLayout}>
              自动布局
            </Button>
            <Button
              type="primary"
              icon={<SaveOutlined />}
              onClick={handleSave}
              loading={saving}
            >
              保存
            </Button>
            <Button onClick={() => navigate('/ui/load-balancers')}>
              取消
            </Button>
          </Space>

          {/* 流程图画布 */}
          <div style={{ height: '500px', border: '1px solid #d9d9d9', borderRadius: '4px' }}>
            <ReactFlow
              nodes={nodes}
              edges={edges}
              onNodesChange={onNodesChange}
              onEdgesChange={onEdgesChange}
              onConnect={onConnect}
              onNodeClick={onNodeClick}
              onNodeContextMenu={onNodeContextMenu}
              selectionMode={SelectionMode.Partial}
              fitView
            >
              <Background />
              <Controls />
              <Panel position="top-right">
                <Card size="small" style={{ width: 200 }}>
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    提示：点击配置节点可编辑权重
                  </Text>
                </Card>
              </Panel>
            </ReactFlow>
          </div>

          {/* 右键菜单 */}
          {contextMenuVisible && (
            <Dropdown
              open={contextMenuVisible}
              onOpenChange={handleContextMenuClose}
              dropdownRender={() => (
                <Menu
                  onClick={handleContextMenuClose}
                  items={[
                    {
                      key: 'copy',
                      icon: <CopyOutlined />,
                      label: '复制',
                      onClick: handleCopyNode,
                    },
                    {
                      key: 'cut',
                      icon: <ScissorOutlined />,
                      label: '剪切',
                      onClick: handleCutNode,
                    },
                    {
                      key: 'paste',
                      icon: <SnippetsOutlined />,
                      label: '粘贴',
                      onClick: handlePasteNode,
                      disabled: !clipboardNode,
                    },
                    { type: 'divider' },
                    {
                      key: 'lock',
                      icon: <LockOutlined />,
                      label: '锁定',
                      onClick: handleLockNode,
                    },
                    {
                      key: 'unlock',
                      icon: <UnlockOutlined />,
                      label: '解锁',
                      onClick: handleUnlockNode,
                    },
                    { type: 'divider' },
                    {
                      key: 'hide',
                      icon: <EyeInvisibleOutlined />,
                      label: '隐藏',
                      onClick: handleHideNode,
                    },
                    {
                      key: 'show',
                      icon: <EyeOutlined />,
                      label: '显示',
                      onClick: handleShowNode,
                    },
                    { type: 'divider' },
                    {
                      key: 'delete',
                      icon: <DeleteOutlined />,
                      label: '删除',
                      onClick: handleDeleteNodeFromMenu,
                      danger: true,
                    },
                  ]}
                  style={{
                    position: 'absolute',
                    left: contextMenuPosition.x,
                    top: contextMenuPosition.y,
                    zIndex: 1000,
                  }}
                />
              )}
            />
          )}
        </Space>
      </Card>

      {/* 节点编辑抽屉 */}
      <Drawer
        title="编辑配置节点"
        placement="right"
        onClose={() => setDrawerVisible(false)}
        open={drawerVisible}
        width={400}
      >
        {selectedNode && (
          <Form form={nodeForm} layout="vertical">
            <Form.Item label="配置名称">
              <Text>{configs.find(c => c.id === selectedNode.id)?.name || selectedNode.id}</Text>
            </Form.Item>
            <Form.Item
              label="权重"
              name="weight"
              rules={[
                { required: true, message: '请输入权重' },
                { type: 'number', min: 1, max: 100, message: '权重范围：1-100' },
              ]}
            >
              <InputNumber min={1} max={100} style={{ width: '100%' }} />
            </Form.Item>
            <Form.Item label="启用状态" name="enabled" valuePropName="checked">
              <Switch checkedChildren="启用" unCheckedChildren="禁用" />
            </Form.Item>
            <Space>
              <Button type="primary" onClick={handleUpdateNode}>
                更新
              </Button>
              <Button danger onClick={() => {
                handleDeleteNode(selectedNode.id);
                setDrawerVisible(false);
              }}>
                删除节点
              </Button>
            </Space>
          </Form>
        )}
      </Drawer>
    </div>
  );
};

export default LoadBalancerEditor;
