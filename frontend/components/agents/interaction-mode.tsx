// ============================================================================
// SOLO-120-F: InteractionMode — Agent interaction mode selector
// - Three modes: 主动参与 (active) / 仅 @提及 (mention) / 免打搅 (dnd)
// - Brutalist radio button group matching agent-form.tsx style
// - Saves on change with btn-brutal confirmation
// ============================================================================

'use client';

import { useState } from 'react';
import { MessageCircle, AtSign, BellOff, Save } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { Agent, AgentInteractionMode } from '@/lib/types';

interface InteractionModeProps {
  agent: Agent;
  onSave: (mode: AgentInteractionMode) => Promise<void>;
  isSaving?: boolean;
}

interface ModeOption {
  key: AgentInteractionMode;
  label: string;
  description: string;
  icon: typeof MessageCircle;
}

const MODE_OPTIONS: ModeOption[] = [
  {
    key: 'active',
    label: '主动参与',
    description: 'Agent 自动响应频道消息',
    icon: MessageCircle,
  },
  {
    key: 'mention',
    label: '仅 @提及',
    description: '只有被 @ 时才响应',
    icon: AtSign,
  },
  {
    key: 'dnd',
    label: '免打搅',
    description: '不响应任何消息',
    icon: BellOff,
  },
];

export function InteractionMode({
  agent,
  onSave,
  isSaving = false,
}: InteractionModeProps) {
  const [mode, setMode] = useState<AgentInteractionMode>(
    agent.interaction_mode ?? 'mention',
  );
  const [hasChanges, setHasChanges] = useState(false);

  const handleModeChange = (newMode: AgentInteractionMode) => {
    if (newMode === mode) return;
    setMode(newMode);
    setHasChanges(true);
  };

  const handleSave = async () => {
    if (!hasChanges) return;
    try {
      await onSave(mode);
      setHasChanges(false);
    } catch {
      // Error handled by parent
    }
  };

  return (
    <div className="space-y-3">
      <h3 className="font-heading font-bold text-sm text-muted-foreground uppercase tracking-wider">
        交互模式
      </h3>
      <p className="font-mono text-[11px] text-muted-foreground">
        控制 Agent 何时响应消息。
      </p>

      {/* Brutalist radio group — matches agent-form.tsx pattern */}
      <div className="space-y-2">
        {MODE_OPTIONS.map((option) => {
          const Icon = option.icon;
          const isSelected = mode === option.key;
          return (
            <button
              key={option.key}
              type="button"
              onClick={() => handleModeChange(option.key)}
              className={cn(
                'flex w-full items-center gap-3 border-2 px-4 py-3 text-left font-heading text-sm font-bold transition-all',
                isSelected
                  ? 'border-black bg-brutal-pink text-black shadow-brutal-sm'
                  : 'border-black bg-white text-muted-foreground shadow-brutal-sm hover:bg-black/5',
              )}
              aria-pressed={isSelected}
              aria-label={option.label}
            >
              <Icon
                className={cn(
                  'h-5 w-5 flex-shrink-0',
                  isSelected ? 'text-black' : 'text-muted-foreground',
                )}
              />
              <div className="min-w-0 flex-1">
                <span
                  className={cn(
                    'block',
                    isSelected ? 'text-black' : 'text-foreground',
                  )}
                >
                  {option.label}
                </span>
                <span className="mt-0.5 block font-mono text-[11px] font-bold text-muted-foreground">
                  {option.description}
                </span>
              </div>
            </button>
          );
        })}
      </div>

      {/* Save button — only shows when there are changes */}
      {hasChanges && (
        <button
          type="button"
          onClick={handleSave}
          disabled={isSaving}
          className="btn-brutal btn-brutal-sm"
        >
          {isSaving ? (
            <>
              <div className="mr-2 h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent" />
              保存中...
            </>
          ) : (
            <>
              <Save className="mr-1.5 h-4 w-4" />
              保存模式
            </>
          )}
        </button>
      )}
    </div>
  );
}
