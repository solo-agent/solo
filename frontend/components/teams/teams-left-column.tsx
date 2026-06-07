// ============================================================================
// TeamsLeftColumn — the 220px-wide left navigation column on /teams.
// - Section header style is unified across pages: chevron + UPPERCASE name
//   + plain count (no badge). Decorative icons are dropped — the chevron
//   acts as the visual marker.
// - Graph has no children: no chevron. To preserve a leading marker it
//   keeps the Network icon; click just emits onSelectGraph (no toggle).
// - Agents / Humans have children: header click toggles expand/collapse.
// - Section item click (agent or human row) emits onSelectAgent / onSelectHuman.
// - Selected item gets the brutalist yellow selection style.
// ============================================================================

'use client';

import { useState, useCallback } from 'react';
import { ChevronDown, Network, Plus } from 'lucide-react';
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
  onCreateAgent?: () => void;
}

type SectionKey = 'graph' | 'agents' | 'humans';

const SECTION_HEADER =
  'flex w-full items-center gap-1.5 px-3 py-2 text-left text-xs font-bold uppercase tracking-wider font-heading';
const SECTION_COUNT = 'ml-auto text-xs tabular-nums opacity-50';

export function TeamsLeftColumn({
  agents,
  humans,
  selection,
  onSelectGraph,
  onSelectAgent,
  onSelectHuman,
  onCreateAgent,
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
    <div className="flex h-full flex-col overflow-hidden border-r-2 border-black bg-brutal-cream">
      {/* Page label — matches Sidebar / Tasks / Computers top label style */}
      <div className="flex items-center h-14 border-b-2 border-black bg-brutal-cream px-4">
        <span className="font-heading text-lg font-bold">Teams</span>
      </div>

      {/* Sections */}
      <div className="flex-1 overflow-y-auto pt-0 pb-2">
        {/* Graph — no children, so no ChevronDown; keep Network as the
            leading marker; click selects the view */}
        <button
          type="button"
          onClick={onSelectGraph}
          className={cn(
            SECTION_HEADER,
            selection?.kind === 'graph'
              ? 'bg-brutal-primary text-black border-black shadow-brutal-sm'
              : 'text-muted-foreground',
          )}
          aria-label="进入 Graph 视图"
        >
          <Network className="h-3.5 w-3.5" aria-hidden="true" />
          <span>Graph</span>
          <span className={SECTION_COUNT}>{agents.length}</span>
        </button>

        {/* Agents */}
        <div className="group flex items-center justify-between border-2 border-transparent hover:border-black transition-all">
          <button
            type="button"
            onClick={() => toggle('agents')}
            className={cn(SECTION_HEADER, 'flex-1 text-muted-foreground')}
            aria-label="展开或折叠 Agents"
            aria-expanded={isAgentsExpanded}
          >
            <ChevronDown
              aria-hidden="true"
              className={cn(
                'h-3 w-3 transition-transform',
                isAgentsExpanded ? 'rotate-0' : '-rotate-90',
              )}
            />
            <span>Agents</span>
            <span className={SECTION_COUNT}>{agents.length}</span>
          </button>
          {onCreateAgent && (
            <button
              onClick={onCreateAgent}
              className="mr-2 flex h-5 w-5 items-center justify-center border-2 border-transparent text-sidebar-muted-foreground group-hover:border-black group-hover:text-black hover:bg-brutal-primary/40 active:bg-brutal-primary active:text-black active:ring-2 active:ring-black transition-all cursor-pointer"
              aria-label="创建 Agent"
            >
              <Plus className="h-3.5 w-3.5" />
            </button>
          )}
        </div>
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
          className={cn(SECTION_HEADER, 'text-muted-foreground')}
          aria-label="展开或折叠 Humans"
          aria-expanded={isHumansExpanded}
        >
          <ChevronDown
            aria-hidden="true"
            className={cn(
              'h-3 w-3 transition-transform',
              isHumansExpanded ? 'rotate-0' : '-rotate-90',
            )}
          />
          <span>Humans</span>
          <span className={SECTION_COUNT}>{humans.length}</span>
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
