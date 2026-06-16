// ============================================================================
// templates-api — small wrappers around the agent template endpoints (1.7).
//
// GET  /api/v1/templates             → list built-in team templates
// POST /api/v1/templates/{id}/apply  → instantiate a template into a user-owned
//                                      set of agents + assigns_to relationships.
// ============================================================================

import { apiClient } from '@/lib/api-client';

export interface Template {
  id: string;
  name: string;
  description: string;
  category: string;
  icon: string;
}

export interface ApplyTemplateResult {
  created_agent_ids: string[];
  created_relationship_ids: string[];
  template_id: string;
}

// listTemplates — fetch all built-in templates, sorted by name ASC server-side.
// Returns [] (never null) when the table is empty.
export async function listTemplates(): Promise<Template[]> {
  const res = await apiClient.get<Template[]>('/api/v1/templates');
  return Array.isArray(res) ? res : [];
}

// applyTemplate — instantiate the template under the given owner, creating
// N agents and N-1 assigns_to edges in one transaction.
export async function applyTemplate(
  id: string,
  ownerId: string,
  modelProvider?: string,
): Promise<ApplyTemplateResult> {
  return apiClient.post<ApplyTemplateResult>(
    `/api/v1/templates/${id}/apply`,
    { owner_id: ownerId, model_provider: modelProvider || '' },
  );
}
