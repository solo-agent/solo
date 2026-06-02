'use client';

import { useEffect, useRef } from 'react';
import { Eye, EyeOff } from 'lucide-react';
import { useAgentChunks } from '@/lib/hooks/use-agent-chunks';
import { AgentTrack } from './agent-track';

interface AgentViewPanelProps {
  channelId: string | null;
  visible: boolean;
  width: number;
  onWidthChange: (width: number) => void;
}

export function AgentViewPanel({ channelId, visible, width, onWidthChange }: AgentViewPanelProps) {
  const { agentTracks, activeAgentIds, clearAgentChunks, clearAllChunks } = useAgentChunks(channelId);
  const isDragging = useRef(false);

  useEffect(() => {
    return () => {
      // cleanup if component unmounts mid-drag
      isDragging.current = false;
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
    };
  }, []);

  if (!visible) return null;

  const hasContent = agentTracks.size > 0;

  return (
    <div
      className="flex-shrink-0 bg-brutal-cream border-l-2 border-black overflow-hidden relative flex flex-col"
      style={{ width }}
    >
      {/* Resize handle (left edge) */}
      <div
        className="absolute left-0 top-0 bottom-0 w-1.5 cursor-col-resize hover:bg-brutal-pink/50 transition-colors z-10"
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
          <Eye className="h-4 w-4 text-brutal-pink" />
          <span className="font-heading text-sm font-bold">Agent View</span>
          {activeAgentIds.length > 0 && (
            <span className="inline-flex items-center justify-center h-5 min-w-[20px] px-1.5 font-mono text-[10px] font-bold bg-brutal-pink text-white rounded-full">
              {activeAgentIds.length}
            </span>
          )}
        </div>
        {hasContent && (
          <button
            type="button"
            onClick={clearAllChunks}
            className="font-mono text-[10px] text-muted-foreground hover:text-foreground"
          >
            清除全部
          </button>
        )}
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto">
        {!hasContent && (
          <div className="flex flex-col items-center justify-center h-full text-center px-4 py-8">
            <EyeOff className="h-8 w-8 text-muted-foreground/30 mb-2" />
            <p className="font-mono text-xs text-muted-foreground">暂无 agent 执行中</p>
            <p className="font-mono text-[10px] text-muted-foreground/50 mt-1">
              Agent 执行时思考过程会出现在这里
            </p>
          </div>
        )}
        {Array.from(agentTracks.entries()).map(([agentId, chunks]) => (
          <AgentTrack
            key={agentId}
            agentId={agentId}
            agentName={chunks[0]?.agentName || agentId.slice(0, 8)}
            chunks={chunks}
            isActive={activeAgentIds.includes(agentId)}
            onClear={clearAgentChunks}
          />
        ))}
      </div>
    </div>
  );
}
