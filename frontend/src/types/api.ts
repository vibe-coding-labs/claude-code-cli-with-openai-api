export interface APIConfig {
  id: string;
  name: string;
  description?: string;
  openai_api_key: string;
  openai_base_url: string;
  azure_api_version?: string;
  anthropic_api_key?: string;
  big_model: string;
  middle_model: string;
  small_model: string;
  max_tokens_limit: number;
  min_tokens_limit: number;
  request_timeout: number;
  custom_headers?: Record<string, string>;
  enabled: boolean;
  created_at: string;
  updated_at: string;
  last_tested_at?: string;
  last_test_status?: string;
  last_test_error?: string;
}

export interface APIConfigRequest {
  name: string;
  description?: string;
  openai_api_key: string;
  openai_base_url?: string;
  azure_api_version?: string;
  anthropic_api_key?: string;
  big_model?: string;
  middle_model?: string;
  small_model?: string;
  max_tokens_limit?: number;
  min_tokens_limit?: number;
  request_timeout?: number;
  custom_headers?: Record<string, string>;
  enabled?: boolean;
}

export interface ConfigListResponse {
  configs: APIConfig[];
  default_config_id?: string;
}

export interface TestConfigResponse {
  status: string;
  message: string;
  model_used?: string;
  response_id?: string;
  error?: string;
  error_type?: string;
  timestamp: string;
  suggestions?: string[];
}

export interface ClaudeConfigFormat {
  ANTHROPIC_BASE_URL: string;
  ANTHROPIC_API_KEY: string;
  config_id: string;
  config_name: string;
}

export interface APIError {
  type: string;
  message: string;
}

