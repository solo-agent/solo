'use client';

import { useEffect, useMemo, useState } from 'react';
import { Check, ChevronDown, ChevronRight, Clock3, RefreshCw } from 'lucide-react';
import { MessageMarkdown } from '@/components/dashboard/message-markdown';
import { Button } from '@/components/ui/button';
import { agentRunShowsHalo } from '@/lib/agent-activity';
import { useTeamAgentActivity } from '@/lib/hooks/use-team-agent-activity';
import { cn } from '@/lib/utils';
import { t } from '@/lib/i18n';
import type { ThinkingNode, ThinkingSpace } from '@/lib/types';

type HandoffKind = 'inherited' | 'active' | 'returned';

function HandoffCard({
  kind,
  title,
  status,
  content,
  onOpenArtifactReference,
}: {
  kind: HandoffKind;
  title: string;
  status: string;
  content: string;
  onOpenArtifactReference?: (ref: string) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  return (
    <article className="border-2 border-black bg-white shadow-brutal-sm" data-handoff-kind={kind}>
      <div className="flex items-center justify-between gap-2 border-b-2 border-black px-3 py-2">
        <p className="min-w-0 truncate font-heading text-xs font-black">{title}</p>
        <span className={cn(
          'shrink-0 border border-black px-1.5 py-0.5 font-mono text-[9px] font-bold uppercase',
          kind === 'returned' ? 'bg-brutal-success-light' : kind === 'active' ? 'bg-brutal-info-light' : 'bg-brutal-muted',
        )}>
          {status}
        </span>
      </div>
      <div className={cn('relative px-3 py-2', expanded ? 'max-h-80 overflow-y-auto' : 'max-h-24 overflow-hidden')}>
        <MessageMarkdown content={content} onOpenArtifactReference={onOpenArtifactReference} />
        {!expanded && <div className="pointer-events-none absolute inset-x-0 bottom-0 h-8 bg-gradient-to-t from-white to-transparent" />}
      </div>
      <button
        type="button"
        onClick={() => setExpanded((value) => !value)}
        className="flex w-full items-center justify-center gap-1 border-t border-black px-2 py-1 font-mono text-[9px] font-bold uppercase hover:bg-brutal-muted"
      >
        {expanded ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
        {expanded ? t('thinkingContextShowLess') : t('thinkingContextShowMore')}
      </button>
    </article>
  );
}

function EmptyState({ children }: { children: string }) {
  return (
    <div className="border-2 border-dashed border-black bg-white px-3 py-2 font-mono text-[10px] text-muted-foreground">
      {children}
    </div>
  );
}

export function NodeContextPanel({
  node,
  space,
  refreshing,
  onRefresh,
  onOpenArtifactReference,
}: {
  node: ThinkingNode;
  space: ThinkingSpace;
  refreshing: boolean;
  onRefresh: (nodeId: string) => Promise<ThinkingNode>;
  onOpenArtifactReference?: (ref: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const { liveByThinkingNode } = useTeamAgentActivity();
  const runActive = agentRunShowsHalo(liveByThinkingNode.get(node.id)?.currentRun?.status);
  const siblings = useMemo(
    () => node.parent_id ? space.nodes.filter((item) => item.parent_id === node.parent_id && item.id !== node.id) : [],
    [node.id, node.parent_id, space.nodes],
  );
  const returnedChildren = useMemo(
    () => space.nodes.filter((item) => item.parent_id === node.id && item.returned_handoff),
    [node.id, space.nodes],
  );
  const refreshEligible = Boolean(
    node.agent_id
    && node.message_count > 0
    && !node.fork_handoff_pending
    && !node.returning_at
    && !node.returned_at,
  );
  const canRefresh = refreshEligible && !runActive;
  const checkpointLabel = refreshing
    ? t('thinkingCurrentStateRefreshing')
    : node.checkpoint_status === 'final'
      ? t('thinkingCurrentStateFinal')
      : node.checkpoint_status === 'stale'
        ? t('thinkingCurrentStateStale')
        : node.checkpoint_status === 'fresh'
          ? t('thinkingCurrentStateFresh')
          : t('thinkingCurrentStateMissing');

  useEffect(() => {
    setOpen(false);
    setError(null);
  }, [node.id]);

  const refresh = async () => {
    if (!canRefresh || refreshing) return;
    setError(null);
    try {
      await onRefresh(node.id);
    } catch (err) {
      setError(err instanceof Error ? err.message : t('thinkingSpaceError'));
    }
  };

  return (
    <section className="shrink-0 border-b-2 border-black bg-brutal-cream" data-thinking-node-context={node.id}>
      <button
        type="button"
        onClick={() => setOpen((value) => !value)}
        className="flex w-full items-center justify-between gap-3 px-4 py-2 text-left hover:bg-brutal-muted"
        aria-expanded={open}
      >
        <span className="flex min-w-0 items-center gap-2 font-mono text-[10px] font-bold uppercase tracking-wider">
          {open ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
          {t('thinkingContext')}
          {node.inherited_handoff && <span className="text-muted-foreground">· {t('thinkingContextInherited')}</span>}
          {siblings.length > 0 && <span className="text-muted-foreground">· {t('thinkingContextRelatedCount', { n: siblings.length })}</span>}
        </span>
        <span className="flex shrink-0 items-center gap-1 font-mono text-[9px] font-bold uppercase text-muted-foreground">
          {node.checkpoint_status === 'fresh' || node.checkpoint_status === 'final'
            ? <Check className="h-3 w-3 text-brutal-success" />
            : <Clock3 className="h-3 w-3" />}
          {checkpointLabel}
        </span>
      </button>

      {open && (
        <div className="max-h-[46vh] space-y-4 overflow-y-auto border-t-2 border-black p-4">
          {node.parent_id && (
            <div className="space-y-2">
              <h3 className="font-mono text-[10px] font-bold uppercase tracking-widest text-muted-foreground">{t('thinkingContextFromParent')}</h3>
              {node.inherited_handoff ? (
                <HandoffCard
                  kind="inherited"
                  title={t('thinkingContextInheritedHandoff')}
                  status={t('thinkingContextInherited')}
                  content={node.inherited_handoff}
                  onOpenArtifactReference={onOpenArtifactReference}
                />
              ) : node.fork_handoff_pending ? (
                <EmptyState>{t('thinkingContextParentPreparing')}</EmptyState>
              ) : (
                <EmptyState>{t('thinkingContextNoParent')}</EmptyState>
              )}
            </div>
          )}

          <div className="space-y-2">
            <div className="flex items-center justify-between gap-3">
              <h3 className="font-mono text-[10px] font-bold uppercase tracking-widest text-muted-foreground">{t('thinkingCurrentState')}</h3>
              {refreshEligible && (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="h-7 bg-white px-2 text-[10px]"
                  onClick={() => { void refresh(); }}
                  disabled={refreshing || runActive}
                  title={runActive ? t('thinkingCurrentStateBusy') : undefined}
                >
                  <RefreshCw className={cn('h-3 w-3', refreshing && 'animate-spin')} />
                  {refreshing ? t('thinkingCurrentStateRefreshing') : t('thinkingCurrentStateRefresh')}
                </Button>
              )}
            </div>
            {node.returned_handoff ? (
              <HandoffCard
                kind="returned"
                title={node.title}
                status={checkpointLabel}
                content={node.returned_handoff}
                onOpenArtifactReference={onOpenArtifactReference}
              />
            ) : node.checkpoint_handoff ? (
              <HandoffCard
                kind="active"
                title={node.title}
                status={checkpointLabel}
                content={node.checkpoint_handoff}
                onOpenArtifactReference={onOpenArtifactReference}
              />
            ) : (
              <EmptyState>{node.message_count === 0 ? t('thinkingCurrentStateStartFirst') : t('thinkingCurrentStateNotPublished')}</EmptyState>
            )}
            {error && <p className="font-mono text-[10px] font-bold text-brutal-danger">{error}</p>}
          </div>

          {siblings.length > 0 && (
            <div className="space-y-2">
              <h3 className="font-mono text-[10px] font-bold uppercase tracking-widest text-muted-foreground">{t('thinkingContextRelated')}</h3>
              {siblings.map((sibling) => sibling.returned_handoff || sibling.checkpoint_handoff ? (
                <HandoffCard
                  key={sibling.id}
                  kind={sibling.returned_handoff ? 'returned' : 'active'}
                  title={sibling.title}
                  status={sibling.returned_handoff ? t('thinkingCurrentStateFinal') : sibling.checkpoint_status === 'stale' ? t('thinkingCurrentStateStale') : t('thinkingCurrentStateFresh')}
                  content={sibling.returned_handoff || sibling.checkpoint_handoff || ''}
                  onOpenArtifactReference={onOpenArtifactReference}
                />
              ) : (
                <EmptyState key={sibling.id}>{t('thinkingContextRelatedMissing', { name: sibling.title })}</EmptyState>
              ))}
            </div>
          )}

          {returnedChildren.length > 0 && (
            <div className="space-y-2">
              <h3 className="font-mono text-[10px] font-bold uppercase tracking-widest text-muted-foreground">{t('thinkingContextChildReturns')}</h3>
              {returnedChildren.map((child) => (
                <HandoffCard
                  key={child.id}
                  kind="returned"
                  title={child.title}
                  status={t('thinkingCurrentStateFinal')}
                  content={child.returned_handoff || ''}
                  onOpenArtifactReference={onOpenArtifactReference}
                />
              ))}
            </div>
          )}
        </div>
      )}
    </section>
  );
}
