'use client';

import { useState } from 'react';
import { cn } from '@/lib/utils';
import { AlertTriangle, AtSign, CheckCircle2, ClipboardCheck, Clock3, Hash, MessageCircle, UserCheck } from 'lucide-react';
import { relativeTime } from '@/lib/utils/time';
import type { InboxAction, InboxItem as InboxItemType } from '@/lib/types';
import { Tag } from '@/components/ui/tag';
import { Button } from '@/components/ui/button';
import { t } from '@/lib/i18n';

const typeConfig: Record<InboxItemType['type'], { icon: React.ReactNode; label: string; variant: 'agent' | 'type' | 'status' }> = {
  thread_reply: {
    icon: <Hash className="h-3 w-3" />,
    label: t('inboxThreadReply'),
    variant: 'agent',
  },
  dm: {
    icon: <MessageCircle className="h-3 w-3" />,
    label: t('inboxDM'),
    variant: 'type',
  },
  mention: {
    icon: <AtSign className="h-3 w-3" />,
    label: t('inboxMention'),
    variant: 'status',
  },
};

interface InboxItemProps {
  item: InboxItemType;
  onClick: (item: InboxItemType) => void;
}

export function InboxItem({ item, onClick }: InboxItemProps) {
  const config = typeConfig[item.type];

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={() => onClick(item)}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onClick(item);
        }
      }}
      className={cn(
        'group relative flex gap-3 px-6 py-2.5 cursor-pointer transition-all border-b-2 border-black',
        'hover:bg-brutal-accent-light hover:shadow-brutal-sm hover:-translate-y-px',
        'active:translate-y-0.5 active:shadow-none',
        item.is_unread && 'border-l-[3px] border-l-brutal-accent bg-brutal-primary-light',
      )}
    >
      {/* Unread dot */}
      <div className="flex-shrink-0 mt-1.5">
        {item.is_unread ? (
          <span className="block h-2.5 w-2.5 rounded-full bg-brutal-primary border-2 border-black" />
        ) : (
          <span className="block h-2.5 w-2.5" />
        )}
      </div>

      <div className="flex-1 min-w-0">
        <div className="flex items-center justify-between gap-2">
          <div className="flex items-center gap-1.5 min-w-0">
            <span className="font-heading text-xs font-bold text-foreground truncate">
              {item.sender_name}
            </span>
          </div>
          <span className="flex-shrink-0 text-xs tabular-nums text-muted-foreground">
            {relativeTime(item.created_at)}
          </span>
        </div>

        <p className="mt-0.5 text-[11px] leading-snug text-muted-foreground truncate font-body">
          {item.type === 'dm'
            ? t('inboxDMWith', { name: item.sender_name })
            : item.type === 'thread_reply'
              ? t('inboxReplyIn', { channel: item.channel_name || t('unknown') })
              : t('inboxMentionIn', { channel: item.channel_name || t('unknown') })
          }
        </p>

        <p className="mt-0.5 text-[12px] leading-snug text-foreground/80 line-clamp-2 font-body">
          {item.content_preview}
        </p>

        <div className="mt-1.5 flex items-center gap-1.5">
          <Tag variant={config.variant}>
            {config.icon}
            {config.label}
          </Tag>
        </div>
      </div>
    </div>
  );
}

interface InboxActionItemProps {
  item: InboxAction;
  onOpenTask: (item: InboxAction) => void;
  onOpenArtifact: (item: InboxAction) => void;
  onReview: (item: InboxAction, decision: 'accept' | 'reject', reason?: string) => Promise<void>;
}

