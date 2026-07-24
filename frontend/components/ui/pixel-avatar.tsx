'use client';

import { Avatar as DiceBearAvatar, Style } from '@dicebear/core';
import pixelArtDefinition from '@dicebear/styles/pixel-art.json';
import { useMemo } from 'react';
import { cn } from '@/lib/utils';

export const PIXEL_ART_AVATAR_PREFIX = 'dicebear:pixel-art:';

const pixelArtStyle = new Style(pixelArtDefinition);
// ponytail: cache the small local roster; add an LRU only if workspaces reach thousands of identities.
const sourceCache = new Map<string, string>();

export function pixelAvatarSource(seed: string): string {
  const cached = sourceCache.get(seed);
  if (cached) return cached;
  const source = new DiceBearAvatar(pixelArtStyle, { seed }).toDataUri();
  sourceCache.set(seed, source);
  return source;
}

export function agentAvatarSeed(agentId: string, avatarUrl?: string | null): string {
  if (avatarUrl?.startsWith(PIXEL_ART_AVATAR_PREFIX)) {
    const seed = avatarUrl.slice(PIXEL_ART_AVATAR_PREFIX.length).trim();
    if (seed) return seed;
  }
  return `solo-agent-${agentId}`;
}

interface PixelAvatarProps {
  agentId: string;
  avatarUrl?: string | null;
  size?: 'sm' | 'md';
  className?: string;
  onClick?: () => void;
  ariaLabel?: string;
}

export function PixelAvatar({
  agentId,
  avatarUrl,
  size = 'sm',
  className,
  onClick,
  ariaLabel,
}: PixelAvatarProps) {
  const source = useMemo(
    () => pixelAvatarSource(agentAvatarSeed(agentId, avatarUrl)),
    [agentId, avatarUrl],
  );
  const classes = cn(
    'flex flex-shrink-0 items-center justify-center overflow-hidden border-2 border-black bg-white p-0 shadow-brutal-sm',
    onClick && 'cursor-pointer hover:bg-brutal-primary-light',
    size === 'sm' ? 'pixel-avatar-sm' : 'pixel-avatar-md',
    className,
  );
  const image = (
    // DiceBear renders locally as a data URI.
    // eslint-disable-next-line @next/next/no-img-element
    <img src={source} alt="" className="h-full w-full object-cover [image-rendering:pixelated]" />
  );

  if (onClick) {
    return (
      <button
        type="button"
        onClick={(event) => {
          event.stopPropagation();
          onClick();
        }}
        className={classes}
        aria-label={ariaLabel ?? 'Open Agent'}
      >
        {image}
      </button>
    );
  }

  return (
    <span className={classes} aria-label={ariaLabel}>
      {image}
    </span>
  );
}
