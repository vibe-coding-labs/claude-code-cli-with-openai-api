import axios from 'axios';

export interface ConfigNode {
  config_id: string;
  weight: number;
  enabled: boolean;
}

export interface LoadBalancer {
  id: string;
  name: string;
  description: string;
  strategy: string;
  config_nodes: ConfigNode[];
  enabled: boolean;
  anthropic_api_key: string;
  created_at: string;
  updated_at: string;
  // 健康检查配置
  health_check_enabled?: boolean;
  health_check_interval?: number;
  failure_threshold?: number;
  recovery_threshold?: number;
  health_check_timeout?: number;
  // 重试配置
  max_retries?: number;
  initial_retry_delay?: number;
  max_retry_delay?: number;
  // 熔断器配置
  circuit_breaker_enabled?: boolean;
  error_rate_threshold?: number;
  circuit_breaker_window?: number;
  circuit_breaker_timeout?: number;
  half_open_requests?: number;
  // 动态权重配置
  dynamic_weight_enabled?: boolean;
  weight_update_interval?: number;
  // 日志配置
  log_level?: string;
}

export interface LoadBalancerRequest {
  name: string;
  description: string;
  strategy: string;
  config_nodes: ConfigNode[];
  enabled: boolean;
  anthropic_api_key?: string;
  // 健康检查配置
  health_check_enabled?: boolean;
  health_check_interval?: number;
  failure_threshold?: number;
  recovery_threshold?: number;
  health_check_timeout?: number;
  // 重试配置
  max_retries?: number;
  initial_retry_delay?: number;
  max_retry_delay?: number;
  // 熔断器配置
  circuit_breaker_enabled?: boolean;
  error_rate_threshold?: number;
  circuit_breaker_window?: number;
  circuit_breaker_timeout?: number;
  half_open_requests?: number;
  // 动态权重配置
  dynamic_weight_enabled?: boolean;
  weight_update_interval?: number;
  // 日志配置
  log_level?: string;
}

export interface LoadBalancerStats {
  load_balancer_id: string;
  total_requests: number;
  success_requests: number;
  error_requests: number;
  total_input_tokens: number;
  total_output_tokens: number;
  total_tokens: number;
  avg_duration_ms: number;
  config_count: number;
}

export interface HealthStatus {
  config_id: string;
  status: string;
  last_check_time: string;
  consecutive_successes: number;
  consecutive_failures: number;
  last_error?: string;
  response_time_ms: number;
  created_at: string;
  updated_at: string;
}

export interface CircuitBreakerState {
  config_id: string;
  state: string; // closed, open, half_open
  failure_count: number;
  success_count: number;
  last_state_change: string;
  next_retry_time?: string;
  created_at: string;
  updated_at: string;
}

export interface NodeStats {
  config_id: string;
  config_name: string;
  health_status: string;
  circuit_breaker_state: string;
  request_count: number;
  success_rate: number;
  avg_response_time_ms: number;
  current_weight: number;
  base_weight: number;
}

export interface EnhancedStats {
  load_balancer_id: string;
  time_window: string;
  total_requests: number;
  success_requests: number;
  failed_requests: number;
  avg_response_time_ms: number;
  p50_response_time_ms: number;
  p95_response_time_ms: number;
  p99_response_time_ms: number;
  error_rate: number;
  active_connections: number;
  node_stats: NodeStats[];
}

export interface RequestLog {
  id: string;
  load_balancer_id: string;
  selected_config_id: string;
  request_time: string;
  response_time: string;
  duration_ms: number;
  status_code: number;
  success: boolean;
  retry_count: number;
  error_message?: string;
  request_summary?: string;
  response_preview?: string;
  created_at: string;
}

export interface Alert {
  id: string;
  load_balancer_id: string;
  level: string; // critical, warning, info
  type: string;  // all_nodes_down, low_healthy_nodes, high_error_rate, circuit_breaker_open
  message: string;
  details?: string;
  acknowledged: boolean;
  acknowledged_at?: string;
  created_at: string;
}

const API_BASE = '/api';

