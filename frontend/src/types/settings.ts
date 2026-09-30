// frontend/src/types/settings.ts

/** System settings returned by GET /api/settings */
export interface SystemSettingsData {
  log_retention_days?: string;
  max_db_size_gb?: string;
  log_body_storage?: string;
  proxy_error_retention_days?: string;
  /** 服务级全局默认代理（config 级 proxy_url 为空时的回落层） */
  proxy_url?: string;
  proxy_type?: string;
  proxy_username?: string;
  proxy_password?: string;
}

/** Log storage stats returned by GET /api/log-stats */
export interface LogStorageStats {
  total_logs: number;
  db_size_bytes?: number;
  wal_size_bytes?: number;
  total_size_bytes?: number;
  free_pages?: number;
  total_pages?: number;
  page_size?: number;
  body_size_bytes?: number;
  db_path?: string;
}
