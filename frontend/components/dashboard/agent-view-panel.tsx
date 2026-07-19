'use client';

import { useEffect, useRef, useState } from 'react';
import { Eye, EyeOff, X } from 'lucide-react';
import { useAgentChunks } from '@/lib/hooks/use-agent-chunks';
import { AgentTrack } from './agent-track';
import { cn } from '@/lib/utils';
import { t } from '@/lib/i18n';
import { motionScrollBehavior } from '@/lib/motion';

interface AgentViewPanelProps {
  channelId: string | null;
  width: number;
  onWidthChange: (width: number) => void;
  /**
   * SOLO-island PR3 — when set, the panel scrolls the matching agent's
   * track into view and highlights it briefly. Cleared when the user
   * closes the panel.
   */
  focusedAgentId?: string | null;
  /**
   * Called when the user clicks the close button. Parent should set
   * visibility to false and clear focusedAgentId.
   */
  onClose?: () => void;
}

export function AgentViewPanel({
  channelId,
  width,
  onWidthChange,
  focusedAgentId,
  onClose,
}: AgentViewPanelProps) {
  const { agentTracks, activeAgentIds, clearAgentChunks, clearAllChunks } = useAgentChunks(channelId);
  const isDragging = useRef(false);
  // Local highlight flag — toggled to true when focusedAgentId changes,
  // auto-clears after a short visual flash so the user can see which row
  // the island summoned them to.
  const [highlightedId, setHighlightedId] = useState<string | null>(null);
  const trackRefs = useRef<Map<string, HTMLDivElement>>(new Map());

  useEffect(() => {
    return () => {
      // cleanup if component unmounts mid-drag
      isDragging.current = false;
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
    };
  }, []);

  // SOLO-island PR3: scroll + highlight the focused agent when it
  // changes (driven by an island click). Clears the highlight after a
  // brief flash so the user gets clear visual feedback without
  // permanently styling the row.
  useEffect(() => {
    if (!focusedAgentId) return;
    const el = trackRefs.current.get(focusedAgentId);
    if (el) {
      el.scrollIntoView({ behavior: motionScrollBehavior(), block: 'center' });
    }
    setHighlightedId(focusedAgentId);
    const timer = setTimeout(() => {
      setHighlightedId(null);
    }, 1500);
    return () => clearTimeout(timer);
  }, [focusedAgentId]);

  const hasContent = agentTracks.size > 0;

  return (
    <div
      className="flex-shrink-0 bg-brutal-cream border-l-2 border-black overflow-hidden relative flex flex-col"
      style={{ width }}
    >
      {/* Resize handle (left edge) */}
      <div
        className="absolute left-0 top-0 bottom-0 w-1.5 cursor-col-resize hover:bg-brutal-primary/50 transition-colors z-10"
        onMouseDown={(e) => {
          e.preventDefault();
          const startX = e.clientX;
          const startWidth = width;
          const onMove = (ev: MouseEvent) => {
            const newWidth = Math.max(280, Math.min(600, startWidth + ev.clientX - startX));
            onWidthChange(newWidth);
          };
          const onUp = () => {
            document.removeEventListener('mousemove', onMove);
            document.removeEventListener('mouseup', onUp);
          };
          document.addEventListener('mousemove', onMove);
          document.addEventListener('mouseup', onUp);
        }}
      />

      {/* Header */}
      <div className="flex items-center justify-between px-3 py-2 border-b-2 border-black bg-white flex-shrink-0">
        <div className="flex items-center gap-2">
          <Eye className="h-4 w-4 text-brutal-primary" />
          <span className="font-heading text-sm font-bold">{t('agentView')}</span>
          {activeAgentIds.length > 0 && (
            <span className="inline-flex items-center justify-center h-5 min-w-[20px] px-1.5 font-mono text-[10px] font-bold bg-brutal-primary text-black border-2 border-black shadow-brutal-sm">
              {activeAgentIds.length}
            </span>
          )}
        </div>
        <div className="flex items-center gap-1">
          {hasContent && (
            <button
              type="button"
              onClick={clearAllChunks}
              className="font-mono text-[10px] text-muted-foreground hover:text-foreground"
            >
              {t('agentClearAll')}
            </button>
          )}
          {onClose && (
            <button
              type="button"
              onClick={onClose}
              className="flex h-6 w-6 items-center justify-center border-2 border-black bg-brutal-primary text-white hover:bg-brutal-primary/80"
              aria-label={t('agentDetailPanelClose')}
              title={t('close')}
            >
              <X className="h-3 w-3" />
            </button>
          )}
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto">
        {!hasContent && (
          <div className="flex flex-col items-center justify-center h-full text-center px-4 py-8">
            <EyeOff className="h-8 w-8 text-muted-foreground/30 mb-2" />
            <p className="font-mono text-xs text-muted-foreground">{t('agentNoActive')}</p>
            <p className="font-mono text-[10px] text-muted-foreground/50 mt-1">
              {t('agentPanelEmptyDesc')}
            </p>
          </div>
        )}
        {Array.from(agentTracks.entries()).map(([agentId, chunks]) => (
          <div
            key={agentId}
            ref={(el) => {
              if (el) trackRefs.current.set(agentId, el);
              else trackRefs.current.delete(agentId);
            }}
            className={cn(
              'transition-colors duration-700',
              highlightedId === agentId && 'bg-brutal-primary/15',
            )}
          >
            <AgentTrack
              agentId={agentId}
              agentName={chunks[0]?.agentName || agentId.slice(0, 8)}
              chunks={chunks}
              isActive={activeAgentIds.includes(agentId)}
              onClear={clearAgentChunks}
            />
          </div>
        ))}
      </div>
    </div>
  );
}
