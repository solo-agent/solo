// ============================================================================
// useMentions — @mention detection and member filtering (SOLO-52-F)
// - Detects `@` trigger based on cursor position
// - Filters channel members by typed text after @
// - Provides highlighted text parts and selected mentioned_agent_ids
// ============================================================================

'use client';

import { useMemo, useState, useCallback } from 'react';
import type { ChannelMember } from '@/lib/types';

// ---- Types ----

interface MentionSuggestion {
  member: ChannelMember;
  /** The search text used for filtering (text after @) */
  matchText: string;
}

interface TextPart {
  text: string;
  isMention: boolean;
  member?: ChannelMember;
}

interface ResolvedMention {
  start: number;
  end: number;
  members: ChannelMember[];
}

interface UseMentionsResult {
  /**
   * Parsed text parts with mention highlighting.
   * Each part is either plain text or a @mention reference.
   */
  parsedParts: TextPart[];
  /**
   * Currently visible suggestion list (filtered by typed text after @).
   */
  suggestions: MentionSuggestion[];
  /**
   * Whether to show the suggestion dropdown.
   */
  showSuggestions: boolean;
  /**
   * Index of currently highlighted suggestion (for keyboard nav).
   */
  selectedIndex: number;
  /**
   * Current @mention search query (text after the @ trigger).
   */
  searchQuery: string;
  /**
   * Select a suggestion by index. Returns the @mention text to insert.
   */
  selectSuggestion: (index: number) => string | null;
  /**
   * Handle keyboard events for navigation (ArrowUp/Down, Enter, Escape).
   * Returns true if the event was handled.
   */
  handleKeyDown: (e: React.KeyboardEvent) => boolean;
  /**
   * Reset the mention state (after selection or cancel).
   */
  resetMention: () => void;
  /**
   * IDs of agents mentioned in the current text (for sending with message).
   */
  mentionedAgentIds: string[];
}

// ---- Constants ----

const MENTION_REGEX = /@([\p{L}\p{N}_.-]*)$/u;
const MENTION_BOUNDARY_REGEX = /[\s\p{P}]/u;

function isMentionBoundary(value: string | undefined): boolean {
  return value === undefined || MENTION_BOUNDARY_REGEX.test(value);
}

export function resolveAgentMentions(
  value: string,
  members: ChannelMember[],
): ResolvedMention[] {
  const agents = members
    .filter((member) => member.member_type === 'agent' && member.display_name)
    .sort(
      (left, right) =>
        Array.from(right.display_name).length -
        Array.from(left.display_name).length,
    );
  const matches: ResolvedMention[] = [];

  for (let at = value.indexOf('@'); at >= 0; at = value.indexOf('@', at + 1)) {
    if (!isMentionBoundary(at === 0 ? undefined : value.at(at - 1))) continue;

    const afterAt = value.slice(at + 1);
    const longest = agents.find(
      (member) =>
        afterAt.startsWith(member.display_name) &&
        isMentionBoundary(afterAt.at(member.display_name.length)),
    );
    if (!longest) continue;

    const sameNameMembers = agents.filter(
      (member) => member.display_name === longest.display_name,
    );
    matches.push({
      start: at,
      end: at + 1 + longest.display_name.length,
      members: sameNameMembers,
    });
  }

  return matches;
}

// ---- Hook ----

export function useMentions(
  members: ChannelMember[],
  value: string,
  cursorPosition: number,
): UseMentionsResult {
  const [selectedIndex, setSelectedIndex] = useState(0);

  // Detect @mention trigger from cursor position
  const mentionMatch = useMemo(() => {
    if (cursorPosition <= 0) return null;

    const textBeforeCursor = value.slice(0, cursorPosition);
    const match = textBeforeCursor.match(MENTION_REGEX);
    if (!match || match.index === undefined) return null;

    return {
      start: match.index,
      query: match[1].toLowerCase(),
    };
  }, [value, cursorPosition]);

  // Filter members by search query
  const suggestions = useMemo(() => {
    if (!mentionMatch) return [];

    return members
      .filter((m) =>
        m.display_name.toLowerCase().includes(mentionMatch.query),
      )
      .map((member) => ({
        member,
        matchText: mentionMatch.query,
      }));
  }, [members, mentionMatch]);

  const showSuggestions = suggestions.length > 0;

  // Select a suggestion
  const selectSuggestion = useCallback(
    (index: number): string | null => {
      if (!mentionMatch || index < 0 || index >= suggestions.length)
        return null;

      const selected = suggestions[index];
      // Replace from @ to cursor with @display_name + space
      const before = value.slice(0, mentionMatch.start);
      const after = value.slice(cursorPosition);
      const mentionText = `@${selected.member.display_name}`;

      return `${before}${mentionText} ${after}`;
    },
    [mentionMatch, suggestions, value, cursorPosition],
  );

  // Handle keyboard events for dropdown navigation
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent): boolean => {
      if (!showSuggestions) return false;

      switch (e.key) {
        case 'ArrowDown':
          e.preventDefault();
          setSelectedIndex((prev) =>
            prev < suggestions.length - 1 ? prev + 1 : 0,
          );
          return true;
        case 'ArrowUp':
          e.preventDefault();
          setSelectedIndex((prev) =>
            prev > 0 ? prev - 1 : suggestions.length - 1,
          );
          return true;
        case 'Enter':
          if (showSuggestions && selectedIndex >= 0) {
            e.preventDefault();
            selectSuggestion(selectedIndex);
            return true;
          }
          return false;
        case 'Escape':
          if (showSuggestions) {
            e.preventDefault();
            setSelectedIndex(0);
            return true;
          }
          return false;
        default:
          return false;
      }
    },
    [showSuggestions, suggestions, selectedIndex, selectSuggestion],
  );

  const resetMention = useCallback(() => {
    setSelectedIndex(0);
  }, []);

  // Parse all @mentions in the full text for highlighted display
  const parsedParts = useMemo(() => {
    const parts: TextPart[] = [];
    let lastIndex = 0;
    for (const mention of resolveAgentMentions(value, members)) {
      // Text before this mention
      if (mention.start > lastIndex) {
        parts.push({
          text: value.slice(lastIndex, mention.start),
          isMention: false,
        });
      }

      parts.push({
        text: value.slice(mention.start, mention.end),
        isMention: true,
        member: mention.members[0],
      });

      lastIndex = mention.end;
    }

    // Remaining text after last mention
    if (lastIndex < value.length) {
      parts.push({ text: value.slice(lastIndex), isMention: false });
    }

    return parts;
  }, [value, members]);

  // Extract mentioned agent IDs from the full text
  const mentionedAgentIds = useMemo(() => {
    const ids: string[] = [];
    for (const mention of resolveAgentMentions(value, members)) {
      for (const member of mention.members) {
        if (!ids.includes(member.member_id)) {
          ids.push(member.member_id);
        }
      }
    }

    return ids;
  }, [value, members]);

  return {
    parsedParts,
    suggestions,
    showSuggestions,
    selectedIndex,
    searchQuery: mentionMatch?.query ?? '',
    selectSuggestion,
    handleKeyDown,
    resetMention,
    mentionedAgentIds,
  };
}
