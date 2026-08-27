'use client';

import { useRef } from 'react';
import { Bookmark, Forward, GitBranch, MoreHorizontal } from 'lucide-react';
import type { Message } from '@/lib/types';
import { t } from '@/lib/i18n';

export function MessageReuseMenu({
  message,
  onFavorite,
  onForward,
  onBranch,
}: {
  message: Message;
  onFavorite?: (message: Message) => void;
  onForward?: (message: Message) => void;
  onBranch?: (message: Message) => void;
}) {
  const detailsRef = useRef<HTMLDetailsElement>(null);
  if (!onFavorite && !onForward && !onBranch) return null;

  const run = (action: (message: Message) => void) => {
    detailsRef.current?.removeAttribute('open');
    action(message);
  };

  return (
    <details ref={detailsRef} className="relative" onClick={(event) => event.stopPropagation()}>
      <summary
        className="btn-brutal btn-brutal-sm flex h-7 w-7 cursor-pointer list-none items-center justify-center p-0 [&::-webkit-details-marker]:hidden"
        aria-label={t('messageMoreActions')}
        title={t('messageMoreActions')}
      >
        <MoreHorizontal className="h-3.5 w-3.5" />
      </summary>
      <div className="absolute right-0 top-9 z-30 w-40 rounded-lg border border-brutal-border bg-card p-1.5 shadow-card">
        {onFavorite && <MenuItem icon={Bookmark} label={t('favoriteMessage')} onClick={() => run(onFavorite)} />}
        {onForward && <MenuItem icon={Forward} label={t('forwardMessage')} onClick={() => run(onForward)} />}
        {onBranch && <MenuItem icon={GitBranch} label={t('branchFromMessage')} onClick={() => run(onBranch)} />}
      </div>
    </details>
  );
}

function MenuItem({ icon: Icon, label, onClick }: { icon: typeof Bookmark; label: string; onClick: () => void }) {
  return (
    <button type="button" onClick={onClick} className="flex w-full items-center gap-2 rounded-md px-2.5 py-2 text-left font-body text-xs hover:bg-brutal-muted-light">
      <Icon className="h-3.5 w-3.5" />
      {label}
    </button>
  );
}
