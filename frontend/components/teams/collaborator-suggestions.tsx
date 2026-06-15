// ============================================================================
// CollaboratorSuggestions — T1.5 smart-default UI for the delegate picker.
//
// Renders the current agent's `collaborates_with` partners (in either
// direction) as clickable pills. When the user clicks a pill, the parent
// (the delegate picker) is told to pre-fill that handle. The user can
// always ignore the pills and pick a different agent manually — this is a
// non-AI-inferred default.
//
// Rendering: returns `null` when there are no collaborators, so the
// section disappears entirely for agents that haven't set up any
// collaboration edges.
// ============================================================================

'use client';

import { useEffect, useState } from 'react';
import { getCollaborators, type MentionCandidate } from '@/lib/agents-api';

interface CollaboratorSuggestionsProps {
  /** The agent whose bidirectional `collaborates_with` partners we render. */
  agentId: string;
  /** Called when the user picks a collaborator pill. */
  onSelect: (candidate: MentionCandidate) => void;
}

export function CollaboratorSuggestions({ agentId, onSelect }: CollaboratorSuggestionsProps) {
  const [collaborators, setCollaborators] = useState<MentionCandidate[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    getCollaborators(agentId)
      .then((res) => {
        if (!cancelled) setCollaborators(res);
      })
      .catch((err) => {
        // Soft-fail: hide the section on error rather than blocking the
        // user from picking a collaborator manually.
        if (!cancelled) {
          console.error('collaborator-suggestions load failed', err);
          setCollaborators([]);
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, [agentId]);

  if (loading) {
    return (
      <div className="card-brutal p-3">
        <div className="text-xs font-bold uppercase tracking-wider text-muted-foreground">
          Loading…
        </div>
      </div>
    );
  }

  if (collaborators.length === 0) {
    return null;
  }

  return (
    <div className="card-brutal p-3" data-testid="collaborator-suggestions">
      <div className="mb-2 text-xs font-bold uppercase tracking-wider">
        My collaborators (smart default)
      </div>
      <div className="flex flex-wrap gap-2">
        {collaborators.map((c) => (
          <button
            key={c.id}
            type="button"
            onClick={() => onSelect(c)}
            className="badge-brutal cursor-pointer bg-brutal-primary text-black transition-colors hover:bg-brutal-primary-light"
            data-testid={`collaborator-suggestion-${c.name}`}
          >
            @{c.name} <span className="text-[10px]">({c.weight})</span>
          </button>
        ))}
      </div>
    </div>
  );
}
