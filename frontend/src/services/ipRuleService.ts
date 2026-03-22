import { apiClient } from './api';

export interface IPRule {
  id: string;
  tenant_id?: string;
  rule_type: 'whitelist' | 'blacklist';
  ip_address: string;
  description: string;
  created_at: string;
}

export interface CreateIPRuleRequest {
  rule_type: string;
  ip_address: string;
  description?: string;
}

export const ipRuleService = {
  async listRules(tenantId: string): Promise<{ ip_rules: IPRule[] }> {
    const response = await apiClient.get(`/admin/tenants/${tenantId}/ip-rules`);
    return response.data;
  },

  async createRule(tenantId: string, data: CreateIPRuleRequest): Promise<{ ip_rule: IPRule }> {
    const response = await apiClient.post(`/admin/tenants/${tenantId}/ip-rules`, data);
    return response.data;
  },

  async deleteRule(ruleId: string): Promise<void> {
    await apiClient.delete(`/admin/ip-rules/${ruleId}`);
  },
};
