// ============================================================================
// RuntimeTab — display/edit Agent runtime configuration
// - Shows: model_provider, model_name, temperature, max_tokens
// - Edit mode with input-brutal fields
// - Save/Cancel with btn-brutal
// - All neubrutalism style, zero rounding
// ============================================================================

'use client';

import { useState } from 'react';
import { Bot, Pencil, X, Check } from 'lucide-react';
import { cn } from '@/lib/utils';
import { MODEL_OPTIONS, type AgentModelProvider } from '@/lib/types';
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

const PROVIDER_COLORS: Record<string, string> = {
  anthropic: 'bg-brutal-pink text-black',
  openai: 'bg-brutal-cyan text-black',
  ollama: 'bg-brutal-pink text-black',
  local: 'bg-brutal-lavender text-black',
};

export function RuntimeTab({ agent, onSave, isSaving = false }: RuntimeTabProps) {
  const [editing, setEditing] = useState(false);

  // Editable form state
  const [modelProvider, setModelProvider] = useState<AgentModelProvider>(
    agent.model_provider,
  );
  const [modelName, setModelName] = useState(agent.model_name);
  const [temperature, setTemperature] = useState(agent.temperature);
  const [maxTokens, setMaxTokens] = useState(agent.max_tokens);

  const handleSave = async () => {
    try {
      await onSave({
        model_provider: modelProvider,
        model_name: modelName,
        temperature,
        max_tokens: maxTokens,
      });
      setEditing(false);
    } catch {
      // Error handled by parent
    }
  };

  const handleCancel = () => {
    // Reset to original values
    setModelProvider(agent.model_provider);
    setModelName(agent.model_name);
    setTemperature(agent.temperature);
    setMaxTokens(agent.max_tokens);
    setEditing(false);
  };

  const currentModels = MODEL_OPTIONS[modelProvider]?.models || [];

  return (
    <div className="space-y-6">
      {/* Provider + Model */}
      <div className="space-y-3">
        <h3 className="font-heading font-bold text-sm text-muted-foreground uppercase tracking-wider">
          模型
        </h3>

        {editing ? (
          <>
            {/* Provider selector — inline brutalist radio group */}
            <div className="flex gap-2">
              {(
                Object.entries(MODEL_OPTIONS) as [
                  AgentModelProvider,
                  (typeof MODEL_OPTIONS)['anthropic'],
                ][]
              ).map(([key, option]) => (
                <button
                  key={key}
                  type="button"
                  onClick={() => {
                    setModelProvider(key);
                    const first = MODEL_OPTIONS[key]?.models[0]?.value;
                    if (first) setModelName(first);
                  }}
                  className={cn(
                    'flex-1 border-2 border-black px-3 py-2 font-heading text-sm font-bold transition-all',
                    modelProvider === key
                      ? 'bg-brutal-pink text-black shadow-brutal-sm'
                      : 'bg-white text-muted-foreground shadow-brutal-sm hover:bg-black/5',
                  )}
                >
                  {option.label}
                </button>
              ))}
            </div>

            {/* Model selector dropdown */}
            <select
              value={modelName}
              onChange={(e) => setModelName(e.target.value)}
              className="input-brutal h-10 cursor-pointer appearance-none bg-white pr-8 font-body text-sm"
              style={{
                backgroundImage:
                  "url(\"data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 24 24' fill='none' stroke='%23000' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='m6 9 6 6 6-6'/%3E%3C/svg%3E\")",
                backgroundRepeat: 'no-repeat',
                backgroundPosition: 'right 0.75rem center',
              }}
            >
              {currentModels.map((model) => (
                <option key={model.value} value={model.value}>
                  {model.label}
                </option>
              ))}
            </select>
          </>
        ) : (
          /* Display mode */
          <div className="flex items-center gap-3">
            <span
              className={cn(
                'badge-brutal',
                PROVIDER_COLORS[agent.model_provider] ?? 'bg-white',
              )}
            >
              {MODEL_OPTIONS[agent.model_provider]?.label ??
                agent.model_provider}
            </span>
            <span className="font-mono text-sm text-foreground">
              {agent.model_name}
            </span>
          </div>
        )}
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
