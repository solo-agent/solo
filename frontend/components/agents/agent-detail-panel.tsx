// ============================================================================
// AgentDetailPanel — Tab container for agent detail view (v1.5)
// - Slides in from right as a side panel
// - 4 tabs: Profile, Runtime, Skills, Workspace
// - Close button to dismiss
// ============================================================================

'use client';

import { useState } from 'react';
import { X, User, Settings, Puzzle, FolderOpen, ArrowLeft } from 'lucide-react';
import { cn } from '@/lib/utils';
import { AgentProfileTab } from './agent-profile-tab';
import { AgentRuntimeTab } from './agent-runtime-tab';
import { AgentSkillsTab } from './agent-skills-tab';
import { AgentWorkspaceTab } from './agent-workspace-tab';
import { Skeleton } from '@/components/ui/skeleton';
import type { Agent } from '@/lib/types';

type TabKey = 'profile' | 'runtime' | 'skills' | 'workspace';

interface TabDef {
  key: TabKey;
  label: string;
  icon: typeof User;
}

const TABS: TabDef[] = [
  { key: 'profile', label: 'Profile', icon: User },
  { key: 'runtime', label: 'Runtime', icon: Settings },
  { key: 'skills', label: 'Skills', icon: Puzzle },
  { key: 'workspace', label: 'Workspace', icon: FolderOpen },
];

interface AgentDetailPanelProps {
  agentId: string;
  onClose: () => void;
}

export function AgentDetailPanel({ agentId, onClose }: AgentDetailPanelProps) {
  const [activeTab, setActiveTab] = useState<TabKey>('profile');
  // Agent data is loaded by each tab component independently via the API

  // Close on Escape
  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Escape') {
      onClose();
    }
  };

  return (
    <div
      className="fixed inset-y-0 right-0 z-40 flex w-full max-w-lg flex-col border-l-2 border-black bg-white shadow-brutal-xl"
      role="dialog"
      aria-modal="true"
      aria-label="Agent 详情面板"
      onKeyDown={handleKeyDown}
    >
      {/* Header */}
      <div className="flex items-center justify-between border-b-2 border-black px-4 py-3">
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={onClose}
            className="flex h-8 w-8 items-center justify-center border-2 border-black bg-white shadow-brutal-sm hover:shadow-brutal transition-all"
            aria-label="关闭 Agent 详情面板"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <h2 className="font-heading text-base font-bold text-foreground">
            Agent 详情
          </h2>
        </div>
        <button
          type="button"
          onClick={onClose}
          className="flex h-8 w-8 items-center justify-center border-2 border-black bg-brutal-pink shadow-brutal-sm hover:shadow-brutal transition-all"
          aria-label="关闭"
        >
          <X className="h-4 w-4" />
        </button>
      </div>

      {/* Tab bar */}
      <div className="flex border-b-2 border-black">
        {TABS.map((tab) => {
          const Icon = tab.icon;
          const isActive = activeTab === tab.key;
          return (
            <button
              key={tab.key}
              type="button"
              onClick={() => setActiveTab(tab.key)}
              className={cn(
                'flex flex-1 items-center justify-center gap-1.5 border-r-2 border-black px-2 py-2.5 font-heading text-xs font-bold transition-colors last:border-r-0',
                isActive
                  ? 'bg-brutal-pink text-black'
                  : 'bg-white text-muted-foreground hover:bg-muted',
              )}
              role="tab"
              aria-selected={isActive}
            >
              <Icon className="h-3.5 w-3.5" />
              {tab.label}
            </button>
          );
        })}
      </div>

      {/* Tab content */}
      <div className="flex-1 overflow-y-auto p-4">
        {activeTab === 'profile' && <AgentProfileTab agentId={agentId} />}
        {activeTab === 'runtime' && <AgentRuntimeTab agentId={agentId} />}
        {activeTab === 'skills' && <AgentSkillsTab agentId={agentId} />}
        {activeTab === 'workspace' && <AgentWorkspaceTab agentId={agentId} />}
      </div>
    </div>
  );
}
