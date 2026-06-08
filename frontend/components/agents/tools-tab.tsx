// ============================================================================
// SOLO-119-F: ToolsTab — Agent tools configuration tab
// - Toggle list of available file tools (read_file / write_file / list_files / search_files)
// - Each tool has a brute toggle switch and short description
// - Enabled tools show green badge, disabled show gray badge
// - Save button persists tool config
// - All neubrutalism, zero rounding
// ============================================================================

'use client';

import { useState } from 'react';
import { Save } from 'lucide-react';
import { cn } from '@/lib/utils';
import { t } from '@/lib/i18n';
import { AVAILABLE_TOOLS } from '@/lib/types';
import type { Agent } from '@/lib/types';

interface ToolsTabProps {
  agent: Agent;
  onSave: (enabledTools: string[]) => Promise<void>;
  isSaving?: boolean;
}

export function ToolsTab({ agent, onSave, isSaving = false }: ToolsTabProps) {
  const [enabledTools, setEnabledTools] = useState<string[]>(
    agent.enabled_tools ?? [],
  );
  const [hasChanges, setHasChanges] = useState(false);

  const toggleTool = (toolId: string) => {
    setEnabledTools((prev) => {
      const next = prev.includes(toolId)
        ? prev.filter((id) => id !== toolId)
        : [...prev, toolId];
      setHasChanges(true);
      return next;
    });
  };

  const handleSave = async () => {
    if (!hasChanges) return;
    try {
      await onSave(enabledTools);
      setHasChanges(false);
    } catch {
      // Error handled by parent
    }
  };

  return (
    <div className="space-y-5">
      {/* Header */}
      <div>
        <h3 className="font-heading font-bold text-sm text-muted-foreground uppercase tracking-wider">
          {t('agentToolsAvailable')}
        </h3>
        <p className="mt-1 font-mono text-[11px] text-muted-foreground">
          {t('agentToolsHelp')}
        </p>
      </div>

      {/* Tool list */}
      <div className="divide-y-2 divide-black border-2 border-black shadow-brutal-sm">
        {AVAILABLE_TOOLS.map((tool) => {
          const isEnabled = enabledTools.includes(tool.id);
          return (
            <div
              key={tool.id}
              className="flex items-center gap-3 px-4 py-3"
            >
              {/* Neubrutalist toggle switch */}
              <button
                type="button"
                onClick={() => toggleTool(tool.id)}
                className={cn(
                  'relative flex h-7 w-11 flex-shrink-0 items-center border-2 border-black transition-colors',
                  isEnabled ? 'bg-brutal-success' : 'bg-brutal-muted',
                )}
                role="switch"
                aria-checked={isEnabled}
                aria-label={t(isEnabled ? 'agentSkillsDisable' : 'agentSkillsEnable', { tool: tool.name })}
              >
                <span
                  className={cn(
                    'absolute h-7 w-[18px] border-r-2 border-l-2 border-black bg-white transition-all',
                    isEnabled ? 'left-[calc(100%-18px)]' : 'left-0',
                  )}
                />
              </button>

              {/* Tool info */}
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="font-heading text-sm font-bold text-foreground">
                    {tool.name}
                  </span>
                  <span
                    className={cn(
                      'badge-brutal text-[10px] px-1.5',
                      isEnabled
                        ? 'bg-brutal-success text-black'
                        : 'bg-brutal-muted text-white',
                    )}
                  >
                    {isEnabled ? t('agentToolsEnabled') : t('agentToolsNotEnabled')}
                  </span>
                </div>
                <p className="mt-0.5 font-mono text-[11px] text-muted-foreground leading-relaxed">
                  {tool.description}
                </p>
              </div>
            </div>
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
              {t('agentToolsSaving')}
            </>
          ) : (
            <>
              <Save className="mr-1.5 h-4 w-4" />
              {t('agentToolsSave')}
            </>
          )}
        </button>
      )}
    </div>
  );
}
