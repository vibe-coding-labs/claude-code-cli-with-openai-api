import axios from 'axios';
import {
  APIConfig,
  APIConfigRequest,
  ConfigListResponse,
  TestConfigResponse,
  ClaudeConfigFormat,
} from '../types/api';
import {
  User,
  UserCreateRequest,
  UserUpdateRequest,
  UserPasswordRequest,
  UserStatusRequest,
  UserTokenStats,
  LogsResult,
} from '../types/user';
import { getToken } from './auth';
import { getApiOrigin } from './apiBase';

// ⚠️ 严禁随意修改！后端固定端口54988，前端固定端口54989
// 开发模式下使用绝对路径，避免 homepage 影响
const API_BASE_URL = process.env.REACT_APP_API_URL || getApiOrigin();

const api = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Add auth token to requests
api.interceptors.request.use(
  (config) => {
    const token = getToken();
    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);

// Handle auth errors
api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      // Redirect to login on auth error
      window.location.href = '/ui/login';
    }
    return Promise.reject(error);
  }
);

export const configAPI = {
  // List all configurations
  listConfigs: async (): Promise<ConfigListResponse> => {
    const response = await api.get<ConfigListResponse>('/api/configs');
    return response.data;
  },

  // Get a specific configuration
  getConfig: async (id: string): Promise<APIConfig> => {
    const response = await api.get<APIConfig>(`/api/configs/${id}`);
    return response.data;
  },

  // Create a new configuration
  createConfig: async (config: APIConfigRequest): Promise<APIConfig> => {
    const response = await api.post<APIConfig>('/api/configs', config);
    return response.data;
  },

  // Update a configuration
  updateConfig: async (id: string, config: APIConfigRequest): Promise<APIConfig> => {
    const response = await api.put<APIConfig>(`/api/configs/${id}`, config);
    return response.data;
  },

  // Delete a configuration
  deleteConfig: async (id: string): Promise<void> => {
    await api.delete(`/api/configs/${id}`);
  },

  // Test a configuration
  testConfig: async (id: string): Promise<TestConfigResponse> => {
    const response = await api.post<TestConfigResponse>(`/api/configs/${id}/test`);
    return response.data;
  },

  // Set default configuration
  setDefaultConfig: async (id: string): Promise<void> => {
    await api.post(`/api/configs/${id}/set-default`);
  },

  // Get Claude format configuration
  getClaudeConfig: async (id: string, server?: string): Promise<ClaudeConfigFormat> => {
    const params = server ? { server } : {};
    const response = await api.get<ClaudeConfigFormat>(`/api/configs/${id}/claude-config`, { params });
    return response.data;
  },

  // Get API documentation
  getAPIDocs: async (): Promise<any> => {
    const response = await api.get('/api/docs');
    return response.data;
  },
};

export const userAPI = {
  listUsers: async (): Promise<User[]> => {
    const response = await api.get<{ users: User[] }>('/api/users');
    return response.data.users;
  },

  createUser: async (payload: UserCreateRequest): Promise<User> => {
    const response = await api.post<{ user: User }>('/api/users', payload);
    return response.data.user;
  },

  updateUser: async (id: number, payload: UserUpdateRequest): Promise<User> => {
    const response = await api.put<{ user: User }>(`/api/users/${id}`, payload);
    return response.data.user;
  },

  updateUserPassword: async (id: number, payload: UserPasswordRequest): Promise<void> => {
    await api.put(`/api/users/${id}/password`, payload);
  },

  updateUserStatus: async (id: number, payload: UserStatusRequest): Promise<void> => {
    await api.put(`/api/users/${id}/status`, payload);
  },

  deleteUser: async (id: number): Promise<void> => {
    await api.delete(`/api/users/${id}`);
  },

  getUserStats: async (id: number, days?: number): Promise<UserTokenStats[]> => {
    const params = days ? { days } : undefined;
    const response = await api.get<{ stats: UserTokenStats[] }>(`/api/users/${id}/stats`, { params });
    return response.data.stats;
  },

  getUserLogs: async (
    id: number,
    params?: {
      config_id?: string;
      status?: string;
      model?: string;
      sort_by?: string;
      sort_order?: string;
      search?: string;
      page?: number;
      page_size?: number;
    }
  ): Promise<LogsResult> => {
    const response = await api.get<LogsResult>(`/api/users/${id}/logs`, { params });
    return response.data;
  },
};

export default api;

