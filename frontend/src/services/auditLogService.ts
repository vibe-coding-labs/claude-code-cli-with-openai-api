import { apiClient } from './api';

export interface AuditEvent {
  id: string;
  tenant_id?: string;
  event_type: string;
  actor: string;
  resource: string;
  action: string;
  result: string;
  details?: string;
  ip_address: string;
  timestamp: string;
}

export interface AuditFilters {
  tenant_id?: string;
  event_type?: string;
  actor?: string;
  result?: string;
  start_time?: string;
  end_time?: string;
  limit?: number;
  offset?: number;
}

export const auditLogService = {
  async queryEvents(filters: AuditFilters = {}): Promise<{ audit_logs: AuditEvent[] }> {
    const params = new URLSearchParams();
    if (filters.tenant_id) params.append('tenant_id', filters.tenant_id);
    if (filters.event_type) params.append('event_type', filters.event_type);
    if (filters.actor) params.append('actor', filters.actor);
    if (filters.result) params.append('result', filters.result);
    if (filters.limit) params.append('limit', filters.limit.toString());
    if (filters.offset) params.append('offset', filters.offset.toString());

    const response = await apiClient.get(`/admin/audit-logs?${params.toString()}`);
    return response.data;
  },
};
