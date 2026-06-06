// ============================================================================
// TeamsAgentProfile — Profile tab content for an agent on /teams.
// Stacks three existing sub-components vertically:
//   - AgentProfileTab  (display name, description, info, status)
//   - AgentRuntimeTab  (model, reasoning, env vars)
//   - AgentSkillsTab   (tools/skills toggle list)
// Each sub-component fetches its own copy of the agent; we accept the
// duplication in exchange for not having to refactor the shared panel.
// ============================================================================

'use client';

import { AgentProfileTab } from '@/components/agents/agent-profile-tab';
import { AgentRuntimeTab } from '@/components/agents/agent-runtime-tab';
import { AgentSkillsTab } from '@/components/agents/agent-skills-tab';

interface TeamsAgentProfileProps {
  agentId: string;
}

export function TeamsAgentProfile({ agentId }: TeamsAgentProfileProps) {
  return (
    <div className="space-y-8">
      <section>
        <h2 className="mb-3 font-heading text-xs font-bold uppercase tracking-wider text-muted-foreground">
          Profile
        </h2>
        <AgentProfileTab agentId={agentId} />
      </section>
      <section>
        <h2 className="mb-3 font-heading text-xs font-bold uppercase tracking-wider text-muted-foreground">
          Runtime Config
        </h2>
        <AgentRuntimeTab agentId={agentId} />
      </section>
      <section>
        <h2 className="mb-3 font-heading text-xs font-bold uppercase tracking-wider text-muted-foreground">
          Skills
        </h2>
        <AgentSkillsTab agentId={agentId} />
      </section>
    </div>
  );
}
