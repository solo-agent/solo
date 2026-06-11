// highlightSpecials — @mention / #tasknum inline highlighting with member-name whitelist.
// Replaces duplicated implementations in agent-message.tsx and thread-panel.tsx.
//
// Regex rationale:
//   (?<![a-zA-Z0-9_])
//                  — ASCII-only word boundary. @ must not be preceded by an
//                    ASCII letter/digit/underscore, so email@example.com does
//                    NOT trigger a mention on `@example`. CJK connectors like
//                    `和` are intentionally NOT in the lookbehind class, so
//                    `@leader和@designer` reaches the callback for both `@`s.
//   @              — literal @.
//   ([\p{L}\p{N}_-]+) — identifier chars: Unicode letters, Unicode digits, `_`, `-`.
//                    Greedy: `@leader和` captures `leader和`. The callback
//                    then tries all prefixes of the capture against the
//                    member whitelist, so `leader` matches even though the
//                    full capture includes the CJK connector `和`.
//   u flag         — required for \p{} Unicode property classes.

import type { ChannelMember } from '@/lib/types';

const MENTION_REGEX = /(?<![a-zA-Z0-9_])@([\p{L}\p{N}_-]+)/gu;
const TASKNUM_REGEX = /#(\d+)/g;

export function buildValidNames(members: ChannelMember[]): string[] {
  return Array.from(new Set(members.map((m) => m.display_name.toLowerCase())));
}

/** Try every prefix of `name` against the whitelist; return the longest match, or null. */
function longestValidPrefix(name: string, valid: Set<string>): string | null {
  for (let i = name.length; i > 0; i--) {
    if (valid.has(name.slice(0, i).toLowerCase())) return name.slice(0, i);
  }
  return null;
}

export function highlightSpecials(text: string, validNames: string[]): string {
  const valid = new Set(validNames);
  const parts = text.split(/(```[\s\S]*?```)/g);
  return parts
    .map((part, i) => {
      if (i % 2 === 1) return part;
      let processed = part.replace(MENTION_REGEX, (match, name) => {
        const matched = longestValidPrefix(name, valid);
        if (!matched) return match;
        const tail = name.slice(matched.length);
        return `<span class="mention-highlight">@${matched}</span>${tail}`;
      });
      processed = processed.replace(TASKNUM_REGEX, '<span class="tasknum-highlight">#$1</span>');
      return processed;
    })
    .join('');
}