export function InboxActionItem({ item, onOpenTask, onOpenArtifact, onReview }: InboxActionItemProps) {
  const [rejecting, setRejecting] = useState(false);
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);
  const config = {
    review: { icon: <ClipboardCheck className="h-4 w-4" />, label: t('inboxActionReview') },
    waiting_input: { icon: <Clock3 className="h-4 w-4" />, label: t('inboxActionWaitingInput') },
    waiting_approval: { icon: <Clock3 className="h-4 w-4" />, label: t('inboxActionWaitingApproval') },
    failed: { icon: <AlertTriangle className="h-4 w-4" />, label: t('inboxActionFailed') },
    assigned: { icon: <UserCheck className="h-4 w-4" />, label: t('inboxActionAssigned') },
  }[item.type];
  const handled = item.state === 'handled';
  const detail = item.task_description === item.task_title
    ? ''
    : item.task_description || (item.activity_text?.startsWith('agent.') ? '' : item.activity_text);

  const review = async (decision: 'accept' | 'reject') => {
    setBusy(true);
    try {
      await onReview(item, decision, reason.trim());
    } finally {
      setBusy(false);
    }
  };

  return (
    <article className="border-b-2 border-black bg-brutal-cream px-6 py-4 transition-colors hover:bg-brutal-primary-light/40">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg border-2 border-black bg-card shadow-brutal-sm">
          {handled
            ? item.decision === 'accepted'
              ? <CheckCircle2 className="h-4 w-4" />
              : <AlertTriangle className="h-4 w-4" />
            : config.icon}
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-start justify-between gap-2">
            <div className="min-w-0">
              <p className="font-heading text-sm font-bold text-foreground">
                {handled
                  ? item.decision === 'accepted' ? t('inboxActionHandledAccepted') : t('inboxActionHandledRejected')
                  : config.label}
              </p>
              <p className="mt-0.5 truncate font-body text-xs text-muted-foreground">
                {t('inboxActionSource', { workspace: item.workspace_name, channel: item.channel_name, number: item.task_number })}
              </p>
            </div>
            <span className="flex-shrink-0 font-body text-xs text-muted-foreground">
              {handled && item.reviewer_name
                ? t('inboxActionReviewedBy', { name: item.reviewer_name, time: relativeTime(item.waiting_since) })
                : t('inboxActionWaited', { time: relativeTime(item.waiting_since) })}
            </span>
          </div>

          <h3 className="mt-2 font-heading text-base font-bold text-foreground">{item.task_title}</h3>
          {detail && <p className="mt-1 line-clamp-2 font-body text-sm text-muted-foreground">{detail}</p>}
          {item.reason && <p className="mt-1 rounded-lg border border-border bg-card px-3 py-2 font-body text-sm text-foreground">{item.reason}</p>}
          {item.source && <p className="mt-1 truncate font-body text-xs text-muted-foreground">{t('inboxActionRunSource', { source: item.source })}</p>}
          {handled && item.next_owner_name && <p className="mt-1 font-body text-xs text-muted-foreground">{t('inboxActionNextOwner', { name: item.next_owner_name })}</p>}

          {rejecting && !handled && (
            <div className="mt-3 max-w-xl">
              <textarea
                autoFocus
                value={reason}
                onChange={(event) => setReason(event.target.value)}
                placeholder={t('inboxActionRejectReason')}
                rows={2}
                className="w-full resize-none rounded-lg border-2 border-black bg-card px-3 py-2 font-body text-sm outline-none focus:ring-2 focus:ring-brutal-primary"
              />
            </div>
          )}

          <div className="mt-3 flex flex-wrap items-center gap-2">
            <Button type="button" variant="outline" size="sm" onClick={() => onOpenTask(item)}>
              {item.type === 'waiting_input' || item.type === 'waiting_approval' ? t('inboxActionReply') : t('inboxActionOpenTask')}
            </Button>
            {item.artifact_id && (
              <Button type="button" variant="outline" size="sm" onClick={() => onOpenArtifact(item)}>
                {handled
                  ? t('inboxActionEvidence', { title: item.artifact_title || t('taskArtifactRead') })
                  : t('inboxActionViewArtifact')}
              </Button>
            )}
            {!handled && item.type === 'review' ? (
              <>
                <Button type="button" variant="success" size="sm" disabled={busy} onClick={() => review('accept')}>
                  {t('inboxActionAccept')}
                </Button>
                {rejecting ? (
                  <>
                    <Button type="button" variant="danger" size="sm" disabled={busy || reason.trim() === ''} onClick={() => review('reject')}>
                      {t('inboxActionConfirmReject')}
                    </Button>
                    <Button type="button" variant="ghost" size="sm" disabled={busy} onClick={() => setRejecting(false)}>
                      {t('cancel')}
                    </Button>
                  </>
                ) : (
                  <Button type="button" variant="outline" size="sm" disabled={busy} onClick={() => setRejecting(true)}>
                    {t('inboxActionReject')}
                  </Button>
                )}
              </>
            ) : null}
          </div>
        </div>
      </div>
    </article>
  );
}
