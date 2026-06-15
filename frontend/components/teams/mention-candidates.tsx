// ============================================================================
// MentionCandidates — T1.4.2 smart-default UI for the @mention picker.
//
// Renders the current agent's outgoing `assigns_to` list as clickable pills.
// When the user clicks a pill, the parent (the @mention editor) is told to
// pre-fill that handle. The user can always ignore the pills and type a
// different @mention manually — this is a non-AI-inferred default.
//
// Rendering: returns `null` when there are no candidates, so the section
// disappears entirely for agents that haven't set up any delegation.
// ============================================================================

'use client';

import { useEffect, useState } from 'react';
import { getMentionCandidates, type MentionCandidate } from '@/lib/agents-api';

interface MentionCandidatesProps {
  /** The agent whose outgoing `assigns_to` list we render. */
  agentId: string;
  /** Called when the user picks a candidate pill. */
  onSelect: (candidate: MentionCandidate) => void;
}

export function MentionCandidates({ agentId, onSelect }: MentionCandidatesProps) {
  const [candidates, setCandidates] = useState<MentionCandidate[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    getMentionCandidates(agentId)
      .then((res) => {
        if (!cancelled) setCandidates(res);
      })
      .catch((err) => {
        // Soft-fail: hide the section on error rather than blocking the
        // user from typing a manual @mention.
        if (!cancelled) {
          console.error('mention-candidates load failed', err);
          setCandidates([]);
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

  if (candidates.length === 0) {
    return null;
  }

  return (
    <div className="card-brutal p-3">
      <div className="mb-2 text-xs font-bold uppercase tracking-wider">
        My assigns_to (smart default)
      </div>
      <div className="flex flex-wrap gap-2">
        {candidates.map((c) => (
          <button
            key={c.id}
            type="button"
            onClick={() => onSelect(c)}
            className="badge-brutal cursor-pointer bg-brutal-primary text-black transition-colors hover:bg-brutal-primary-light"
            data-testid={`mention-candidate-${c.name}`}
          >
            @{c.name} <span className="text-[10px]">({c.weight})</span>
          </button>
        ))}
      </div>
    </div>
  );
}
