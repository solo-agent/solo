import { apiClient } from '@/lib/api-client';
import type { AgentRelationship, RelationshipType } from '@/lib/types';

export interface CreateRelationshipInput {
  from_agent_id: string;
  to_agent_id: string;
  rel_type: RelationshipType;
  instruction?: string;
}

export interface UpdateRelationshipInput {
  instruction?: string;
  weight?: number;
}

export async function listRelationships(): Promise<AgentRelationship[]> {
  const res = await apiClient.get<AgentRelationship[]>('/api/v1/agent-relationships');
  return Array.isArray(res) ? res : [];
}

export async function createRelationship(input: CreateRelationshipInput): Promise<AgentRelationship> {
  return apiClient.post<AgentRelationship>('/api/v1/agent-relationships', input);
}

export async function updateRelationship(id: string, input: UpdateRelationshipInput): Promise<AgentRelationship> {
  return apiClient.patch<AgentRelationship>(`/api/v1/agent-relationships/${id}`, input);
}

export async function deleteRelationship(id: string): Promise<void> {
  await apiClient.delete(`/api/v1/agent-relationships/${id}`);
}
