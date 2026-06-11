// ============================================================================
// MentionDropdown — @mention autocomplete dropdown rendered via portal
// - Renders at document.body level to avoid overflow clipping
// - Shows member list filtered by search query
// - Empty state: "没有匹配的成员"
// - Auto-scrolls highlighted item into view
// ============================================================================

'use client';

import { useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import { Bot, User } from 'lucide-react';
import { cn } from '@/lib/utils';
import { t } from '@/lib/i18n';
import type { ChannelMember } from '@/lib/types';

// ---- Types ----

export interface DropdownAnchor {
  top: number;
  left: number;
  width: number;
}

interface MentionDropdownProps {
  suggestions: readonly { member: ChannelMember; matchText: string }[];
  selectedIndex: number;
  searchQuery: string;
  anchor: DropdownAnchor;
  onSelect: (index: number) => void;
}

// ---- Component ----

export function MentionDropdown({
  suggestions,
  selectedIndex,
  searchQuery,
  anchor,
  onSelect,
}: MentionDropdownProps) {
  const listRef = useRef<HTMLDivElement>(null);

  // Auto-scroll highlighted item into view
  useEffect(() => {
    if (!listRef.current) return;
    const item = listRef.current.querySelector(
      `[data-index="${selectedIndex}"]`,
    ) as HTMLElement | null;
    item?.scrollIntoView({ block: 'nearest' });
  }, [selectedIndex]);

  return createPortal(
    <div
      // v3.1: panel switched from white+sm-shadow to cream+brutal-shadow.
      // White-on-cream floated like a Material card; cream-on-cream with
      // 2px black border + 5px hard shadow reads as brutalist "a piece
      // of the page" instead of a modern soft dropdown.
      className="fixed z-[100] border-2 border-black bg-brutal-cream shadow-brutal rounded-none"
      style={{
        left: anchor.left,
        width: anchor.width,
        bottom: window.innerHeight - anchor.top,
      }}
      role="listbox"
      aria-label={t('mentionSelect')}
    >
      {suggestions.length === 0 && searchQuery !== '' ? (
        <div className="py-3 text-center text-xs text-muted-foreground">
          No matching members
        </div>
      ) : (
        <div ref={listRef} className="max-h-48 overflow-y-auto py-1">
          {suggestions.map((suggestion, index) => {
            const isAgent = suggestion.member.member_type === 'agent';
            return (
              <button
                key={suggestion.member.member_id}
                data-index={index}
                className={cn(
                  'flex w-full items-center gap-2 px-3 py-2 text-left text-sm font-bold transition-colors',
                  // v3.1: 3-tier color hierarchy — cream (default, blends
                  // with panel) → yellow (hover/active) → black (selected).
                  // White default looked flat on the new cream panel.
                  index === selectedIndex
                    ? 'bg-black text-brutal-primary'
                    : 'bg-brutal-cream text-foreground hover:bg-brutal-primary hover:text-black',
                )}
                role="option"
                aria-selected={index === selectedIndex}
                onMouseDown={(e) => {
                  // Prevent textarea blur on click
                  e.preventDefault();
                  onSelect(index);
                }}
              >
                {isAgent ? (
                  <Bot className="h-4 w-4 flex-shrink-0 text-brutal-success" />
                ) : (
                  <User className="h-4 w-4 flex-shrink-0 text-brutal-info" />
                )}
                <span className="font-medium">
                  {suggestion.member.display_name}
                </span>
                {isAgent && (
                  <span className="ml-auto text-[10px] text-brutal-success">
                    Agent
                  </span>
                )}
              </button>
            );
          })}
        </div>
      )}
    </div>,
    document.body,
  );
}
