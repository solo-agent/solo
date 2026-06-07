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
      className="fixed z-[100] border-2 border-black bg-white shadow-brutal-sm rounded-none"
      style={{
        left: anchor.left,
        width: anchor.width,
        bottom: window.innerHeight - anchor.top,
      }}
      role="listbox"
      aria-label="提及成员选择"
    >
      {suggestions.length === 0 && searchQuery !== '' ? (
        <div className="py-3 text-center text-xs text-muted-foreground">
          没有匹配的成员
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
                  'flex w-full items-center gap-2 px-3 py-2 text-left text-sm transition-colors',
                  index === selectedIndex
                    ? 'bg-accent text-accent-foreground'
                    : 'text-foreground hover:bg-accent/50',
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
