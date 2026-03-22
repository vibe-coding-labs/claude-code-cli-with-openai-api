import { apiClient } from './api';

export interface APIKey {
  id: string;
  tenant_id: string;
  name: string;
  status: 'active' | 'revoked' | 'expired';
  created_at: string;
  expires_at?: string;
  last_used_at?: string;
}

export interface CreateApiKeyRequest {
  name: string;
  expires_at?: Date;
}

export interface CreateApiKeyResponse {
  key: APIKey & { plain_key: string };
}

export interface RotateApiKeyResponse {
  key: APIKey & { plain_key: string };
}

export const apiKeyService = {
  async listApiKeys(tenantId: string): Promise<{ keys: APIKey[] }> {
    const response = await apiClient.get(`/admin/tenants/${tenantId}/api-keys`);
    return response.data;
  },

  async createApiKey(tenantId: string, data: CreateApiKeyRequest): Promise<CreateApiKeyResponse> {
    const response = await apiClient.post(`/admin/tenants/${tenantId}/api-keys`, data);
    return response.data;
  },

  async revokeApiKey(keyId: string): Promise<void> {
    await apiClient.post(`/admin/api-keys/${keyId}/revoke`);
  },

  async rotateApiKey(keyId: string, gracePeriodSeconds: number): Promise<RotateApiKeyResponse> {
    const response = await apiClient.post(`/admin/api-keys/${keyId}/rotate`, {
      grace_period_seconds: gracePeriodSeconds,
    });
    return response.data;
  },
};
