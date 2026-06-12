// ============================================================================
// TeamsAgentProfile — Profile tab content for an agent on /teams.
// Stacks three existing sub-components vertically:
//   - AgentProfileTab  (display name, description, info, status)
//   - AgentRuntimeTab  (model, reasoning, env vars)
//   - AgentSkillsTab   (tools/skills toggle list)
// Each sub-component fetches its own copy of the agent; we accept the
// duplication in exchange for not having to refactor the shared panel.
// v3.3: color lives in the sub-components (status pill, field tags,
// avatar ornament) — no outer tag/header wrapper here.
// ============================================================================

'use client';

import { AgentProfileTab } from '@/components/agents/agent-profile-tab';
import { AgentRuntimeTab } from '@/components/agents/agent-runtime-tab';
import { AgentSkillsTab } from '@/components/agents/agent-skills-tab';
import { BrutalSeparator } from '@/components/ui/brutal-separator';

interface TeamsAgentProfileProps {
  agentId: string;
}

export function TeamsAgentProfile({ agentId }: TeamsAgentProfileProps) {
  return (
    <div className="space-y-6">
      <AgentProfileTab agentId={agentId} />
      <BrutalSeparator />
      <AgentRuntimeTab agentId={agentId} />
      <BrutalSeparator />
      <AgentSkillsTab agentId={agentId} />
    </div>
  );
}

function Section({
  header,
  headerColor,
  children,
}: {
  header: string;
  headerColor: string;
  children: React.ReactNode;
}) {
  return (
    <section>
      <span
        className={`inline-block ${headerColor} border-2 border-black px-2 py-0.5 font-heading text-[10px] font-black uppercase tracking-widest text-black shadow-brutal-sm`}
        style={{ transform: 'rotate(-0.6deg)' }}
      >
        ★ {header}
      </span>
      <div className="mt-3">{children}</div>
    </section>
  );
}
