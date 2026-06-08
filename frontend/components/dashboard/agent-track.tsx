'use client';

import { useEffect, useRef } from 'react';
import { X } from 'lucide-react';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { AgentChunkItem } from './agent-chunk';
import type { AgentChunk } from '@/lib/hooks/use-agent-chunks';
import { cn } from '@/lib/utils';
import { t } from '@/lib/i18n';

interface AgentTrackProps {
  agentId: string;
  agentName: string;
  chunks: AgentChunk[];
  isActive: boolean;
  onClear: (agentId: string) => void;
}

export function AgentTrack({ agentId, agentName, chunks, isActive, onClear }: AgentTrackProps) {
  const scrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = scrollRef.current;
    if (!el || !isActive) return;
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 50;
    if (atBottom) {
      el.scrollTop = el.scrollHeight;
    }
  }, [chunks, isActive]);

  return (
    <div className={cn(
      'agent-track border-b-2 border-brutal-muted pb-2',
      !isActive && 'opacity-60',
    )}>
      <div className="flex items-center justify-between px-2 py-1.5 bg-brutal-violet-light border-b-2 border-black">
        <div className="flex items-center gap-1.5 min-w-0">
          <PixelAvatar agentId={agentId} size="sm" />
          <span className="font-heading text-[11px] font-bold truncate">{agentName}</span>
        </div>
        <div className="flex items-center gap-1 flex-shrink-0">
          <span className="font-mono text-[10px] text-muted-foreground">{chunks.length} chunks</span>
          <button
            type="button"
            onClick={() => onClear(agentId)}
            className="p-0.5 hover:bg-black/10 rounded-none"
            aria-label={`Clear ${agentName} chunks`}
          >
            <X className="h-3 w-3 text-muted-foreground" />
          </button>
        </div>
      </div>

      <div ref={scrollRef} className="max-h-64 overflow-y-auto px-1 py-1 space-y-0.5 font-mono text-[11px]">
        {chunks.length === 0 && (
          <p className="text-muted-foreground italic text-[10px] px-2 py-2">{t('agentWaiting')}</p>
        )}
        {chunks.map((chunk, i) => (
          <AgentChunkItem key={`${chunk.agentId}-${i}-${chunk.chunkType}`} chunk={chunk} />
        ))}
      </div>
    </div>
  );
}
