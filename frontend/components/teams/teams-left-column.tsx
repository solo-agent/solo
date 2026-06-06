// ============================================================================
// TeamsLeftColumn — the 220px-wide left navigation column on /teams.
// - Graph has no children: header click just emits onSelectGraph (no toggle,
//   no ChevronDown).
// - Agents / Humans have children: header click toggles expand/collapse.
// - Section item click (agent or human row) emits onSelectAgent / onSelectHuman.
// - Selected item gets the brutalist yellow selection style.
// ============================================================================

'use client';

import { useState, useCallback } from 'react';
import { ChevronDown, Network, Bot, User } from 'lucide-react';
import { cn } from '@/lib/utils';
import { TeamsAgentItem } from './teams-agent-item';
import { TeamsHumanItem } from './teams-human-item';
import type { Agent } from '@/lib/types';

export type TeamsSelectionKind = 'graph' | 'agent' | 'human';

export interface TeamsSelection {
  kind: TeamsSelectionKind;
  id?: string;
}

interface TeamsLeftColumnProps {
  agents: Agent[];
  humans: Array<{ id: string; display_name: string; avatar_url?: string | null }>;
  selection: TeamsSelection | null;
  onSelectGraph: () => void;
  onSelectAgent: (agentId: string) => void;
  onSelectHuman: (userId: string) => void;
}

type SectionKey = 'graph' | 'agents' | 'humans';

export function TeamsLeftColumn({
  agents,
  humans,
  selection,
  onSelectGraph,
  onSelectAgent,
  onSelectHuman,
}: TeamsLeftColumnProps) {
  // Default: Agents + Humans expanded. Graph has no children, so its expand
  // state is irrelevant and not tracked.
  const [expanded, setExpanded] = useState<Set<SectionKey>>(
    () => new Set<SectionKey>(['agents', 'humans']),
  );

  const toggle = useCallback((key: SectionKey) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }, []);

  const isAgentsExpanded = expanded.has('agents');
  const isHumansExpanded = expanded.has('humans');

  return (
    <div className="flex h-full flex-col overflow-hidden border-r-2 border-black bg-white">
      {/* Page label — matches Sidebar / Tasks / Computers top label style */}
      <div className="border-b-2 border-black px-4 py-3">
        <span className="font-heading text-lg font-bold">Teams</span>
      </div>

      {/* Sections */}
      <div className="flex-1 overflow-y-auto py-2">
        {/* Graph — no children, so no ChevronDown; click selects the view */}
        <button
          type="button"
          onClick={onSelectGraph}
          className={cn(
            'flex w-full items-center gap-2 px-3 py-2 text-left text-sm font-bold',
            selection?.kind === 'graph'
              ? 'bg-brutal-pink text-black'
              : 'text-foreground hover:bg-brutal-pink/40',
          )}
          aria-label="进入 Graph 视图"
        >
          <Network className="h-4 w-4" />
          <span>Graph</span>
          <span className="ml-auto border border-black bg-brutal-yellow px-1.5 py-0.5 font-mono text-[10px]">
            {agents.length}
          </span>
        </button>

        {/* Agents */}
        <button
          type="button"
          onClick={() => toggle('agents')}
          className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm font-bold text-foreground hover:bg-brutal-pink/40"
          aria-label="展开或折叠 Agents"
          aria-expanded={isAgentsExpanded}
        >
          <ChevronDown
            aria-hidden="true"
            className={cn(
              'h-4 w-4 transition-transform',
              isAgentsExpanded ? 'rotate-0' : '-rotate-90',
            )}
          />
          <Bot className="h-4 w-4" />
          <span>Agents</span>
          <span className="ml-auto border border-black bg-brutal-stone px-1.5 py-0.5 font-mono text-[10px] text-white">
            {agents.length}
          </span>
        </button>
        {isAgentsExpanded && (
          <div>
            {agents.length === 0 ? (
              <p className="px-6 py-2 font-mono text-[10px] italic text-muted-foreground">
                暂无 agent
              </p>
            ) : (
              agents.map((agent) => (
                <TeamsAgentItem
                  key={agent.id}
                  agent={agent}
                  isSelected={
                    selection?.kind === 'agent' && selection.id === agent.id
                  }
                  onSelect={onSelectAgent}
                />
              ))
            )}
          </div>
        )}

        {/* Humans */}
        <button
          type="button"
          onClick={() => toggle('humans')}
          className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm font-bold text-foreground hover:bg-brutal-pink/40"
          aria-label="展开或折叠 Humans"
          aria-expanded={isHumansExpanded}
        >
          <ChevronDown
            aria-hidden="true"
            className={cn(
              'h-4 w-4 transition-transform',
              isHumansExpanded ? 'rotate-0' : '-rotate-90',
            )}
          />
          <User className="h-4 w-4" />
          <span>Humans</span>
          <span className="ml-auto border border-black bg-brutal-stone px-1.5 py-0.5 font-mono text-[10px] text-white">
            {humans.length}
          </span>
        </button>
        {isHumansExpanded && (
          <div>
            {humans.length === 0 ? (
              <p className="px-6 py-2 font-mono text-[10px] italic text-muted-foreground">
                暂无 human
              </p>
            ) : (
              humans.map((human) => (
                <TeamsHumanItem
                  key={human.id}
                  user={human}
                  isSelected={
                    selection?.kind === 'human' && selection.id === human.id
                  }
                  onSelect={onSelectHuman}
                />
              ))
            )}
          </div>
        )}
      </div>
    </div>
  );
}
