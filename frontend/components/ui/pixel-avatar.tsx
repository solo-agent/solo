'use client';

import { useMemo } from 'react';
import { cn } from '@/lib/utils';

// 8 preset pixel art patterns (7×7 grid, 0=empty 1=color1 2=color2)
// Each is a retro game character silhouette
const PATTERNS: number[][] = [
  // 0: Knight — helmet shape
  [0,0,1,1,1,0,0, 0,1,1,1,1,1,0, 1,1,0,1,0,1,1, 1,1,1,1,1,1,1, 1,1,0,1,0,1,1, 0,1,1,1,1,1,0, 0,0,1,1,1,0,0],
  // 1: Mage — pointed hat
  [0,0,0,2,0,0,0, 0,0,2,2,2,0,0, 0,1,1,1,1,1,0, 1,1,0,1,0,1,1, 1,1,1,1,1,1,1, 1,0,1,1,1,0,1, 0,1,0,1,0,1,0],
  // 2: Ranger — hood
  [0,0,1,1,1,0,0, 0,1,1,1,1,1,0, 1,1,1,1,1,1,1, 1,1,0,1,0,1,1, 1,1,1,1,1,1,1, 0,1,1,1,1,1,0, 0,1,0,1,0,1,0],
  // 3: Cleric — cross helmet
  [0,0,1,1,1,0,0, 0,1,1,2,1,1,0, 1,1,2,1,2,1,1, 1,1,1,1,1,1,1, 1,1,0,1,0,1,1, 0,1,1,1,1,1,0, 0,0,1,1,1,0,0],
  // 4: Rogue — mask
  [0,0,1,1,1,0,0, 0,1,1,1,1,1,0, 1,1,2,1,2,1,1, 1,1,1,2,1,1,1, 1,1,2,1,2,1,1, 0,1,1,1,1,1,0, 0,0,1,1,1,0,0],
  // 5: Monk — bald head
  [0,0,1,1,1,0,0, 0,1,1,1,1,1,0, 1,1,1,1,1,1,1, 1,1,0,1,0,1,1, 1,1,1,1,1,1,1, 0,1,1,1,1,1,0, 0,0,1,1,1,0,0],
  // 6: Mech — square head
  [0,1,1,1,1,1,0, 1,1,1,1,1,1,1, 1,1,2,1,2,1,1, 1,1,1,2,1,1,1, 1,1,2,1,2,1,1, 1,1,1,1,1,1,1, 0,1,1,1,1,1,0],
  // 7: Slime — round blob
  [0,0,1,1,1,0,0, 0,1,1,1,1,1,0, 1,1,1,1,1,1,1, 1,1,1,1,1,1,1, 1,1,1,1,1,1,1, 0,1,1,1,1,1,0, 0,0,1,1,1,0,0],
];

const COLOR_PAIRS: [string, string][] = [
  ['#4a6fa5', '#2d4a7a'], // Knight: steel blue
  ['#8b5cf6', '#FFD23F'], // Mage: purple + gold
  ['#5d8a4c', '#8b6914'], // Ranger: green + brown
  ['#f8f8f8', '#FF6B6B'], // Cleric: white + coral pink
  ['#1a1a1a', '#f97264'], // Rogue: black + red
  ['#f8a16f', '#FFD23F'], // Monk: orange + yellow
  ['#808080', '#74B9FF'], // Mech: gray + sky blue
  ['#88D498', '#74B9FF'], // Slime: soft green + sky blue
];

export function getPixelAvatarIndex(agentId: string, existingUrl?: string | null): number {
  if (existingUrl?.startsWith('pixel:')) {
    const idx = parseInt(existingUrl.replace('pixel:', ''), 10);
    if (idx >= 0 && idx < 8) return idx;
  }
  // Hash the ID to 0-7
  let hash = 0;
  for (let i = 0; i < agentId.length; i++) {
    hash = ((hash << 5) - hash) + agentId.charCodeAt(i);
    hash |= 0;
  }
  return Math.abs(hash) % 8;
}

interface PixelAvatarProps {
  agentId: string;
  avatarUrl?: string | null;
  size?: 'sm' | 'md';
  className?: string;
}

export function PixelAvatar({ agentId, avatarUrl, size = 'sm', className }: PixelAvatarProps) {
  const index = useMemo(
    () => getPixelAvatarIndex(agentId, avatarUrl),
    [agentId, avatarUrl],
  );
  const pattern = PATTERNS[index];
  const [color1, color2] = COLOR_PAIRS[index];
  const sizeClass = size === 'sm' ? 'pixel-avatar-sm' : 'pixel-avatar-md';

  return (
    <div
      className={cn(
        'flex items-center justify-center border-2 border-black shadow-brutal-sm bg-white',
        sizeClass,
        className,
      )}
      aria-label={`Pixel avatar ${index}`}
    >
      <div className="pixel-avatar-grid">
        {pattern.map((cell, i) => (
          <div
            key={i}
            className="pixel-avatar-cell"
            style={{
              backgroundColor: cell === 1 ? color1 : cell === 2 ? color2 : 'transparent',
            }}
          />
        ))}
      </div>
    </div>
  );
}

/** Find the next unused pixel avatar index (round-robin). Returns 0 if all used. */
export function getNextPixelAvatarIndex(existingUrls: (string | null | undefined)[]): number {
  const usedIndices = new Set<number>();
  for (const url of existingUrls) {
    if (url?.startsWith('pixel:')) {
      const idx = parseInt(url.replace('pixel:', ''), 10);
      if (idx >= 0 && idx < PIXEL_AVATAR_COUNT) usedIndices.add(idx);
    }
  }
  for (let i = 0; i < PIXEL_AVATAR_COUNT; i++) {
    if (!usedIndices.has(i)) return i;
  }
  return 0;
}

export const PIXEL_AVATAR_COUNT = 8;
