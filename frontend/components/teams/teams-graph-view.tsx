// ============================================================================
// TeamsGraphView — the "Graph" section's content in the right main panel.
// Renders a horizontal flow of role group cards (from keyword inference) with
// arrows between them. Each card is clickable to expand and list the agents
// in that role. This is the same structure view that existed in v1.5 of
// /teams, but now it's the only view for the Graph section.
// ============================================================================

'use client';

import { useMemo, useState, useCallback } from 'react';
import { ArrowRight, Bot } from 'lucide-react';
import {
  inferAgentGroup,
  GROUP_ORDER,
  type TeamGroup,
} from './infer-agent-group';
import { cn } from '@/lib/utils';
import { t } from '@/lib/i18n';
import type { Agent } from '@/lib/types';

interface TeamsGraphViewProps {
  agents: Agent[];
  /** Called when the user picks an agent from inside a group card. */
  onSelectAgent: (agentId: string) => void;
}

export function TeamsGraphView({ agents, onSelectAgent }: TeamsGraphViewProps) {
  const [expanded, setExpanded] = useState<Set<TeamGroup>>(new Set());

  const grouped = useMemo(() => {
    const map = new Map<TeamGroup, Agent[]>();
    for (const g of GROUP_ORDER) map.set(g, []);
    for (const agent of agents) {
      map.get(inferAgentGroup(agent.system_prompt))!.push(agent);
    }
    return map;
  }, [agents]);

  const toggle = useCallback((group: TeamGroup) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(group)) next.delete(group);
      else next.add(group);
      return next;
    });
  }, []);

  const visibleGroups = GROUP_ORDER.filter((g) => (grouped.get(g) ?? []).length > 0);

  if (visibleGroups.length === 0) {
    return (
      <div className="flex h-full items-center justify-center p-8 text-center">
        <p className="font-body text-sm text-muted-foreground">
          No agents yet. Create agents to view the organization graph.
        </p>
      </div>
    );
  }

  return (
    <div className="h-full overflow-auto p-6">
      <div className="min-w-max rounded-none border-2 border-black bg-white p-6 shadow-brutal">
        <div className="flex flex-col items-stretch gap-3 md:flex-row md:items-stretch md:gap-0">
          {visibleGroups.map((group, idx) => {
            const items = grouped.get(group) ?? [];
            const isExpanded = expanded.has(group);
            return (
              <div key={group} className="flex flex-col items-center md:flex-row">
                <div
                  className={cn(
                    'w-[160px] border-2 border-black bg-brutal-cream p-3',
                    isExpanded && 'border-brutal-primary shadow-brutal',
                  )}
                >
                  <button
                    type="button"
                    onClick={() => toggle(group)}
                    className="w-full text-left"
                    aria-expanded={isExpanded}
                    aria-label={t('expandGroup', { action: isExpanded ? t('collapse') : t('expand'), group })}
                  >
                    <div className="flex items-center justify-between">
                      <h3 className="font-heading text-sm font-bold">{group}</h3>
                      <span className="font-mono text-[10px] text-muted-foreground">
                        {items.length}
                      </span>
                    </div>
                    <div className="mt-2 space-y-1.5">
                      {(isExpanded ? items : items.slice(0, 3)).map((agent) => (
                        <div
                          key={agent.id}
                          className="flex items-center gap-2"
                        >
                          <div className="flex h-5 w-5 flex-shrink-0 items-center justify-center border-2 border-black bg-brutal-primary">
                            <Bot className="h-3 w-3 text-white" />
                          </div>
                          <span className="truncate font-body text-xs">{agent.name}</span>
                        </div>
                      ))}
                      {!isExpanded && items.length > 3 && (
                        <p className="pl-7 font-mono text-[10px] text-muted-foreground">
                          +{items.length - 3} more...
                        </p>
                      )}
                    </div>
                  </button>
                  {isExpanded && (
                    <div className="mt-3 grid grid-cols-1 gap-2 border-t-2 border-brutal-muted pt-3">
                      {items.map((agent) => (
                        <button
                          key={agent.id}
                          type="button"
                          onClick={() => onSelectAgent(agent.id)}
                          className="border-2 border-black bg-white px-2 py-1.5 text-left text-xs font-bold hover:bg-brutal-primary/60"
                        >
                          → {agent.name}
                        </button>
                      ))}
                    </div>
                  )}
                </div>
                {idx < visibleGroups.length - 1 && (
                  <div className="flex items-center justify-center px-3 py-2 md:py-0">
                    <ArrowRight className="h-5 w-5 rotate-90 text-brutal-muted md:rotate-0" />
                  </div>
                )}
              </div>
            );
          })}
        </div>
      </div>
      <p className="mt-3 text-center font-mono text-[10px] text-muted-foreground">
        Scroll horizontally for more · Click group card to expand agent list
      </p>
    </div>
  );
}
