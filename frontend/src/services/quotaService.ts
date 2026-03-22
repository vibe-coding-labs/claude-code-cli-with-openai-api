import { apiClient } from './api';

export interface Quota {
  id?: string;
  tenant_id: string;
  quota_type: string;
  period: string;
  limit: number;
  current_usage: number;
  reset_at: string;
  updated_at?: string;
}

export interface SetQuotaRequest {
  quota_type: string;
  period: string;
  limit: number;
}

export const quotaService = {
  async getQuotas(tenantId: string): Promise<{ quotas: Quota[] }> {
    const response = await apiClient.get(`/admin/tenants/${tenantId}/quota`);
    return response.data;
  },

  async setQuota(tenantId: string, data: SetQuotaRequest): Promise<{ quota: Quota }> {
    const response = await apiClient.post(`/admin/tenants/${tenantId}/quota`, data);
    return response.data;
  },

  async resetQuota(tenantId: string, quotaType: string): Promise<void> {
    await apiClient.post(`/admin/tenants/${tenantId}/quota/${quotaType}/reset`);
  },
};
