// ============================================================================
// AgentCard — brutalist card display for a single Agent
// - card-brutal with large pink Bot icon
// - font-heading font-bold for agent name
// - Status indicator: lime (online) / yellow (thinking) / gray (offline)
// - Action buttons: btn-brutal-sm edit + delete
// - Click card to navigate to Agent detail page (/agents/[id])
// ============================================================================

'use client';

import { Edit, Trash2, Circle } from 'lucide-react';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { cn } from '@/lib/utils';
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import type { Agent } from '@/lib/types';

interface AgentCardProps {
  agent: Agent;
  onEdit: (agentId: string) => void;
  onDelete: (agentId: string) => void;
  onClick?: (agentId: string) => void;
}


export function AgentCard({ agent, onEdit, onDelete, onClick }: AgentCardProps) {
  // Status color: lime=online, yellow=thinking, gray=offline
  const statusColor = agent.is_active
    ? 'fill-brutal-lime text-brutal-lime'
    : 'fill-brutal-stone text-brutal-stone';
  const statusLabel = agent.is_active ? '在线' : '离线';

  const handleCardClick = () => {
    onClick?.(agent.id);
  };

  const handleEditClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    onEdit(agent.id);
  };

  const handleDeleteClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    onDelete(agent.id);
  };

  return (
    <Card
      className={cn(
        'group relative flex flex-col',
        onClick && 'cursor-pointer',
      )}
      onClick={onClick ? handleCardClick : undefined}
      role={onClick ? 'button' : undefined}
      tabIndex={onClick ? 0 : undefined}
      onKeyDown={
        onClick
          ? (e: React.KeyboardEvent) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                handleCardClick();
              }
            }
          : undefined
      }
      aria-label={onClick ? `查看 ${agent.name} 详情` : undefined}
    >
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between">
          <div className="flex items-center gap-3">
            <PixelAvatar agentId={agent.id} avatarUrl={agent.avatar_url} size="md" />
            <div className="min-w-0">
              <CardTitle className="font-heading font-bold text-base truncate">
                {agent.name}
              </CardTitle>
              <p className="font-mono text-[11px] text-muted-foreground mt-0.5">
                {agent.model_provider}
              </p>
            </div>
          </div>

          {/* Status indicator */}
          <div className="flex items-center gap-1.5">
            <Circle className={`h-3 w-3 ${statusColor}`} />
            <span className="font-mono text-[11px] text-muted-foreground">
              {statusLabel}
            </span>
          </div>
        </div>
      </CardHeader>

      <CardContent className="flex-1 pt-0">
        {/* Description */}
        {agent.description && (
          <p className="mb-2 font-body text-sm text-muted-foreground line-clamp-2">
            {agent.description}
          </p>
        )}

        {/* System prompt preview */}
        <p
          className={agent.system_prompt ? 'text-xs text-gray-400 truncate max-w-[240px]' : 'text-xs text-gray-300 italic'}
          title={agent.system_prompt ? agent.system_prompt.slice(0, 200) : undefined}
        >
          {agent.system_prompt
            ? agent.system_prompt.slice(0, 80) + (agent.system_prompt.length > 80 ? '...' : '')
            : '未设置 system prompt'}
        </p>
      </CardContent>

      {/* Hover action buttons: edit + delete */}
      <div className="absolute right-3 top-3 flex gap-1 opacity-0 transition-opacity group-hover:opacity-100">
        <button
          type="button"
          onClick={handleEditClick}
          className="btn-brutal btn-brutal-sm flex h-7 w-7 items-center justify-center p-0"
          aria-label={`编辑 ${agent.name}`}
          title="编辑"
        >
          <Edit className="h-3.5 w-3.5" />
        </button>
        <button
          type="button"
          onClick={handleDeleteClick}
          className="btn-brutal btn-brutal-sm flex h-7 w-7 items-center justify-center p-0 bg-brutal-red-light"
          aria-label={`删除 ${agent.name}`}
          title="删除"
        >
          <Trash2 className="h-3.5 w-3.5 text-brutal-red" />
        </button>
      </div>
    </Card>
  );
}
