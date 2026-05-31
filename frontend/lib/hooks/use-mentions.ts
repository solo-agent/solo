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

const MENTION_REGEX = /@(\w*)$/;

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
    const globalRegex = /@(\S+)/g;
    let lastIndex = 0;
    let matchResult: RegExpExecArray | null;

    while ((matchResult = globalRegex.exec(value)) !== null) {
      const m = matchResult;
      // Text before this mention
      if (m.index > lastIndex) {
        parts.push({ text: value.slice(lastIndex, m.index), isMention: false });
      }

      // Find matching member
      const matchedMember = members.find(
        (member) => member.display_name === m[1] || member.member_id === m[1],
      );

      parts.push({
        text: m[0],
        isMention: true,
        member: matchedMember,
      });

      lastIndex = m.index + m[0].length;
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
    const globalRegex = /@(\S+)/g;
    let matchResult: RegExpExecArray | null;

    while ((matchResult = globalRegex.exec(value)) !== null) {
      const m = matchResult;
      const member = members.find((mem) => mem.display_name === m[1]);
      if (
        member &&
        member.member_type === 'agent' &&
        !ids.includes(member.member_id)
      ) {
        ids.push(member.member_id);
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
