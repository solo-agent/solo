'use client';

import { useEffect, useLayoutEffect, useRef, useState, type CSSProperties } from 'react';
import { createPortal } from 'react-dom';
import {
  Bookmark,
  Copy,
  Forward,
  GitBranch,
  ListChecks,
  MoreHorizontal,
  Pencil,
  Pin,
  PinOff,
  SquareCheckBig,
  Trash2,
  type LucideIcon,
} from 'lucide-react';
import type { Message } from '@/lib/types';
import { cn } from '@/lib/utils';
import { t } from '@/lib/i18n';

const MENU_WIDTH = 192;
const MENU_GAP = 8;

export function MessageReuseMenu({
  message,
  onCopy,
  onSelect,
  onEdit,
  onDelete,
  onAsTask,
  onPin,
  pinned,
  onFavorite,
  onForward,
  onBranch,
}: {
  message: Message;
  onCopy?: (message: Message) => void;
  onSelect?: (message: Message) => void;
  onEdit?: (message: Message) => void;
  onDelete?: (message: Message) => void;
  onAsTask?: (message: Message) => void;
  onPin?: (message: Message) => void | Promise<void>;
  pinned?: boolean;
  onFavorite?: (message: Message) => void;
  onForward?: (message: Message) => void;
  onBranch?: (message: Message) => void;
}) {
  const triggerRef = useRef<HTMLButtonElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const [mounted, setMounted] = useState(false);
  const [panelStyle, setPanelStyle] = useState<CSSProperties>({ visibility: 'hidden' });
  const hasActions = Boolean(onCopy || onSelect || onEdit || onDelete || onAsTask || onPin || onFavorite || onForward || onBranch);

  useEffect(() => setMounted(true), []);

  useEffect(() => {
    if (!open) return;
    const closeOutside = (event: MouseEvent) => {
      const target = event.target as Node;
      if (!triggerRef.current?.contains(target) && !panelRef.current?.contains(target)) setOpen(false);
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false);
    };
    const closeOnViewportMove = () => setOpen(false);
    document.addEventListener('mousedown', closeOutside);
    document.addEventListener('keydown', closeOnEscape);
    window.addEventListener('resize', closeOnViewportMove);
    window.addEventListener('scroll', closeOnViewportMove, true);
    return () => {
      document.removeEventListener('mousedown', closeOutside);
      document.removeEventListener('keydown', closeOnEscape);
      window.removeEventListener('resize', closeOnViewportMove);
      window.removeEventListener('scroll', closeOnViewportMove, true);
    };
  }, [open]);

  useLayoutEffect(() => {
    if (!open || !triggerRef.current || !panelRef.current) return;
    const triggerRect = triggerRef.current.getBoundingClientRect();
    const panelHeight = panelRef.current.getBoundingClientRect().height;
    const spaceBelow = window.innerHeight - triggerRect.bottom;
    const openDown = spaceBelow >= panelHeight + MENU_GAP || spaceBelow >= triggerRect.top;
    const left = Math.max(
      MENU_GAP,
      Math.min(triggerRect.right - MENU_WIDTH, window.innerWidth - MENU_WIDTH - MENU_GAP),
    );
    const desiredTop = openDown
      ? triggerRect.bottom + MENU_GAP
      : triggerRect.top - panelHeight - MENU_GAP;
    const top = Math.max(
      MENU_GAP,
      Math.min(desiredTop, window.innerHeight - panelHeight - MENU_GAP),
    );
    setPanelStyle({ position: 'fixed', left, top, zIndex: 80, visibility: 'visible' });
  }, [open]);

  if (!hasActions) return null;

  const run = (action: (message: Message) => void | Promise<void>) => {
    setOpen(false);
    void action(message);
  };

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        onClick={(event) => {
          event.stopPropagation();
          setOpen((value) => !value);
        }}
        className="btn-brutal btn-brutal-sm flex h-7 w-7 items-center justify-center p-0"
        aria-label={t('messageMoreActions')}
        title={t('messageMoreActions')}
        aria-expanded={open}
        aria-haspopup="menu"
      >
        <MoreHorizontal className="h-3.5 w-3.5" />
      </button>
      {open && mounted && typeof document !== 'undefined' && createPortal(
        <div
          ref={panelRef}
          role="menu"
          style={panelStyle}
          className="max-h-[calc(100vh-1rem)] w-48 overflow-y-auto rounded-lg border border-brutal-border bg-card p-1.5 shadow-card"
          onClick={(event) => event.stopPropagation()}
        >
          {onCopy && <MenuItem icon={Copy} label={t('copyMessage')} onClick={() => run(onCopy)} />}
          {onSelect && <MenuItem icon={ListChecks} label={t('selectMessage')} onClick={() => run(onSelect)} />}
          {onAsTask && <MenuItem icon={SquareCheckBig} label={t('convertToTask')} onClick={() => run(onAsTask)} />}
          {onPin && <MenuItem icon={pinned ? PinOff : Pin} label={pinned ? t('channelUnpin') : t('channelPin')} onClick={() => run(onPin)} />}
          {onFavorite && <MenuItem icon={Bookmark} label={t('favoriteMessage')} onClick={() => run(onFavorite)} />}
          {onForward && <MenuItem icon={Forward} label={t('forwardMessage')} onClick={() => run(onForward)} />}
          {onBranch && <MenuItem icon={GitBranch} label={t('branchFromMessage')} onClick={() => run(onBranch)} />}
          {onEdit && <MenuItem icon={Pencil} label={t('editMessage')} onClick={() => run(onEdit)} />}
          {onDelete && <MenuItem icon={Trash2} label={t('deleteMessage')} onClick={() => run(onDelete)} danger />}
        </div>,
        document.body,
      )}
    </>
  );
}

function MenuItem({ icon: Icon, label, onClick, danger = false }: { icon: LucideIcon; label: string; onClick: () => void; danger?: boolean }) {
  return (
    <button
      type="button"
      role="menuitem"
      onClick={onClick}
      className={cn(
        'flex w-full items-center gap-2 rounded-md px-2.5 py-2 text-left font-body text-xs hover:bg-brutal-muted-light',
        danger && 'text-brutal-danger',
      )}
    >
      <Icon className="h-3.5 w-3.5" />
      {label}
    </button>
  );
}
