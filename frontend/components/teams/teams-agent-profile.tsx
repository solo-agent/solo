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

import { useState, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { Trash2 } from 'lucide-react';
import { AgentProfileTab } from '@/components/agents/agent-profile-tab';
import { AgentRuntimeTab } from '@/components/agents/agent-runtime-tab';
import { AgentSkillsTab } from '@/components/agents/agent-skills-tab';
import { BrutalSeparator } from '@/components/ui/brutal-separator';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  DialogCloseButton,
} from '@/components/ui/dialog';
import { useAgents } from '@/lib/hooks/use-agents';
import { useToast } from '@/components/ui/toast';
import { t } from '@/lib/i18n';

interface TeamsAgentProfileProps {
  agentId: string;
  /**
   * Called after the delete API returns success. The parent owns the
   * canonical agent list (sidebar) and should refetch + clear its
   * selection so the deleted agent disappears without a manual refresh.
   */
  onAgentDeleted?: (deletedId: string) => void;
}

export function TeamsAgentProfile({ agentId, onAgentDeleted }: TeamsAgentProfileProps) {
  const router = useRouter();
  const { agents, deleteAgent } = useAgents();
  const { showToast } = useToast();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const agentName = agents.find((a) => a.id === agentId)?.name ?? 'this agent';

  const handleConfirmDelete = useCallback(async () => {
    setDeleting(true);
    try {
      await deleteAgent(agentId);
      showToast(t('agentDeleteSuccess'), 'success');
      onAgentDeleted?.(agentId);
      const remaining = agents.filter((a) => a.id !== agentId);
      if (remaining.length > 0) {
        router.replace(`/teams?agent=${remaining[0].id}&tab=profile`, { scroll: false });
      } else {
        router.replace('/teams', { scroll: false });
      }
    } catch {
      showToast(t('agentDeleteError'), 'error');
    } finally {
      setDeleting(false);
      setConfirmOpen(false);
    }
  }, [agentId, agents, deleteAgent, onAgentDeleted, router, showToast]);

  return (
    <div className="space-y-6">
      <AgentProfileTab agentId={agentId} />
      <BrutalSeparator />
      <AgentRuntimeTab agentId={agentId} />
      <BrutalSeparator />
      <AgentSkillsTab agentId={agentId} />

      {/* Danger zone — delete agent (soft delete: retains DM history) */}
      <BrutalSeparator />
      <div className="flex justify-end pt-2">
        <Button
          type="button"
          variant="danger"
          onClick={() => setConfirmOpen(true)}
        >
          <Trash2 className="mr-2 h-4 w-4" />
          {t('agentDeleteButton')}
        </Button>
      </div>

      <Dialog open={confirmOpen} onOpenChange={(open) => !deleting && setConfirmOpen(open)}>
        <DialogHeader>
          <DialogTitle>{t('agentDeleteTitle')}</DialogTitle>
          <DialogCloseButton onClick={() => setConfirmOpen(false)} />
        </DialogHeader>
        <DialogDescription>
          {t('agentDeleteDesc', { name: agentName })}
        </DialogDescription>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => setConfirmOpen(false)}
            disabled={deleting}
            className="min-w-[100px]"
          >
            {t('cancel')}
          </Button>
          <Button
            type="button"
            variant="danger"
            onClick={handleConfirmDelete}
            disabled={deleting}
            className="min-w-[100px]"
          >
            {deleting ? t('deleting') : t('delete')}
          </Button>
        </DialogFooter>
      </Dialog>
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
