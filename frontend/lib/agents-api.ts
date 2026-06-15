// ============================================================================
// agents-api — small wrappers around agent-scoped REST endpoints.
//
// Right now this exposes mention-candidates (T1.4.1) and collaborators
// (T1.5.1). The generic CRUD for /api/v1/agents lives in apiClient calls
// scattered through hooks; we only centralize endpoints that don't have a
// dedicated hook.
// ============================================================================

import { apiClient } from '@/lib/api-client';

export interface MentionCandidate {
  id: string;
  name: string;
  weight: number;
}

// getMentionCandidates — fetch the agents that the given agent has
// assigns_to relationships with, ordered by weight DESC then name ASC.
// Returns [] (never null) when the agent has no outgoing assigns_to edges.
export async function getMentionCandidates(agentId: string): Promise<MentionCandidate[]> {
  const res = await apiClient.get<MentionCandidate[]>(
    `/api/v1/agents/${agentId}/mention-candidates`,
  );
  return Array.isArray(res) ? res : [];
}

// getCollaborators — fetch the agents that share a collaborates_with edge
// with the given agent in either direction, ordered by weight DESC then
// name ASC. Returns [] (never null) when the agent has no collaborators.
export async function getCollaborators(agentId: string): Promise<MentionCandidate[]> {
  const res = await apiClient.get<MentionCandidate[]>(
    `/api/v1/agents/${agentId}/collaborators`,
  );
  return Array.isArray(res) ? res : [];
}
