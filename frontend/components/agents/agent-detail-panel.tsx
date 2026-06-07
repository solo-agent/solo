// ============================================================================
// AgentDetailPanel — Tab container for agent detail view (v1.5)
// - Slides in from right as a side panel
// - 4 tabs: Profile, Runtime, Skills, Workspace
// - Close button to dismiss
// ============================================================================

'use client';

import { useState } from 'react';
import { X, User, Settings, Puzzle, FolderOpen, ArrowLeft } from 'lucide-react';
import { AgentProfileTab } from './agent-profile-tab';
import { AgentRuntimeTab } from './agent-runtime-tab';
import { AgentSkillsTab } from './agent-skills-tab';
import { AgentWorkspaceTab } from './agent-workspace-tab';
import { Skeleton } from '@/components/ui/skeleton';
import { TabBar } from '@/components/ui/tab-bar';
import type { TabBarTab } from '@/components/ui/tab-bar';
import type { Agent } from '@/lib/types';

type TabKey = 'profile' | 'runtime' | 'skills' | 'workspace';

const TABS: TabBarTab[] = [
  { key: 'profile', label: 'Profile', icon: <User className="h-3.5 w-3.5" /> },
  { key: 'runtime', label: 'Runtime', icon: <Settings className="h-3.5 w-3.5" /> },
  { key: 'skills', label: 'Skills', icon: <Puzzle className="h-3.5 w-3.5" /> },
  { key: 'workspace', label: 'Workspace', icon: <FolderOpen className="h-3.5 w-3.5" /> },
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
          className="flex h-8 w-8 items-center justify-center border-2 border-black bg-brutal-primary shadow-brutal-sm hover:shadow-brutal transition-all"
          aria-label="关闭"
        >
          <X className="h-4 w-4" />
        </button>
      </div>

      {/* Tab bar */}
      <TabBar
        tabs={TABS}
        activeKey={activeTab}
        onChange={(key) => setActiveTab(key as TabKey)}
        variant="segment"
      />

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
