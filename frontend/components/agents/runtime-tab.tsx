// ============================================================================
// RuntimeTab — display/edit Agent runtime configuration
// - Shows: temperature, max_tokens
// - Model is managed by CLI local configuration, not selected here.
// - Edit mode with input-brutal fields
// - Save/Cancel with btn-brutal
// - All neubrutalism style, zero rounding
// ============================================================================

'use client';

import { useState } from 'react';
import { Bot, Pencil, X, Check } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { AgentModelProvider } from '@/lib/types';
import type { Agent } from '@/lib/types';

interface RuntimeTabProps {
  agent: Agent;
  onSave: (updates: {
    model_provider: AgentModelProvider;
    model_name: string;
    temperature: number;
    max_tokens: number;
  }) => Promise<void>;
  isSaving?: boolean;
}

export function RuntimeTab({ agent, onSave, isSaving = false }: RuntimeTabProps) {
  const [editing, setEditing] = useState(false);

  const [temperature, setTemperature] = useState(agent.temperature);
  const [maxTokens, setMaxTokens] = useState(agent.max_tokens);

  const handleSave = async () => {
    try {
      await onSave({
        model_provider: agent.model_provider,
        model_name: agent.model_name,
        temperature,
        max_tokens: maxTokens,
      });
      setEditing(false);
    } catch {
      // Error handled by parent
    }
  };

  const handleCancel = () => {
    setTemperature(agent.temperature);
    setMaxTokens(agent.max_tokens);
    setEditing(false);
  };

  return (
    <div className="space-y-6">
      {/* Model info — managed by CLI config */}
      <div className="space-y-3">
        <h3 className="font-heading font-bold text-sm text-muted-foreground uppercase tracking-wider">
          模型
        </h3>
        <p className="font-mono text-sm text-muted-foreground">
          Model is configured in the CLI's local settings.
        </p>
      </div>

      <hr className="divider-brutal" />

      {/* Temperature */}
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <h3 className="font-heading font-bold text-sm text-muted-foreground uppercase tracking-wider">
            Temperature
          </h3>
          <span className="font-mono text-xs text-muted-foreground">
            {editing ? temperature.toFixed(1) : agent.temperature.toFixed(1)}
          </span>
        </div>
        {editing ? (
          <input
            type="range"
            min="0"
            max="2"
            step="0.1"
            value={temperature}
            onChange={(e) => setTemperature(parseFloat(e.target.value))}
            className="w-full cursor-pointer accent-brutal-pink"
            aria-label="Temperature"
          />
        ) : (
          <div className="h-2 w-full border-2 border-black bg-black/5">
            <div
              className="h-full bg-brutal-pink"
              style={{ width: `${(agent.temperature / 2) * 100}%` }}
            />
          </div>
        )}
        <p className="font-mono text-[11px] text-muted-foreground">
          控制输出随机性。较低值更确定，较高值更多样。
        </p>
      </div>

      <hr className="divider-brutal" />

      {/* Max Tokens */}
      <div className="space-y-2">
        <h3 className="font-heading font-bold text-sm text-muted-foreground uppercase tracking-wider">
          Max Tokens
        </h3>
        {editing ? (
          <input
            type="number"
            value={maxTokens}
            onChange={(e) => setMaxTokens(parseInt(e.target.value, 10) || 0)}
            min={1}
            max={128000}
            className="input-brutal h-10 font-mono text-sm"
            aria-label="Max Tokens"
          />
        ) : (
          <p className="font-mono text-sm text-foreground">
            {agent.max_tokens.toLocaleString()}
          </p>
        )}
        <p className="font-mono text-[11px] text-muted-foreground">
          单次响应的最大 token 数。
        </p>
      </div>

      {/* Edit/Save buttons */}
      <div className="flex items-center gap-2 pt-2">
        {editing ? (
          <>
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
                  <Check className="mr-1.5 h-4 w-4" />
                  保存
                </>
              )}
            </button>
            <button
              type="button"
              onClick={handleCancel}
              disabled={isSaving}
              className="btn-flat"
            >
              <X className="mr-1.5 h-4 w-4" />
              取消
            </button>
          </>
        ) : (
          <button
            type="button"
            onClick={() => setEditing(true)}
            className="btn-brutal btn-brutal-sm"
          >
            <Pencil className="mr-1.5 h-4 w-4" />
            编辑
          </button>
        )}
      </div>
    </div>
  );
}
