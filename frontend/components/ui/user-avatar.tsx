'use client';

import { useEffect, useMemo, useState } from 'react';
import {
  PIXEL_ART_AVATAR_PREFIX,
  pixelAvatarSource,
} from '@/components/ui/pixel-avatar';
import { resolveAttachmentUrl } from '@/lib/attachment-url';
import { cn } from '@/lib/utils';

export const USER_AVATAR_PRESET_COUNT = 12;
export const USER_AVATAR_PRESET_PREFIX = PIXEL_ART_AVATAR_PREFIX;

const presetSources = Array.from({ length: USER_AVATAR_PRESET_COUNT }, (_, index) => (
  pixelAvatarSource(`solo-user-${index}`)
));

function stablePresetIndex(userId: string): number {
  let hash = 0;
  for (let index = 0; index < userId.length; index += 1) {
    hash = ((hash << 5) - hash) + userId.charCodeAt(index);
    hash |= 0;
  }
  return Math.abs(hash) % USER_AVATAR_PRESET_COUNT;
}

export function userAvatarPresetValue(index: number): string {
  return `${USER_AVATAR_PRESET_PREFIX}${index}`;
}

export function userAvatarPresetIndex(userId: string, avatarUrl?: string | null): number {
  if (avatarUrl?.startsWith(USER_AVATAR_PRESET_PREFIX)) {
    const index = Number.parseInt(avatarUrl.slice(USER_AVATAR_PRESET_PREFIX.length), 10);
    if (index >= 0 && index < USER_AVATAR_PRESET_COUNT) return index;
  }
  return stablePresetIndex(userId);
}

export function userAvatarPresetSource(index: number): string {
  return presetSources[index] ?? presetSources[0];
}

interface UserAvatarProps {
  userId: string;
  name: string;
  avatarUrl?: string | null;
  size?: 'sm' | 'md' | 'lg';
  className?: string;
}

export function UserAvatar({
  userId,
  name,
  avatarUrl,
  size = 'sm',
  className,
}: UserAvatarProps) {
  const [imageFailed, setImageFailed] = useState(false);
  const preset = avatarUrl?.startsWith(USER_AVATAR_PRESET_PREFIX) || !avatarUrl;
  const src = useMemo(
    () => preset
      ? userAvatarPresetSource(userAvatarPresetIndex(userId, avatarUrl))
      : resolveAttachmentUrl(avatarUrl || ''),
    [avatarUrl, preset, userId],
  );

  useEffect(() => setImageFailed(false), [src]);

  const initials = name
    .split(/\s+/)
    .filter(Boolean)
    .map((part) => part[0])
    .join('')
    .toUpperCase()
    .slice(0, 2) || '?';

  return (
    <span
      className={cn(
        'inline-flex flex-shrink-0 items-center justify-center overflow-hidden border-2 border-black bg-white shadow-brutal-sm',
        size === 'sm' && 'h-7 w-7',
        size === 'md' && 'h-8 w-8',
        size === 'lg' && 'h-20 w-20 border-[3px] shadow-brutal',
        className,
      )}
      aria-label={name}
    >
      {imageFailed || !src ? (
        <span className="font-heading text-xs font-black text-black">{initials}</span>
      ) : (
        // DiceBear renders locally as a data URI; uploaded photos use Solo's attachment URL.
        // eslint-disable-next-line @next/next/no-img-element
        <img
          src={src}
          alt=""
          className="h-full w-full object-cover"
          style={{ imageRendering: preset ? 'pixelated' : 'auto' }}
          onError={() => setImageFailed(true)}
        />
      )}
    </span>
  );
}
