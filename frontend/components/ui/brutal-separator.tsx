// ============================================================================
// BrutalSeparator — v3.3: chunky torn-paper divider used between hero
// sections in the Teams agent profile. Replaces a plain <hr> with a
// "═══ ✕ ═══" composition that matches the energy of the surrounding
// chunky tilted sticker headers. The center glyph (default ✕) carries
// the same brutalist weight (font-black, 2px black ring via shadow) as
// the section stickers, so the divider reads as "another brutal object"
// rather than a passive line.
// ============================================================================

import * as React from "react";
import { cn } from "@/lib/utils";

export interface BrutalSeparatorProps {
  className?: string;
}

export function BrutalSeparator({ className }: BrutalSeparatorProps) {
  return (
    <div
      role="separator"
      aria-orientation="horizontal"
      className={cn("h-0 border-t-2 border-black", className)}
    />
  );
}