export const loadBalancerApi = {
  // Get all load balancers
  getAll: async (): Promise<LoadBalancer[]> => {
    const response = await axios.get(`${API_BASE}/load-balancers`);
    return response.data.load_balancers || [];
  },

  // Get a single load balancer
  get: async (id: string): Promise<LoadBalancer> => {
    const response = await axios.get(`${API_BASE}/load-balancers/${id}`);
    return response.data;
  },

  // Create a new load balancer
  create: async (data: LoadBalancerRequest): Promise<LoadBalancer> => {
    const response = await axios.post(`${API_BASE}/load-balancers`, data);
    return response.data;
  },

  // Update a load balancer
  update: async (id: string, data: LoadBalancerRequest): Promise<LoadBalancer> => {
    const response = await axios.put(`${API_BASE}/load-balancers/${id}`, data);
    return response.data;
  },

  // Delete a load balancer
  delete: async (id: string): Promise<void> => {
    await axios.delete(`${API_BASE}/load-balancers/${id}`);
  },

  // Renew API key
  renewKey: async (id: string, customToken?: string): Promise<{ anthropic_api_key: string; message: string }> => {
    const response = await axios.post(`${API_BASE}/load-balancers/${id}/renew-key`, {
      custom_token: customToken || '',
    });
    return response.data;
  },

  // Test load balancer
  test: async (id: string): Promise<any> => {
    const response = await axios.post(`${API_BASE}/load-balancers/${id}/test`);
    return response.data;
  },

  // Get statistics
  getStats: async (id: string): Promise<LoadBalancerStats> => {
    const response = await axios.get(`${API_BASE}/load-balancers/${id}/stats`);
    return response.data;
  },

  // Get enhanced statistics
  getEnhancedStats: async (id: string, window: string = '24h'): Promise<EnhancedStats> => {
    const response = await axios.get(`${API_BASE}/load-balancers/${id}/stats/enhanced?window=${window}`);
    return response.data;
  },

  // Get health status
  getHealthStatus: async (id: string): Promise<{ 
    load_balancer_id: string;
    total_nodes: number;
    healthy_nodes: number;
    unhealthy_nodes: number;
    statuses: HealthStatus[];
  }> => {
    const response = await axios.get(`${API_BASE}/load-balancers/${id}/health`);
    return response.data;
  },

  // Get circuit breaker states
  getCircuitBreakers: async (id: string): Promise<{
    load_balancer_id: string;
    total_nodes: number;
    closed: number;
    open: number;
    half_open: number;
    states: CircuitBreakerState[];
  }> => {
    const response = await axios.get(`${API_BASE}/load-balancers/${id}/circuit-breakers`);
    return response.data;
  },

  // Get request logs
  getRequestLogs: async (id: string, limit: number = 100, offset: number = 0): Promise<{
    load_balancer_id: string;
    total_count: number;
    limit: number;
    offset: number;
    logs: RequestLog[];
  }> => {
    const response = await axios.get(`${API_BASE}/load-balancers/${id}/logs?limit=${limit}&offset=${offset}`);
    return response.data;
  },

  // Get alerts
  getAlerts: async (id: string, acknowledged: boolean = false): Promise<{
    load_balancer_id: string;
    unacknowledged_count: number;
    alerts: Alert[];
  }> => {
    const response = await axios.get(`${API_BASE}/load-balancers/${id}/alerts?acknowledged=${acknowledged}`);
    return response.data;
  },

  // Acknowledge alert
  acknowledgeAlert: async (loadBalancerId: string, alertId: string): Promise<void> => {
    await axios.post(`${API_BASE}/load-balancers/${loadBalancerId}/alerts/${alertId}/acknowledge`);
  },

  // Trigger health check
  triggerHealthCheck: async (id: string): Promise<void> => {
    await axios.post(`${API_BASE}/load-balancers/${id}/health/check`);
  },

  // Reset circuit breaker
  resetCircuitBreaker: async (id: string, configId: string): Promise<void> => {
    await axios.post(`${API_BASE}/load-balancers/${id}/circuit-breakers/${configId}/reset`);
  },
};
