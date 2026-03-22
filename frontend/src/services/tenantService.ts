import { apiClient } from './api';

export interface Tenant {
  id: string;
  name: string;
  description: string;
  status: 'active' | 'suspended' | 'deleted';
  created_at: string;
  updated_at: string;
  metadata?: string;
}

export interface TenantConfig {
  tenant_id: string;
  allowed_models: string[];
  default_model: string;
  custom_rate_limits: boolean;
  require_hmac: boolean;
  webhook_url: string;
  alert_email: string;
  created_at: string;
  updated_at: string;
}

export interface CreateTenantRequest {
  name: string;
  description?: string;
  status?: string;
  metadata?: string;
}

export interface UpdateTenantRequest {
  name: string;
  description?: string;
  status: string;
  metadata?: string;
}

export interface TenantFilters {
  status?: string;
  sort_by?: string;
  sort_order?: string;
  limit?: number;
  offset?: number;
}

export const tenantService = {
  async listTenants(filters: TenantFilters = {}): Promise<{ tenants: Tenant[] }> {
    const params = new URLSearchParams();
    if (filters.status) params.append('status', filters.status);
    if (filters.sort_by) params.append('sort_by', filters.sort_by);
    if (filters.sort_order) params.append('sort_order', filters.sort_order);
    if (filters.limit) params.append('limit', filters.limit.toString());
    if (filters.offset) params.append('offset', filters.offset.toString());

    const response = await apiClient.get(`/admin/tenants?${params.toString()}`);
    return response.data;
  },

  async getTenant(id: string): Promise<{ tenant: Tenant }> {
    const response = await apiClient.get(`/admin/tenants/${id}`);
    return response.data;
  },

  async createTenant(data: CreateTenantRequest): Promise<{ tenant: Tenant }> {
    const response = await apiClient.post('/admin/tenants', data);
    return response.data;
  },

  async updateTenant(id: string, data: UpdateTenantRequest): Promise<{ tenant: Tenant }> {
    const response = await apiClient.put(`/admin/tenants/${id}`, data);
    return response.data;
  },

  async deleteTenant(id: string): Promise<void> {
    await apiClient.delete(`/admin/tenants/${id}`);
  },

  async getTenantConfig(tenantId: string): Promise<{ config: TenantConfig }> {
    const response = await apiClient.get(`/admin/tenants/${tenantId}/config`);
    return response.data;
  },

  async updateTenantConfig(tenantId: string, config: Partial<TenantConfig>): Promise<{ config: TenantConfig }> {
    const response = await apiClient.put(`/admin/tenants/${tenantId}/config`, config);
    return response.data;
  },
};
