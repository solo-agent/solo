'use client';

import { useEffect, useLayoutEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { SmilePlus } from 'lucide-react';
import { apiClient } from '@/lib/api-client';
import { cn } from '@/lib/utils';
import { t } from '@/lib/i18n';
import type { MessageReaction } from '@/lib/types';

/**
 * Picker dimensions — used by the smart positioning in
 * MessageReactionPicker so the panel never overlaps the message
 * that owns the trigger (body text, reaction chips, etc.).
 */
const PICKER_WIDTH = 288; // w-72
const PICKER_HEIGHT_ESTIMATE = 320; // max-h-64 + headers + padding + safety
const PICKER_GAP = 8;

const EMOJI_GROUPS = [
  {
    label: 'frequentlyUsed',
    emojis: ['👍', '😀', '😘', '😍', '😆', '😛', '😁', '😂', '😱', '❤️', '🎉', '👀'],
  },
  {
    label: 'smileysAndPeople',
    emojis: [
      '😀', '😃', '😄', '😁', '😆', '😅', '😂', '🤣', '😊', '😇', '🙂', '🙃',
      '😉', '😌', '😍', '🥰', '😘', '😗', '😙', '😚', '😋', '😛', '😝', '😜',
      '🤪', '🤨', '🧐', '🤓', '😎', '🤩', '🥳', '😏', '😒', '😞', '😔', '😟',
      '😕', '🙁', '☹️', '😣', '😖', '😫', '😩', '🥺', '😢', '😭', '😤', '😠',
      '😡', '🤬', '🤯', '😳', '🥵', '🥶', '😱', '😨', '😰', '😥', '😓', '🤗',
      '🤔', '🫡', '🤭', '🤫', '🤥', '😶', '😐', '😑', '😬', '🙄', '😯', '😦',
      '😧', '😮', '😲', '🥱', '😴', '🤤', '😪', '😵', '🤐', '🥴', '🤢', '🤮',
      '🤧', '😷', '🤒', '🤕', '👍', '👎', '👏', '🙌', '🙏', '💪', '👋', '🤝',
    ],
  },
];

interface MessageReactionsResponse {
  reactions: MessageReaction[];
}

const EMPTY_REACTIONS: MessageReaction[] = [];

export function useMessageReactions(
  messageId: string,
  initialReactions: MessageReaction[] = EMPTY_REACTIONS,
) {
  const [reactions, setReactions] = useState(initialReactions);
  const [isSaving, setIsSaving] = useState(false);

  useEffect(() => {
    setReactions(initialReactions);
  }, [initialReactions]);

  const toggleReaction = async (emoji: string) => {
    if (isSaving) return;
    setIsSaving(true);
    try {
      const result = await apiClient.post<MessageReactionsResponse>(
        `/api/v1/messages/${messageId}/reactions`,
        { emoji },
      );
      setReactions(result.reactions);
    } finally {
      setIsSaving(false);
    }
  };

  return { reactions, isSaving, toggleReaction };
}

export function MessageReactionChips({
  reactions,
  isSaving,
  toggleReaction,
}: {
  reactions: MessageReaction[];
  isSaving: boolean;
  toggleReaction: (emoji: string) => Promise<void>;
}) {
  if (reactions.length === 0) return null;

  return (
    <div className="mt-2 flex flex-wrap items-center gap-1.5">
      {reactions.map((reaction) => (
        <button
          key={reaction.emoji}
          type="button"
          onClick={() => toggleReaction(reaction.emoji)}
          disabled={isSaving}
          aria-pressed={reaction.reacted}
          aria-label={reaction.reacted ? t('removeReaction', { emoji: reaction.emoji }) : `${reaction.emoji} ${reaction.count}`}
          className={cn(
            'inline-flex h-7 items-center gap-1 rounded-full border border-brutal-muted px-2.5 font-body text-xs',
            'bg-[var(--skin-muted-light)] text-foreground hover:bg-[var(--skin-primary-light)]',
            reaction.reacted && 'border-[var(--skin-subtle-text)]',
            'disabled:cursor-wait disabled:opacity-60',
          )}
        >
          <span>{reaction.emoji}</span>
          <span className="font-mono">{reaction.count}</span>
        </button>
      ))}
    </div>
  );
}

export function MessageReactionPicker({
  open,
  setOpen,
  isSaving,
  toggleReaction,
}: {
  open: boolean;
  setOpen: (open: boolean) => void;
  isSaving: boolean;
  toggleReaction: (emoji: string) => Promise<void>;
}) {
  const pickerRef = useRef<HTMLDivElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const [panelStyle, setPanelStyle] = useState<React.CSSProperties | null>(null);
  const [mounted, setMounted] = useState(false);

  useEffect(() => setMounted(true), []);

  useEffect(() => {
    if (!open) return;
    const close = (event: MouseEvent) => {
      const target = event.target as Node;
      if (
        !pickerRef.current?.contains(target) &&
        !panelRef.current?.contains(target)
      ) {
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', close);
    return () => document.removeEventListener('mousedown', close);
  }, [open, setOpen]);

  // Anchor the panel to the *owning message* (not the trigger) so the
  // panel sits in the empty space above/below the message — never on
  // top of the message body or reaction chips. The panel is rendered
  // with `position: fixed` so it is not clipped by the chat scroll
  // container.
  useLayoutEffect(() => {
    if (!open || !triggerRef.current) return;
    const messageEl = triggerRef.current.closest(
      '.agent-message',
    ) as HTMLElement | null;
    const anchorRect = messageEl
      ? messageEl.getBoundingClientRect()
      : triggerRef.current.getBoundingClientRect();
    const viewportWidth = window.innerWidth;
    const viewportHeight = window.innerHeight;
    const spaceBelow = viewportHeight - anchorRect.bottom;
    const spaceAbove = anchorRect.top;
    const openDown =
      spaceBelow >= PICKER_HEIGHT_ESTIMATE || spaceBelow > spaceAbove;

    // Horizontal: right-align the panel with the right edge of the
    // message so it appears visually attached to the message corner.
    const desiredLeft = anchorRect.right - PICKER_WIDTH;
    const left = Math.max(
      PICKER_GAP,
      Math.min(desiredLeft, viewportWidth - PICKER_WIDTH - PICKER_GAP),
    );
    // Vertical: sit just outside the message — below it (down) or
    // above it (up) — so the message body and reaction chips stay clear.
    const desiredTop = openDown
      ? anchorRect.bottom + PICKER_GAP
      : anchorRect.top - PICKER_HEIGHT_ESTIMATE - PICKER_GAP;
    const top = Math.max(
      PICKER_GAP,
      Math.min(desiredTop, viewportHeight - PICKER_HEIGHT_ESTIMATE - PICKER_GAP),
    );
    setPanelStyle({ position: 'fixed', left, top, zIndex: 50 });
  }, [open]);

  const handleSelect = async (emoji: string) => {
    await toggleReaction(emoji);
    setOpen(false);
  };

  return (
    <div ref={pickerRef} className="relative">
      <button
        ref={triggerRef}
        type="button"
        onClick={(event) => {
          event.stopPropagation();
          setOpen(!open);
        }}
        disabled={isSaving}
        aria-label={t('addReaction')}
        aria-expanded={open}
        aria-haspopup="dialog"
        className={cn(
          'btn-brutal btn-brutal-sm flex h-7 w-7 !rounded-lg items-center justify-center p-0',
          'opacity-0 translate-x-2 transition-[opacity,transform] duration-200 group-hover:translate-x-0 group-hover:opacity-100 group-focus-within:translate-x-0 group-focus-within:opacity-100',
          open && 'opacity-100 translate-x-0',
          'disabled:cursor-wait disabled:opacity-60',
        )}
      >
        <SmilePlus className="h-3.5 w-3.5" />
      </button>

      {open && mounted && typeof document !== 'undefined' &&
        createPortal(
          <div ref={panelRef}>
            <EmojiPickerPanel
              isSaving={isSaving}
              onSelect={handleSelect}
              style={panelStyle ?? undefined}
            />
          </div>,
          document.body,
        )}
    </div>
  );
}

export function EmojiPickerPanel({
  isSaving = false,
  onSelect,
  className,
  style,
}: {
  isSaving?: boolean;
  onSelect: (emoji: string) => void | Promise<void>;
  /**
   * Legacy positioning API. When set, the panel is rendered with
   * `position: absolute` and the className decides placement
   * (e.g. message-input.tsx still uses this).
   */
  className?: string;
  /**
   * Smart positioning API. When set, the panel is rendered with
   * `position: fixed` and the caller has computed coordinates in
   * viewport space (MessageReactionPicker does this so the panel
   * never overlaps the message that owns the trigger).
   */
  style?: React.CSSProperties;
}) {
  return (
    <div
      role="dialog"
      aria-label={t('emojiPicker')}
      className={cn(
        'w-72 overflow-hidden rounded-xl border border-[var(--skin-rule)] bg-[var(--skin-muted-light)] shadow-brutal-sm',
        className,
      )}
      style={style}
      onClick={(event) => event.stopPropagation()}
    >
      <div className="max-h-64 overflow-y-auto px-3 py-3">
        {EMOJI_GROUPS.map((group) => (
          <section key={group.label}>
            <h3 className="mb-1.5 mt-1 text-sm font-medium text-foreground">{t(group.label as 'frequentlyUsed' | 'smileysAndPeople')}</h3>
            <EmojiGrid emojis={group.emojis} isSaving={isSaving} onSelect={onSelect} />
          </section>
        ))}
      </div>
    </div>
  );
}

function EmojiGrid({
  emojis,
  isSaving,
  onSelect,
}: {
  emojis: string[];
  isSaving: boolean;
  onSelect: (emoji: string) => void | Promise<void>;
}) {
  return (
    <div className="grid grid-cols-8 gap-1">
      {emojis.map((emoji, index) => (
        <button
          key={`${emoji}-${index}`}
          type="button"
          onClick={() => onSelect(emoji)}
          disabled={isSaving}
          className="flex h-7 w-7 items-center justify-center rounded-md text-base transition-colors hover:bg-brutal-primary-light focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-brutal-primary disabled:opacity-50"
          aria-label={t('insertEmoji', { emoji })}
        >
          {emoji}
        </button>
      ))}
    </div>
  );
}
