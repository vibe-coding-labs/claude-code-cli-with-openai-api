import axios from 'axios';

// ⚠️ 严禁随意修改！后端固定端口54988，前端固定端口54989
// 开发模式下使用绝对路径，避免 homepage 影响
const API_BASE_URL = process.env.NODE_ENV === 'development' 
  ? 'http://localhost:54988/api' 
  : '/api';

export interface User {
  id: number;
  username: string;
}

export interface AuthResponse {
  token: string;
  user: User;
  message?: string;
}

export interface InitializedResponse {
  initialized: boolean;
}

// Check if system is initialized
export const checkInitialized = async (): Promise<boolean> => {
  try {
    const response = await axios.get<InitializedResponse>(`${API_BASE_URL}/auth/initialized`);
    return response.data.initialized;
  } catch (error) {
    console.error('Failed to check initialization:', error);
    return false;
  }
};

// Initialize system with first user
export const initializeSystem = async (username: string, password: string): Promise<AuthResponse> => {
  const response = await axios.post<AuthResponse>(`${API_BASE_URL}/auth/initialize`, {
    username,
    password,
  });
  return response.data;
};

// Login user
export const login = async (username: string, password: string): Promise<AuthResponse> => {
  const response = await axios.post<AuthResponse>(`${API_BASE_URL}/auth/login`, {
    username,
    password,
  });
  return response.data;
};

// Token management
export const getToken = (): string | null => {
  return localStorage.getItem('auth_token');
};

export const setToken = (token: string): void => {
  localStorage.setItem('auth_token', token);
};

export const removeToken = (): void => {
  localStorage.removeItem('auth_token');
};

export const getCurrentUser = (): User | null => {
  const userStr = localStorage.getItem('current_user');
  if (userStr) {
    try {
      return JSON.parse(userStr);
    } catch {
      return null;
    }
  }
  return null;
};

export const setCurrentUser = (user: User): void => {
  localStorage.setItem('current_user', JSON.stringify(user));
};

export const removeCurrentUser = (): void => {
  localStorage.removeItem('current_user');
};

export const logout = (): void => {
  removeToken();
  removeCurrentUser();
};

// Check if user is authenticated
export const isAuthenticated = (): boolean => {
  return getToken() !== null;
};
