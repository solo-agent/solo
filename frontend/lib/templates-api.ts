import { apiClient } from '@/lib/api-client';

export interface Template {
  id: string;
  name: string;
  description: string;
  category: string;
  icon: string;
  member_count: number;
}

export interface ApplyTemplateResult {
  agent_ids?: string[];
  created_agent_ids: string[];
  created_relationship_ids: string[];
  template_id: string;
}

export async function listTemplates(): Promise<Template[]> {
  const res = await apiClient.get<Template[]>('/api/v1/templates');
  return Array.isArray(res) ? res : [];
}

export async function applyTemplate(
  id: string,
  modelProvider: string,
): Promise<ApplyTemplateResult> {
  return apiClient.post<ApplyTemplateResult>(
    `/api/v1/templates/${id}/apply`,
    { model_provider: modelProvider },
  );
}
