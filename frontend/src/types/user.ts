export interface User {
  id: number;
  username: string;
  role: 'admin' | 'user';
  status: 'active' | 'disabled';
  created_at: string;
  updated_at: string;
}

export interface UserCreateRequest {
  username: string;
  password: string;
  role: 'admin' | 'user';
  status: 'active' | 'disabled';
}

export interface UserUpdateRequest {
  username: string;
  role: 'admin' | 'user';
  status: 'active' | 'disabled';
}

export interface UserPasswordRequest {
  password: string;
}

export interface UserStatusRequest {
  status: 'active' | 'disabled';
}

export interface UserTokenStats {
  user_id: number;
  model: string;
  total_requests: number;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  error_count: number;
}

export interface UserLog {
  id: number;
  config_id: string;
  user_id: number;
  model: string;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  duration_ms: number;
  status: string;
  error_message?: string;
  request_body?: string;
  response_body?: string;
  request_summary?: string;
  response_preview?: string;
  created_at: string;
}

export interface LogsResult {
  logs: UserLog[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}
