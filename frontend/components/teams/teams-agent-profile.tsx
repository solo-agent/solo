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
import { Button } from '@/components/ui/button';
import { detailSectionClass } from '@/components/ui/detail-section';
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
  redirectAfterDelete?: boolean;
  showProfileHeader?: boolean;
  /**
   * Called after the delete API returns success. The parent owns the
   * canonical agent list (sidebar) and should refetch + clear its
   * selection so the deleted agent disappears without a manual refresh.
   */
  onAgentDeleted?: (deletedId: string) => void;
}

export function TeamsAgentProfile({
  agentId,
  redirectAfterDelete = true,
  showProfileHeader = true,
  onAgentDeleted,
}: TeamsAgentProfileProps) {
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
      if (redirectAfterDelete) {
        const remaining = agents.filter((a) => a.id !== agentId);
        if (remaining.length > 0) {
          router.replace(`/teams?agent=${remaining[0].id}&tab=profile`, { scroll: false });
        } else {
          router.replace('/teams', { scroll: false });
        }
      }
    } catch {
      showToast(t('agentDeleteError'), 'error');
    } finally {
      setDeleting(false);
      setConfirmOpen(false);
    }
  }, [agentId, agents, deleteAgent, onAgentDeleted, redirectAfterDelete, router, showToast]);

  return (
    <div className="flex h-full flex-col">
      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        <AgentProfileTab agentId={agentId} showHeader={showProfileHeader} />
        <div className={detailSectionClass()}>
          <AgentRuntimeTab agentId={agentId} />
        </div>
        <div className={detailSectionClass()}>
          <AgentSkillsTab agentId={agentId} />
        </div>
      </div>

      {/* Danger zone — delete agent (soft delete: retains DM history) */}
      <div className="border-t-2 border-black p-4 bg-brutal-cream">
        <Button
          type="button"
          variant="danger"
          onClick={() => setConfirmOpen(true)}
          className="w-full justify-center"
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
