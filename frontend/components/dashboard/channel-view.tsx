// ============================================================================
// ChannelView — main message area + right-side member list with Agent support
// ============================================================================

'use client';

import { useState, useEffect, useRef, useCallback, useMemo, lazy, Suspense, type CSSProperties, type ReactNode } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { ArrowLeft, Users, Loader2, MessageSquare, Plus, X, Target, ListChecks, Network, GitBranch, KanbanSquare, RefreshCw, Send } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useMessages, type MessageScopeOptions } from '@/lib/hooks/use-messages';
import { useChannelMembers } from '@/lib/hooks/use-channel-members';
import { useChannelWorkspace } from '@/lib/hooks/use-channel-workspace';
import { useThoughts } from '@/lib/hooks/use-thoughts';
import { buildDashboardHref, parseDashboardParams, type DashboardPanel, type DashboardUrlState, type DashboardView } from '@/lib/dashboard-url';
import { useWebSocket } from '@/lib/ws-context';
import { useTasks } from '@/lib/hooks/use-tasks';
import { TaskArtifactStillPendingError, useTaskArtifact } from '@/lib/hooks/use-task-artifact';
import { getTaskArtifactAction } from '@/lib/utils/task-artifact';
import { apiClient } from '@/lib/api-client';
import { displayAgentErrorReason } from '@/lib/agent-activity';
import { MessageList } from './message-list';
import { MessageInput } from './message-input';
import { MemberList } from './member-list';
import { AddAgentModal } from './add-agent-modal';
import { TaskBoard } from '@/components/tasks/task-board';
import { RelationshipDetailPanel } from '@/components/relationships/relationship-detail-panel';
import { RelationshipWorkspace } from '@/components/relationships/relationship-workspace';
import { Button } from '@/components/ui/button';
import { tabButtonClass } from '@/components/ui/tab-bar';
import {
  Dialog,
  DialogHeader,
  DialogTitle,
  DialogCloseButton,
} from '@/components/ui/dialog';
import { useToast } from '@/components/ui/toast';
import { WizardCard } from '@/components/onboarding/wizard-card';
import { t } from '@/lib/i18n';
import type { AgentDetailTarget, AgentRelationship, Channel, ChannelAgendaItem, ChannelContext, ChannelMember, ContextRecord, Message, Task, TaskArtifact, TaskStatus, TeamSurface, ThoughtNode, ThoughtSession } from '@/lib/types';

type ArtifactPreview = TaskArtifact & { previewUrl: string };
type WorkspaceDetail = {
  relationship: AgentRelationship | null;
  agent: (AgentDetailTarget & { isActive?: boolean }) | null;
};
type WorkspaceScope = 'channel' | 'thought' | 'task';
type LeftTab = 'conversation' | 'summary' | 'insight' | 'artifact';
type WorkspacePanelTab = 'overview' | 'team' | 'thought' | 'task';
type RightMode = 'map' | 'board' | 'graph';

const SCOPE_LEFT_TABS: Record<WorkspaceScope, Array<{ key: LeftTab; label: string }>> = {
  channel: [
    { key: 'conversation', label: '对话' },
    { key: 'summary', label: '摘要' },
  ],
  thought: [
    { key: 'conversation', label: '对话' },
    { key: 'summary', label: '摘要' },
    { key: 'insight', label: '洞察' },
    { key: 'artifact', label: '产物' },
  ],
  task: [
    { key: 'conversation', label: '对话' },
    { key: 'summary', label: '摘要' },
    { key: 'artifact', label: '产物' },
  ],
};

function defaultRightMode(panel: WorkspacePanelTab): RightMode | undefined {
  if (panel === 'thought') return 'map';
  if (panel === 'task') return 'board';
  return undefined;
}

function viewToWorkspacePanelTab(view: DashboardView): WorkspacePanelTab {
  if (view === 'team') return 'team';
  if (view.startsWith('task.')) return 'task';
  if (view.startsWith('thought.')) return 'thought';
  return 'overview';
}

function viewToRightMode(view: DashboardView): RightMode | undefined {
  if (view.endsWith('.graph')) return 'graph';
  if (view.endsWith('.board')) return 'board';
  if (view.endsWith('.map')) return 'map';
  return undefined;
}

function tabToView(tab: WorkspacePanelTab): DashboardView {
  if (tab === 'team') return 'team';
  if (tab === 'task') return 'task.board';
  if (tab === 'thought') return 'thought.map';
  return 'overview';
}

function panelToWorkspaceScope(panel: DashboardPanel, view: DashboardView): WorkspaceScope {
  if (panel === 'thought' || (panel === 'insight' && view.startsWith('thought.'))) return 'thought';
  if (panel === 'task' || (panel === 'artifact' && view.startsWith('task.'))) return 'task';
  return 'channel';
}

function panelToLeftTab(panel: DashboardPanel): LeftTab {
  if (panel === 'summary' || panel === 'insight' || panel === 'artifact') return panel;
  return 'conversation';
}

function coerceLeftTab(scope: WorkspaceScope, tab: LeftTab | null): LeftTab {
  const tabs = SCOPE_LEFT_TABS[scope].map((item) => item.key);
  return tab && tabs.includes(tab) ? tab : 'conversation';
}

function parseMessageCardPayload<T>(message: Message): T | null {
  try {
    return JSON.parse(message.content) as T;
  } catch {
    return null;
  }
}

// SOLO-63-F: Lazy-load ThreadPanel (only rendered when a thread is open)
const ThreadPanel = lazy(() =>
  import('./thread-panel').then((m) => ({ default: m.ThreadPanel })),
);

const WORKSPACE_PANEL_TABS: Array<{
  key: WorkspacePanelTab;
  label: string;
  icon: ReactNode;
}> = [
  { key: 'overview', label: 'Overview', icon: <Target className="h-3.5 w-3.5" /> },
  { key: 'team', label: 'Team', icon: <Network className="h-3.5 w-3.5" /> },
  { key: 'task', label: 'Task', icon: <KanbanSquare className="h-3.5 w-3.5" /> },
];

function ChannelWorkspacePanel({
  activeTab,
  mode,
  channelId,
  onTabChange,
  onModeChange,
  context,
  team,
  tasks,
  tasksLoading,
  tasksError,
  selectedTask,
  selectedThoughtNodeId,
  thought,
  thoughts,
  thoughtsLoading,
  thoughtsError,
  onThoughtRetry,
  onCompleteThought,
  onThoughtNodeSelect,
  onTaskSelect,
  onTaskRetry,
  onTaskActionComplete,
  onGenerateTaskArtifact,
  isTaskArtifactGenerating,
  onTeamDetailOpen,
  onTeamDetailClose,
  canAddAgents,
  onAddAgent,
  onOpenMembers,
  isLoading,
  error,
  onRetry,
}: {
  activeTab: WorkspacePanelTab;
  mode?: RightMode;
  channelId: string;
  onTabChange: (tab: WorkspacePanelTab) => void;
  onModeChange: (mode: RightMode) => void;
  context: ChannelContext | null;
  team: TeamSurface | null;
  tasks: Task[];
  tasksLoading: boolean;
  tasksError: string | null;
  selectedTask: Task | null;
  selectedThoughtNodeId?: string;
  thought: ThoughtSession | null;
  thoughts: ThoughtSession[];
  thoughtsLoading: boolean;
  thoughtsError: string | null;
  onThoughtRetry: () => void;
  onCompleteThought: (thoughtId: string) => Promise<void>;
  onThoughtNodeSelect: (nodeId: string) => void;
  onTaskSelect: (task: Task) => void;
  onTaskRetry: () => void;
  onTaskActionComplete: (task: Task) => void;
  onGenerateTaskArtifact: (task: Task) => void;
  isTaskArtifactGenerating: (task: Task) => boolean;
  onTeamDetailOpen: (detail: WorkspaceDetail) => void;
  onTeamDetailClose: () => void;
  canAddAgents: boolean;
  onAddAgent: () => void;
  onOpenMembers: () => void;
  isLoading: boolean;
  error: string | null;
  onRetry: () => void;
}) {
  return (
    <aside className="flex h-full min-w-[360px] flex-col bg-brutal-cream">
      <div className="flex h-14 flex-shrink-0 items-center justify-between border-b-2 border-black px-4">
        <div className="flex items-center gap-1">
          {WORKSPACE_PANEL_TABS.map((tab) => (
            <button
              key={tab.key}
              type="button"
              onClick={() => onTabChange(tab.key)}
              className={tabButtonClass(activeTab === tab.key)}
            >
              {tab.icon}
              {tab.label}
            </button>
          ))}
        </div>
        {isLoading && <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />}
      </div>

      <div className={cn('min-h-0 flex-1', activeTab === 'team' && !error ? 'flex overflow-hidden' : 'overflow-y-auto p-4')}>
        {error ? (
          <div className="border-2 border-black bg-brutal-danger-light p-4 shadow-brutal-sm">
            <div className="font-heading text-sm font-black">Workspace failed to load</div>
            <p className="mt-1 font-body text-sm text-muted-foreground">{error}</p>
            <Button variant="outline" size="sm" className="mt-3" onClick={onRetry}>
              <RefreshCw className="mr-2 h-3.5 w-3.5" />
              {t('retry')}
            </Button>
          </div>
        ) : activeTab === 'overview' ? (
          <OverviewPanel context={context} />
        ) : activeTab === 'team' ? (
          isLoading ? (
            <div className="flex flex-1 items-center justify-center">
              <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
            </div>
          ) : (
            <RelationshipWorkspace
              embedded
              channelFilterId={channelId}
              channelTeam={team}
              onChannelTeamRefresh={onRetry}
              onDetailOpen={onTeamDetailOpen}
              onDetailClose={onTeamDetailClose}
              embeddedActions={(
                <>
                  {canAddAgents && (
                    <Button
                      type="button"
                      onClick={onAddAgent}
                      variant="outline"
                      size="sm"
                      className="h-8 w-8 p-0"
                      aria-label={t('addAgentToChannel')}
                      title={t('addAgentToChannel')}
                    >
                      <Plus className="h-4 w-4" />
                    </Button>
                  )}
                  <Button
                    type="button"
                    onClick={onOpenMembers}
                    variant="outline"
                    size="sm"
                    className="h-8 w-8 p-0"
                    aria-label={t('channelMembers')}
                    title={t('channelMembers')}
                  >
                    <Users className="h-4 w-4" />
                  </Button>
                </>
              )}
            />
          )
        ) : activeTab === 'thought' ? (
          <ThoughtPanel
            thought={thought}
            thoughts={thoughts}
            selectedNodeId={selectedThoughtNodeId}
            mode={mode === 'board' ? 'board' : 'map'}
            onModeChange={onModeChange}
            onNodeSelect={onThoughtNodeSelect}
            isLoading={thoughtsLoading}
            error={thoughtsError}
            onRetry={onThoughtRetry}
            onComplete={onCompleteThought}
          />
        ) : (
          <TaskPanel
            tasks={tasks}
            tasksLoading={tasksLoading}
            tasksError={tasksError}
            selectedTask={selectedTask}
            mode={mode === 'board' ? 'board' : 'graph'}
            onModeChange={onModeChange}
            onTaskSelect={onTaskSelect}
            onTaskRetry={onTaskRetry}
            onTaskActionComplete={onTaskActionComplete}
            onGenerateTaskArtifact={onGenerateTaskArtifact}
            isTaskArtifactGenerating={isTaskArtifactGenerating}
          />
        )}
      </div>
    </aside>
  );
}

function OverviewPanel({ context }: { context: ChannelContext | null }) {
  const agenda = context?.agenda ?? [];
  const hasContext = Boolean(context?.target?.trim()) || agenda.length > 0;

  if (!hasContext) {
    return (
      <EmptyWorkspacePanel
        title="Channel overview"
        body="Target and agenda will appear here after Lucy creates the channel context."
      />
    );
  }

  return (
    <div className="space-y-4">
      <section className="border-2 border-black bg-white p-4 shadow-brutal-sm">
        <div className="mb-2 flex items-center gap-2 font-heading text-xs font-black uppercase tracking-wide text-muted-foreground">
          <Target className="h-4 w-4 text-brutal-info" />
          Target
        </div>
        <p className="whitespace-pre-wrap font-body text-sm leading-6 text-foreground">
          {context?.target || 'No target yet.'}
        </p>
      </section>

      <section className="border-2 border-black bg-white p-4 shadow-brutal-sm">
        <div className="mb-3 flex items-center gap-2 font-heading text-xs font-black uppercase tracking-wide text-muted-foreground">
          <ListChecks className="h-4 w-4 text-brutal-success" />
          Agenda
        </div>
        <AgendaTree items={agenda} />
      </section>

    </div>
  );
}

function AgendaTree({ items, depth = 0 }: { items: ChannelAgendaItem[]; depth?: number }) {
  if (items.length === 0) {
    return <p className="font-body text-sm text-muted-foreground">No agenda yet.</p>;
  }

  return (
    <ul className={cn('space-y-2', depth > 0 && 'mt-2 border-l-2 border-black pl-3')}>
      {items.map((item, index) => (
        <li key={item.id || `${depth}-${index}`}>
          <div className="flex items-start gap-2">
            <span className="mt-0.5 inline-flex h-5 min-w-5 items-center justify-center border-2 border-black bg-brutal-primary px-1 font-mono text-[10px] font-black shadow-brutal-sm">
              {index + 1}
            </span>
            <div className="min-w-0 flex-1">
              <div className="font-body text-sm font-bold text-foreground">
                {item.title || 'Untitled'}
              </div>
              {item.status && (
                <div className="mt-1 inline-flex border-2 border-black bg-white px-1.5 py-0.5 font-mono text-[10px] font-bold uppercase text-muted-foreground">
                  {item.status.replace('_', ' ')}
                </div>
              )}
              {item.children?.length ? <AgendaTree items={item.children} depth={depth + 1} /> : null}
            </div>
          </div>
        </li>
      ))}
    </ul>
  );
}

function WorkspaceGraphCanvas({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        'relative min-h-[520px] overflow-hidden border-2 border-black bg-brutal-cream shadow-brutal-sm',
        className,
      )}
      style={{
        backgroundImage: 'linear-gradient(rgba(0,0,0,0.08) 1px, transparent 1px), linear-gradient(90deg, rgba(0,0,0,0.08) 1px, transparent 1px)',
        backgroundSize: '32px 32px',
      }}
    >
      {children}
    </div>
  );
}

function GraphNodeButton({
  active,
  children,
  className,
  onClick,
  style,
}: {
  active?: boolean;
  children: ReactNode;
  className?: string;
  onClick: () => void;
  style?: CSSProperties;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      style={style}
      className={cn(
        'absolute z-10 flex min-h-16 min-w-28 max-w-40 flex-col items-center justify-center border-2 border-black px-3 py-2 text-center shadow-brutal-sm transition-transform hover:-translate-y-px hover:shadow-brutal',
        className,
        active ? 'bg-brutal-primary' : !className && 'bg-white',
      )}
    >
      {children}
    </button>
  );
}

function ThoughtPanel({
  thought,
  thoughts,
  selectedNodeId,
  mode,
  onModeChange,
  onNodeSelect,
  isLoading,
  error,
  onRetry,
  onComplete,
}: {
  thought: ThoughtSession | null;
  thoughts: ThoughtSession[];
  selectedNodeId?: string;
  mode: 'map' | 'board';
  onModeChange: (mode: RightMode) => void;
  onNodeSelect: (nodeId: string) => void;
  isLoading: boolean;
  error: string | null;
  onRetry: () => void;
  onComplete: (thoughtId: string) => Promise<void>;
}) {
  const root = thought?.nodes.find((node) => node.is_root);
  const children = thought?.nodes.filter((node) => node.parent_id === root?.id) ?? [];
  const [busy, setBusy] = useState(false);

  const done = async () => {
    if (!thought || thought.status === 'done' || busy) return;
    setBusy(true);
    try {
      await onComplete(thought.id);
    } finally {
      setBusy(false);
    }
  };

  if (isLoading) {
    return (
      <div className="flex min-h-[240px] items-center justify-center border-2 border-black bg-white">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="border-2 border-black bg-brutal-danger-light p-4 shadow-brutal-sm">
        <div className="font-heading text-sm font-black">Thought failed to load</div>
        <p className="mt-1 font-body text-sm text-muted-foreground">{error}</p>
        <Button variant="outline" size="sm" className="mt-3" onClick={onRetry}>
          <RefreshCw className="mr-2 h-3.5 w-3.5" />
          {t('retry')}
        </Button>
      </div>
    );
  }

  if (!thought || !root) {
    return (
      <EmptyWorkspacePanel
        title="Thought map"
        body="Thought sessions will appear here after Lucy starts exploration."
      />
    );
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <div className="truncate font-heading text-base font-black">{thought.title}</div>
          <div className="mt-1 inline-flex border-2 border-black bg-white px-2 py-0.5 font-mono text-[10px] font-bold uppercase shadow-brutal-sm">
            {thought.status.replace('_', ' ')}
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Button size="sm" variant={mode === 'board' ? 'default' : 'outline'} onClick={() => onModeChange('board')}>
            Board
          </Button>
          <Button size="sm" variant={mode === 'map' ? 'default' : 'outline'} onClick={() => onModeChange('map')}>
            Map
          </Button>
          <Button
            size="sm"
            variant="success"
            disabled={thought.status === 'done' || busy}
            onClick={done}
          >
            {busy ? <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" /> : null}
            Done
          </Button>
        </div>
      </div>

      {mode === 'board' ? (
        <ThoughtBoard thoughts={thoughts} onThoughtSelect={(item) => onNodeSelect(item.nodes.find((node) => node.is_root)?.id ?? item.id)} />
      ) : (
        <WorkspaceGraphCanvas>
          <svg className="pointer-events-none absolute inset-0 h-full w-full" viewBox="0 0 100 100" preserveAspectRatio="none">
            {children.map((node, index) => {
              const left = children.length === 1 ? 50 : 16 + (68 / Math.max(children.length - 1, 1)) * index;
              return (
                <line
                  key={node.id}
                  x1="50"
                  y1="25"
                  x2={left}
                  y2="62"
                  stroke="black"
                  strokeWidth="0.6"
                />
              );
            })}
          </svg>
          <GraphNodeButton
            active={selectedNodeId === root.id}
            onClick={() => onNodeSelect(root.id)}
            style={{ left: '50%', top: '14%', transform: 'translateX(-50%)' }}
            className="min-h-20 min-w-32 rounded-full bg-brutal-primary"
          >
            <span className="font-heading text-sm font-black">{root.title}</span>
            <span className="mt-1 font-mono text-[10px] font-bold uppercase">Root</span>
          </GraphNodeButton>
          {children.map((node, index) => {
            const left = children.length === 1 ? 50 : 16 + (68 / Math.max(children.length - 1, 1)) * index;
            return (
              <GraphNodeButton
                key={node.id}
                active={selectedNodeId === node.id}
                onClick={() => onNodeSelect(node.id)}
                style={{ left: `${left}%`, top: '58%', transform: 'translateX(-50%)' }}
                className="bg-white"
              >
                <span className="w-full truncate font-heading text-sm font-black">{node.title}</span>
                <span className="mt-1 border-2 border-black bg-brutal-info-light px-1.5 py-0.5 font-mono text-[10px] font-bold uppercase">
                  {node.status.replace('_', ' ')}
                </span>
              </GraphNodeButton>
            );
          })}
        </WorkspaceGraphCanvas>
      )}
    </div>
  );
}

function ThoughtBoard({
  thoughts,
  onThoughtSelect,
}: {
  thoughts: ThoughtSession[];
  onThoughtSelect: (thought: ThoughtSession) => void;
}) {
  const statuses: Array<Exclude<TaskStatus, 'closed'>> = ['todo', 'in_progress', 'in_review', 'done'];
  return (
    <div className="grid min-h-[320px] grid-cols-4 gap-3 border-2 border-black bg-brutal-cream p-3">
      {statuses.map((status) => (
        <div key={status} className="border-2 border-black bg-white">
          <div className="border-b-2 border-black bg-brutal-primary px-2 py-1 font-heading text-xs font-black uppercase">
            {status.replace('_', ' ')}
          </div>
          <div className="space-y-2 p-2">
            {thoughts.filter((item) => item.status === status).map((item) => (
              <button
                key={item.id}
                type="button"
                onClick={() => onThoughtSelect(item)}
                className="w-full border-2 border-black bg-brutal-cream p-2 text-left shadow-brutal-sm hover:-translate-y-px hover:shadow-brutal"
              >
                <div className="font-heading text-sm font-bold">{item.title}</div>
                <div className="mt-1 font-mono text-[10px] text-muted-foreground">
                  {item.nodes.length} nodes
                </div>
              </button>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}

function TaskPanel({
  tasks,
  tasksLoading,
  tasksError,
  selectedTask,
  mode,
  onModeChange,
  onTaskSelect,
  onTaskRetry,
  onTaskActionComplete,
  onGenerateTaskArtifact,
  isTaskArtifactGenerating,
}: {
  tasks: Task[];
  tasksLoading: boolean;
  tasksError: string | null;
  selectedTask: Task | null;
  mode: 'graph' | 'board';
  onModeChange: (mode: RightMode) => void;
  onTaskSelect: (task: Task) => void;
  onTaskRetry: () => void;
  onTaskActionComplete: (task: Task) => void;
  onGenerateTaskArtifact: (task: Task) => void;
  isTaskArtifactGenerating: (task: Task) => boolean;
}) {
  const rootTasks = tasks.filter((task) => !task.parent_task_id);
  const selectedRoot = selectedTask && !selectedTask.parent_task_id
    ? selectedTask
    : selectedTask?.parent_task_id
      ? rootTasks.find((task) => task.id === selectedTask.parent_task_id) ?? rootTasks[0] ?? null
      : rootTasks[0] ?? null;
  const childTasks = selectedRoot
    ? tasks.filter((task) => task.parent_task_id === selectedRoot.id)
    : [];

  if (mode === 'board') {
    return (
      <TaskBoard
        tasks={tasks}
        isLoading={tasksLoading}
        error={tasksError}
        onTaskClick={onTaskSelect}
        onRefetch={onTaskRetry}
        onActionComplete={onTaskActionComplete}
        onGenerateArtifact={onGenerateTaskArtifact}
        isArtifactGenerating={isTaskArtifactGenerating}
      />
    );
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <div className="font-heading text-base font-black">Task Graph</div>
        <Button size="sm" variant="outline" className="gap-1.5" onClick={() => onModeChange('board')}>
          <ArrowLeft className="h-3.5 w-3.5" />
          {t('back')}
        </Button>
      </div>
      {selectedRoot ? (
        <WorkspaceGraphCanvas>
          <svg className="pointer-events-none absolute inset-0 h-full w-full" viewBox="0 0 100 100" preserveAspectRatio="none">
            {childTasks.map((task, index) => {
              const left = childTasks.length === 1 ? 50 : 16 + (68 / Math.max(childTasks.length - 1, 1)) * index;
              return (
                <line
                  key={task.id}
                  x1="50"
                  y1="25"
                  x2={left}
                  y2="62"
                  stroke="black"
                  strokeWidth="0.6"
                />
              );
            })}
          </svg>
          <GraphNodeButton
            active={selectedTask?.id === selectedRoot.id}
            onClick={() => onTaskSelect(selectedRoot)}
            style={{ left: '50%', top: '14%', transform: 'translateX(-50%)' }}
            className="min-h-20 min-w-32 rounded-full bg-brutal-primary"
          >
            <span className="font-heading text-sm font-black">
              {selectedRoot.task_number ? `#${selectedRoot.task_number} ` : ''}{selectedRoot.title}
            </span>
            <span className="mt-1 font-mono text-[10px] font-bold uppercase">Main task</span>
          </GraphNodeButton>
          {childTasks.map((task, index) => {
            const left = childTasks.length === 1 ? 50 : 16 + (68 / Math.max(childTasks.length - 1, 1)) * index;
            return (
              <GraphNodeButton
                key={task.id}
                active={selectedTask?.id === task.id}
                onClick={() => onTaskSelect(task)}
                style={{ left: `${left}%`, top: '58%', transform: 'translateX(-50%)' }}
                className="bg-white"
              >
                <span className="w-full truncate font-heading text-sm font-black">
                  {task.task_number ? `#${task.task_number} ` : ''}{task.title}
                </span>
                <span className="mt-1 border-2 border-black bg-brutal-info-light px-1.5 py-0.5 font-mono text-[10px] font-bold uppercase">
                  {task.status.replace('_', ' ')}
                </span>
              </GraphNodeButton>
            );
          })}
        </WorkspaceGraphCanvas>
      ) : (
        <EmptyWorkspacePanel title="Task graph" body="Tasks will appear after Lucy creates work from the channel context." />
      )}
    </div>
  );
}

function RecordList({ title, records }: { title: string; records: ContextRecord[] }) {
  return (
    <section className="border-2 border-black bg-white p-4 shadow-brutal-sm">
      <div className="mb-3 font-heading text-xs font-black uppercase tracking-wide text-muted-foreground">
        {title}
      </div>
      <div className="space-y-2">
        {records.map((record) => (
          <article key={record.id} className="border-2 border-black bg-brutal-cream p-3 shadow-brutal-sm">
            <div className="flex items-center justify-between gap-2">
              <h4 className="truncate font-heading text-sm font-bold">{record.title}</h4>
              <time className="shrink-0 font-mono text-[10px] text-muted-foreground">
                {new Date(record.created_at).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })}
              </time>
            </div>
            <p className="mt-1 line-clamp-3 whitespace-pre-wrap font-body text-sm text-muted-foreground">
              {record.body}
            </p>
          </article>
        ))}
      </div>
    </section>
  );
}

function RecordTimelinePanel({
  title,
  records,
}: {
  title: string;
  records: ContextRecord[];
}) {
  return (
    <div className="flex flex-1 flex-col overflow-hidden bg-brutal-cream">
      <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4">
        <div className="mx-auto max-w-3xl space-y-3">
          {records.length === 0 ? (
            <EmptyWorkspacePanel
              title={title}
              body="这里暂时还没有记录。继续对话或完成 review 后会自动沉淀。"
            />
          ) : records.map((record) => (
            <article key={record.id} className="border-2 border-black bg-white p-4 shadow-brutal-sm">
              <div className="mb-2 flex items-center justify-between gap-3">
                <div className="min-w-0">
                  <div className="font-heading text-sm font-black">{record.title}</div>
                  <div className="mt-1 font-mono text-[10px] font-bold uppercase text-muted-foreground">
                    {record.record_type.replace('_', ' ')} · {record.author_type}
                  </div>
                </div>
                <time className="shrink-0 font-mono text-[10px] text-muted-foreground">
                  {new Date(record.created_at).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })}
                </time>
              </div>
              <p className="whitespace-pre-wrap font-body text-sm leading-6 text-foreground">
                {record.body}
              </p>
              {record.artifact_ref ? (
                <div className="mt-3 inline-flex border-2 border-black bg-brutal-primary px-2 py-1 font-mono text-[10px] font-black uppercase shadow-brutal-sm">
                  artifact
                </div>
              ) : null}
            </article>
          ))}
        </div>
      </div>
    </div>
  );
}

function ScopedConversationInput({
  placeholder,
  onSubmit,
}: {
  placeholder: string;
  onSubmit: (content: string) => void;
}) {
  const [content, setContent] = useState('');
  const trimmed = content.trim();

  return (
    <div className="flex-shrink-0 border-t-2 border-black bg-brutal-cream px-6 py-4">
      <div className="relative flex items-end gap-2">
        <textarea
          value={content}
          onChange={(event) => setContent(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter' && !event.shiftKey) {
              event.preventDefault();
              if (!trimmed) return;
              onSubmit(trimmed);
              setContent('');
            }
          }}
          placeholder={placeholder}
          rows={1}
          className="input-brutal min-h-[44px] resize-none pr-12 font-mono text-sm leading-relaxed placeholder:font-mono placeholder:text-muted-foreground/60"
        />
        <button
          type="button"
          onClick={() => {
            if (!trimmed) return;
            onSubmit(trimmed);
            setContent('');
          }}
          disabled={!trimmed}
          className={cn(
            'btn-brutal btn-brutal-success absolute bottom-2 right-2 flex h-8 w-8 items-center justify-center p-0',
            !trimmed && 'pointer-events-none opacity-40',
          )}
          aria-label="Send scoped message"
        >
          <Send className="h-4 w-4" />
        </button>
      </div>
    </div>
  );
}

function ThoughtConversationPanel({
  thought,
  node,
  messages,
  members,
  onRetry,
  onCompleteThoughtFromCard,
  onSubmit,
}: {
  thought: ThoughtSession | null;
  node: ThoughtNode | null;
  messages: Message[];
  members: ChannelMember[];
  onRetry: (messageId: string, content: string) => void;
  onCompleteThoughtFromCard: (message: Message, thoughtId: string) => Promise<void>;
  onSubmit: (content: string) => void;
}) {
  const beforeItems = thought && node ? (
    <div role="listitem" className="px-6">
      <ThoughtChatBubble
        author="Lucy"
        time={thought.created_at}
        text={node.is_root
          ? `已进入 Thought「${thought.title}」。这里是这次探索的 Root。`
          : `现在选中 ${node.title} 节点。这里的对话只围绕 ${node.title} 展开。`}
      />
      {(node.is_root ? thought.summary_records : thought.summary_records.filter((record) => record.subject_id === node.id)).map((record) => (
        <ThoughtChatBubble
          key={record.id}
          author={record.author_type === 'agent' ? 'Lucy' : 'Solo'}
          time={record.created_at}
          text={record.body}
        />
      ))}
      <div className="mt-3 flex items-center gap-2 border-2 border-black bg-white px-3 py-2 shadow-brutal-sm">
        <GitBranch className="h-4 w-4 text-brutal-info" />
        <div className="min-w-0">
          <div className="truncate font-heading text-sm font-black">{node.title}</div>
          <div className="font-mono text-[10px] font-bold uppercase text-muted-foreground">
            {node.status.replace('_', ' ')}
          </div>
        </div>
      </div>
    </div>
  ) : (
    <div role="listitem" className="px-6">
      <EmptyWorkspacePanel title="Thought conversation" body="Start a thought from the channel timeline first." />
    </div>
  );

  return (
    <div className="flex flex-1 flex-col overflow-hidden bg-brutal-cream">
      <MessageList
        messages={messages}
        beforeItems={beforeItems}
        showBeginningMarker={false}
        isLoading={false}
        error={null}
        onRetry={onRetry}
        hasMore={false}
        isLoadingMore={false}
        loadMoreError={null}
        onLoadMore={() => {}}
        members={members}
        onCompleteThoughtFromCard={onCompleteThoughtFromCard}
      />
      <ScopedConversationInput
        placeholder={node ? `输入 ${node.title} 的探索消息...` : '输入 thought 探索消息...'}
        onSubmit={onSubmit}
      />
    </div>
  );
}

function ThoughtChatBubble({ author, time, text }: { author: string; time: string; text: string }) {
  return (
    <article className="flex gap-3 border-b border-brutal-muted px-2 py-3">
      <div className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center border-2 border-black bg-brutal-primary shadow-brutal-sm">
        <span className="font-heading text-xs font-black">S</span>
      </div>
      <div className="min-w-0 flex-1">
        <div className="mb-1 flex items-baseline gap-2">
          <span className="font-heading text-sm font-black">{author}</span>
          <time className="font-mono text-[11px] text-muted-foreground">
            {new Date(time).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })}
          </time>
        </div>
        <p className="whitespace-pre-wrap break-words font-body text-sm leading-6 text-foreground">{text}</p>
      </div>
    </article>
  );
}

function TaskContextCard({ task }: { task: Task }) {
  return (
    <section className="border-2 border-black bg-white p-4 shadow-brutal-sm">
      <div className="mb-2 flex items-center justify-between gap-3">
        <div className="min-w-0">
          <div className="truncate font-heading text-sm font-black">
            {task.task_number ? `#${task.task_number} ` : ''}{task.title}
          </div>
          <div className="mt-1 inline-flex border-2 border-black bg-brutal-info-light px-2 py-0.5 font-mono text-[10px] font-bold uppercase">
            {task.status.replace('_', ' ')}
          </div>
        </div>
        {task.claimer_name || task.assignee_name ? (
          <div className="shrink-0 border-2 border-black bg-brutal-cream px-2 py-1 font-mono text-[10px] font-bold">
            {task.claimer_name || task.assignee_name}
          </div>
        ) : null}
      </div>
      {task.description ? (
        <p className="whitespace-pre-wrap font-body text-sm leading-6 text-foreground">
          {task.description}
        </p>
      ) : null}
    </section>
  );
}

function TaskConversationPanel({
  task,
  cardMessages,
  members,
  onRetry,
  onViewTaskGraph,
  onTaskReviewAction,
  onSubmitFallback,
}: {
  task: Task | null;
  parentMessage: Message | null;
  cardMessages: Message[];
  members: ChannelMember[];
  onRetry: (messageId: string, content: string) => void;
  onClose: () => void;
  onMarkRead: () => void;
  onViewInChannel: () => void;
  onViewTaskGraph: () => void;
  onTaskReviewAction: () => Promise<void> | void;
  onOpenArtifactReference: (ref: string) => void;
  onAgentClick: (agent: AgentDetailTarget) => void;
  onSubmitFallback: (content: string) => void;
}) {
  if (!task) {
    return (
      <div className="flex flex-1 flex-col overflow-hidden bg-brutal-cream">
        <MessageList
          messages={cardMessages}
          beforeItems={(
            <div role="listitem" className="px-6">
              <EmptyWorkspacePanel title="Task conversation" body="Select a task node to open its thread." />
            </div>
          )}
          showBeginningMarker={false}
          isLoading={false}
          error={null}
          onRetry={onRetry}
          hasMore={false}
          isLoadingMore={false}
          loadMoreError={null}
          onLoadMore={() => {}}
          members={members}
          onTaskReviewAction={onTaskReviewAction}
          onViewTaskGraph={onViewTaskGraph}
        />
        <ScopedConversationInput
          placeholder="选中 task 后可在 thread 中讨论..."
          onSubmit={onSubmitFallback}
        />
      </div>
    );
  }

  return (
    <div className="flex flex-1 flex-col overflow-hidden bg-brutal-cream">
      <MessageList
        messages={cardMessages}
        beforeItems={(
          <div role="listitem" className="px-6">
            <TaskContextCard task={task} />
          </div>
        )}
        showBeginningMarker={false}
        isLoading={false}
        error={null}
        onRetry={onRetry}
        hasMore={false}
        isLoadingMore={false}
        loadMoreError={null}
        onLoadMore={() => {}}
        members={members}
        onTaskReviewAction={onTaskReviewAction}
      />
      <ScopedConversationInput
        placeholder={`输入 ${task.title} 的 task 讨论...`}
        onSubmit={onSubmitFallback}
      />
    </div>
  );
}

function taskArtifactToRecord(artifact: TaskArtifact): ContextRecord {
  return {
    id: artifact.id,
    channel_id: artifact.channel_id,
    scope: 'task',
    subject_type: 'task',
    subject_id: artifact.task_id,
    record_type: 'artifact',
    title: artifact.title,
    body: artifact.summary && artifact.summary !== 'pending'
      ? artifact.summary
      : artifact.html_path,
    author_type: 'system',
    artifact_ref: {
      id: artifact.id,
      title: artifact.title,
      kind: artifact.kind,
      url: artifact.url,
    },
    created_at: artifact.created_at,
  };
}

function EmptyWorkspacePanel({ title, body }: { title: string; body: string }) {
  return (
    <div className="flex min-h-[240px] flex-col items-center justify-center border-2 border-dashed border-black bg-white p-6 text-center">
      <div className="mb-3 flex h-10 w-10 items-center justify-center border-2 border-black bg-brutal-primary shadow-brutal-sm">
        <MessageSquare className="h-5 w-5" />
      </div>
      <h3 className="font-heading text-base font-black text-foreground">{title}</h3>
      <p className="mt-2 max-w-xs font-body text-sm text-muted-foreground">{body}</p>
    </div>
  );
}


interface ChannelViewProps {
  channel: Channel;
  /** Show the onboarding wizard card above the message list */
  showOnboardingWizard?: boolean;
  /** Optional message ID to open ThreadPanel for on mount */
  initialThreadMessageId?: string;
  /** Optional message ID to scroll to on mount */
  initialScrollToMessageId?: string;
  /** Called after a card creates a channel so the sidebar can refresh. */
  onChannelCreated?: () => Promise<void> | void;
}

export function ChannelView({
  channel,
  showOnboardingWizard,
  initialThreadMessageId,
  initialScrollToMessageId,
  onChannelCreated,
}: ChannelViewProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const dashboardUrlState = useMemo(() => parseDashboardParams(searchParams), [searchParams]);
  const {
    messages,
    isLoading,
    error,
    sendMessage,
    retryMessage,
    cancelMessage,
    hasMore,
    isLoadingMore,
    loadMoreError,
    loadMore,
    refetch: refetchMessages,
    markMessageThreadRead,
  } = useMessages(channel.id);

  const {
    members,
    users,
    agents,
    isLoading: membersLoading,
    addAgentToChannel,
    removeMember,
    updateMemberStatus,
  } = useChannelMembers(channel.id);

  // Keep a ref to the latest agents list for use in WS event handler closures
  const agentsRef = useRef(agents);
  agentsRef.current = agents;

  const [threadMessage, setThreadMessage] = useState<Message | null>(null);
  const [isAddAgentModalOpen, setIsAddAgentModalOpen] = useState(false);
  const [workspaceDetail, setWorkspaceDetail] = useState<WorkspaceDetail | null>(null);
  const [mainPanel, setMainPanel] = useState<'thread' | 'detail' | 'thought' | null>(
    dashboardUrlState.panel === 'thread'
      ? 'thread'
      : dashboardUrlState.panel === 'agent' || dashboardUrlState.panel === 'relationship'
        ? 'detail'
        : dashboardUrlState.panel === 'thought'
          ? 'thought'
          : null,
  );
  const [threadTask, setThreadTask] = useState<Task | null>(null);
  const [artifactPreview, setArtifactPreview] = useState<ArtifactPreview | null>(null);
  const [artifactHistory, setArtifactHistory] = useState<TaskArtifact[]>([]);
  const [artifactReviewBusy, setArtifactReviewBusy] = useState(false);

  // ---- Channel search state (SOLO-237-F) ----
  const [scrollToMessageId, setScrollToMessageId] = useState<string | undefined>(undefined);
  const [scrollMsgKey, setScrollMsgKey] = useState(0);

  // ---- Member popover state ----
  const [isMemberPopoverOpen, setIsMemberPopoverOpen] = useState(false);

  const { showToast } = useToast();
  const { generateArtifact, regenerateArtifact, fetchArtifactHTML, listArtifacts, isGeneratingTask } = useTaskArtifact();
  const artifactOpenLinkRef = useRef<HTMLAnchorElement>(null);
  const artifactRegenerateButtonRef = useRef<HTMLButtonElement>(null);
  const artifactFrameRef = useRef<HTMLIFrameElement>(null);
  const artifactCloseButtonRef = useRef<HTMLButtonElement>(null);
  const artifactReturnFocusRef = useRef<HTMLElement | null>(null);
  const artifactPreviewUrlRef = useRef<string | null>(null);

  const closeArtifactPreview = useCallback(() => {
    if (artifactPreviewUrlRef.current) {
      URL.revokeObjectURL(artifactPreviewUrlRef.current);
      artifactPreviewUrlRef.current = null;
    }
    setArtifactPreview(null);
  }, []);

  useEffect(() => () => {
    if (artifactPreviewUrlRef.current) {
      URL.revokeObjectURL(artifactPreviewUrlRef.current);
      artifactPreviewUrlRef.current = null;
    }
  }, []);

  const showArtifactPreview = useCallback(async (artifact: TaskArtifact) => {
    const html = await fetchArtifactHTML(artifact);
    const previewUrl = URL.createObjectURL(new Blob([html], { type: 'text/html' }));
    const previousPreviewUrl = artifactPreviewUrlRef.current;
    artifactPreviewUrlRef.current = previewUrl;
    setArtifactPreview({ ...artifact, previewUrl });
    if (previousPreviewUrl) {
      URL.revokeObjectURL(previousPreviewUrl);
    }
  }, [fetchArtifactHTML]);

  const refreshArtifactHistory = useCallback(async (taskId: string) => {
    const artifacts = await listArtifacts(taskId);
    setArtifactHistory(artifacts);
    return artifacts;
  }, [listArtifacts]);

  const showExistingArtifact = useCallback(async (taskId: string) => {
    const artifacts = await refreshArtifactHistory(taskId);
    const published = artifacts.find((artifact) => artifact.summary !== 'pending');
    if (published) {
      await showArtifactPreview(published);
      return true;
    }
    return false;
  }, [refreshArtifactHistory, showArtifactPreview]);

  const handleOpenArtifactReference = useCallback(async (ref: string) => {
    artifactReturnFocusRef.current = document.activeElement instanceof HTMLElement
      ? document.activeElement
      : null;
    try {
      const url = new URL(ref, window.location.origin);
      if (url.pathname.startsWith('/api/v1/artifacts/')) {
        const artifact = await apiClient.get<TaskArtifact>(`${url.pathname.replace(/\/meta$/, '')}/meta`);
        await showArtifactPreview(artifact);
        return;
      }

      const fileMatch = ref.match(/\/\.solo\/artifacts\/([^/\s]+)\/([^/\s]+\.html)/);
      if (fileMatch) {
        const [, taskId, filename] = fileMatch;
        const artifacts = await refreshArtifactHistory(taskId);
        const artifact = artifacts.find((item) => item.summary !== 'pending' && item.html_path.endsWith(`/${filename}`))
          ?? artifacts.find((item) => item.summary !== 'pending');
        if (artifact) {
          await showArtifactPreview(artifact);
          return;
        }
      }
    } catch {
      // Fall through to toast below.
    }
    artifactReturnFocusRef.current = null;
    showToast('Could not open artifact link.', 'error');
  }, [refreshArtifactHistory, showArtifactPreview, showToast]);

  useEffect(() => {
    if (!artifactPreview) return;

    artifactCloseButtonRef.current?.focus();
    const handleArtifactPreviewKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault();
        closeArtifactPreview();
        return;
      }

      if (event.key === 'Tab') {
        const controls = ([artifactOpenLinkRef.current, artifactRegenerateButtonRef.current, artifactFrameRef.current, artifactCloseButtonRef.current] as Array<HTMLElement | null>).filter(
          (control): control is HTMLElement => Boolean(control),
        );
        if (controls.length === 0) return;

        const firstControl = controls[0];
        const lastControl = controls[controls.length - 1];
        const activeElement = document.activeElement instanceof HTMLElement
          ? document.activeElement
          : null;

        if (event.shiftKey && activeElement === firstControl) {
          event.preventDefault();
          lastControl.focus();
        } else if (!event.shiftKey && activeElement === lastControl) {
          event.preventDefault();
          firstControl.focus();
        } else if (!activeElement || !controls.includes(activeElement)) {
          event.preventDefault();
          firstControl.focus();
        }
      }
    };

    document.addEventListener('keydown', handleArtifactPreviewKeyDown);
    return () => {
      document.removeEventListener('keydown', handleArtifactPreviewKeyDown);
      artifactReturnFocusRef.current?.focus();
      artifactReturnFocusRef.current = null;
    };
  }, [artifactPreview, closeArtifactPreview]);

  const openAgentDetail = useCallback((agent: AgentDetailTarget) => {
    setWorkspaceDetail({ relationship: null, agent });
    setMainPanel('detail');
    router.push(buildDashboardHref(channel.id, {
      ...dashboardUrlState,
      panel: 'agent',
      agentId: agent.id,
      relationshipId: null,
      threadId: null,
      messageId: null,
    }));
  }, [channel.id, dashboardUrlState, router]);

  const openWorkspaceDetail = useCallback((detail: WorkspaceDetail) => {
    setWorkspaceDetail(detail);
    setMainPanel('detail');
    router.push(buildDashboardHref(channel.id, {
      ...dashboardUrlState,
      panel: detail.relationship ? 'relationship' : 'agent',
      agentId: detail.agent?.id ?? null,
      relationshipId: detail.relationship?.id ?? null,
      threadId: null,
      messageId: null,
    }));
  }, [channel.id, dashboardUrlState, router]);

  const {
    tasks: channelTasks,
    isLoading: tasksLoading,
    error: tasksError,
    convertMessageToTask,
    refetch: refetchTasks,
  } = useTasks({ channel_id: channel.id });

  const {
    context: channelContext,
    team: channelTeam,
    isLoading: workspaceLoading,
    error: workspaceError,
    refetch: refetchWorkspace,
  } = useChannelWorkspace(channel.id);

  const {
    thoughts,
    activeThought,
    isLoading: thoughtsLoading,
    error: thoughtsError,
    refetch: refetchThoughts,
    completeThought,
    requestThoughtReview,
  } = useThoughts(channel.id);

  const workspaceScope = panelToWorkspaceScope(dashboardUrlState.panel, dashboardUrlState.view);
  const workspaceLeftTab = coerceLeftTab(workspaceScope, panelToLeftTab(dashboardUrlState.panel));
  const workspacePanelTab = viewToWorkspacePanelTab(dashboardUrlState.view);
  const workspaceRightMode = viewToRightMode(dashboardUrlState.view) ?? defaultRightMode(workspacePanelTab);
  const workspaceLeftTabs = SCOPE_LEFT_TABS[workspaceScope];
  const scopedMessageScope = useMemo<MessageScopeOptions>(() => {
    if (workspaceScope === 'thought') return { workspaceScope: 'thought' };
    if (workspaceScope === 'task') return { workspaceScope: 'task' };
    return { workspaceScope: 'channel' };
  }, [workspaceScope]);
  const scopedMessages = useMessages(channel.id, scopedMessageScope);

  const selectedTask = useMemo(() => {
    if (dashboardUrlState.taskId) {
      return channelTasks.find((task) => task.id === dashboardUrlState.taskId) ?? null;
    }
    return channelTasks.find((task) => !task.parent_task_id) ?? channelTasks[0] ?? null;
  }, [channelTasks, dashboardUrlState.taskId]);

  const selectedThoughtNode = useMemo(() => {
    if (!activeThought) return null;
    const nodeParam = dashboardUrlState.nodeId;
    if (!nodeParam) return activeThought.nodes.find((node) => node.is_root) ?? null;
    return activeThought.nodes.find((node) => node.id === nodeParam || node.title.toLowerCase() === nodeParam.toLowerCase()) ?? null;
  }, [activeThought, dashboardUrlState.nodeId]);

  const selectedTaskParentMessage = useMemo((): Message | null => {
    if (!selectedTask?.message_id) return null;
    const existing = messages.find((message) => message.id === selectedTask.message_id);
    if (existing) return { ...existing, display_name: selectedTask.creator_name || existing.display_name };
    return {
      id: selectedTask.message_id,
      channel_id: channel.id,
      user_id: selectedTask.creator_id,
      display_name: selectedTask.creator_name || selectedTask.creator_id.slice(0, 8),
      content: selectedTask.description || selectedTask.title,
      created_at: selectedTask.created_at,
      status: 'sent',
      sender_type: 'user',
      task_number: selectedTask.task_number,
      task_status: selectedTask.status,
      task_claimer_name: selectedTask.claimer_name || selectedTask.assignee_name,
    };
  }, [channel.id, messages, selectedTask]);

  const thoughtConversationMessages = useMemo(() => (
    scopedMessages.messages.filter((message) => {
      if (message.content_type === 'card.thought_review') {
        const payload = parseMessageCardPayload<{ thought_id?: string }>(message);
        return (!selectedThoughtNode || selectedThoughtNode.is_root)
          && (!activeThought?.id || payload?.thought_id === activeThought.id);
      }
      if (message.content_type && message.content_type !== 'text') return false;
      return !selectedThoughtNode?.id || message.subject_id === selectedThoughtNode.id;
    })
  ), [activeThought?.id, scopedMessages.messages, selectedThoughtNode?.id, selectedThoughtNode?.is_root]);

  const taskConversationMessages = useMemo(() => (
    scopedMessages.messages.filter((message) => {
      if (message.content_type === 'card.task_review') {
        const payload = parseMessageCardPayload<{ task_id?: string }>(message);
        return !selectedTask?.id || payload?.task_id === selectedTask.id;
      }
      if (message.content_type === 'card.tasks_created') {
        return !selectedTask?.id || message.subject_id === selectedTask.id;
      }
      if (message.workspace_scope === 'task') {
        return !selectedTask?.id || message.subject_id === selectedTask.id;
      }
      return false;
    })
  ), [scopedMessages.messages, selectedTask?.id]);

  const workspaceTitle = useMemo(() => {
    if (workspaceScope === 'thought') {
      return [activeThought?.title ?? 'Thought', selectedThoughtNode?.title].filter(Boolean).join(' · ');
    }
    if (workspaceScope === 'task') {
      return selectedTask?.title ?? 'Task';
    }
    return channel.name;
  }, [activeThought?.title, channel.name, selectedTask?.title, selectedThoughtNode?.title, workspaceScope]);

  const updateWorkspaceUrl = useCallback(
    (patch: Partial<Omit<DashboardUrlState, 'channelId'>>) => {
      router.push(buildDashboardHref(channel.id, { ...dashboardUrlState, ...patch }));
    },
    [channel.id, dashboardUrlState, router],
  );

  useEffect(() => {
    if (dashboardUrlState.panel === 'thread' || dashboardUrlState.panel === 'agent' || dashboardUrlState.panel === 'relationship') {
      return;
    }
    if (dashboardUrlState.panel === 'thought') {
      setMainPanel('thought');
      return;
    }
    setThreadMessage(null);
    setThreadTask(null);
    setWorkspaceDetail(null);
    setMainPanel(null);
  }, [dashboardUrlState.panel]);

  useEffect(() => {
    if (dashboardUrlState.panel === 'agent') {
      const memberAgent = agents.find((agent) => agent.member_id === dashboardUrlState.agentId);
      const teamAgent = channelTeam?.agents.find((agent) => agent.id === dashboardUrlState.agentId);
      const agent = teamAgent
        ? { id: teamAgent.id, name: teamAgent.name, is_active: teamAgent.status !== 'offline' }
        : memberAgent
          ? { id: memberAgent.member_id, name: memberAgent.display_name, is_active: memberAgent.status !== 'offline' }
          : null;
      if (!agent) return;
      setWorkspaceDetail({ relationship: null, agent });
      setMainPanel('detail');
      return;
    }

    if (dashboardUrlState.panel === 'relationship') {
      const relationship = channelTeam?.relationships.find((item) => item.id === dashboardUrlState.relationshipId);
      if (!relationship?.id) return;
      const agentsById = new Map((channelTeam?.agents ?? []).map((agent) => [agent.id, agent]));
      const fromAgent = agentsById.get(relationship.from_agent_id);
      const toAgent = agentsById.get(relationship.to_agent_id);
      setWorkspaceDetail({
        relationship: {
          id: relationship.id,
          from_agent_id: relationship.from_agent_id,
          from_agent_name: fromAgent?.name,
          from_agent_active: fromAgent?.status !== 'offline',
          to_agent_id: relationship.to_agent_id,
          to_agent_name: toAgent?.name,
          to_agent_active: toAgent?.status !== 'offline',
          rel_type: relationship.rel_type === 'collaborates_with' ? 'collaborates_with' : 'assigns_to',
          channel_id: channel.id,
        },
        agent: null,
      });
      setMainPanel('detail');
    }
  }, [
    agents,
    channel.id,
    channelTeam?.agents,
    channelTeam?.relationships,
    dashboardUrlState.agentId,
    dashboardUrlState.panel,
    dashboardUrlState.relationshipId,
  ]);

  const setWorkspacePanelTab = useCallback(
    (tab: WorkspacePanelTab) => {
      setThreadMessage(null);
      setThreadTask(null);
      setWorkspaceDetail(null);
      setMainPanel(null);
      updateWorkspaceUrl({
        view: tabToView(tab),
        panel: 'conversation',
        taskId: tab === 'task' ? dashboardUrlState.taskId : null,
        threadId: null,
        nodeId: tab === 'thought' ? dashboardUrlState.nodeId : null,
        messageId: null,
      });
    },
    [dashboardUrlState.nodeId, dashboardUrlState.taskId, updateWorkspaceUrl],
  );

  const setWorkspaceRightMode = useCallback(
    (mode: RightMode) => {
      if (workspacePanelTab === 'thought') {
        updateWorkspaceUrl({
          view: mode === 'board' ? 'thought.board' : 'thought.map',
          nodeId: mode === 'board' ? null : dashboardUrlState.nodeId,
        });
        return;
      }
      if (workspacePanelTab === 'task') {
        updateWorkspaceUrl({
          view: mode === 'graph' ? 'task.graph' : 'task.board',
          taskId: mode === 'graph' ? dashboardUrlState.taskId : null,
        });
        return;
      }
    },
    [dashboardUrlState.nodeId, dashboardUrlState.taskId, updateWorkspaceUrl, workspacePanelTab],
  );

  const [taskArtifactRecords, setTaskArtifactRecords] = useState<ContextRecord[]>([]);

  useEffect(() => {
    let cancelled = false;
    if (
      workspaceScope !== 'task' ||
      workspaceLeftTab !== 'artifact' ||
      !selectedTask ||
      selectedTask.artifact_status !== 'available'
    ) {
      setTaskArtifactRecords([]);
      return () => {
        cancelled = true;
      };
    }
    listArtifacts(selectedTask.id)
      .then((artifacts) => {
        if (!cancelled) setTaskArtifactRecords(artifacts.map(taskArtifactToRecord));
      })
      .catch(() => {
        if (!cancelled) setTaskArtifactRecords([]);
      });
    return () => {
      cancelled = true;
    };
  }, [listArtifacts, selectedTask, workspaceLeftTab, workspaceScope]);

  const leftRecordView = useMemo((): { title: string; records: ContextRecord[] } => {
    if (workspaceScope === 'thought') {
      const scopedRecords = (records: ContextRecord[]) => selectedThoughtNode?.is_root
        ? records
        : records.filter((record) => record.subject_id === selectedThoughtNode?.id);
      if (workspaceLeftTab === 'insight') {
        return { title: 'Thought 洞察', records: scopedRecords(activeThought?.insight_records ?? []) };
      }
      if (workspaceLeftTab === 'artifact') {
        return { title: 'Thought 产物', records: scopedRecords(activeThought?.artifact_records ?? []) };
      }
      return { title: 'Thought 摘要', records: scopedRecords(activeThought?.summary_records ?? []) };
    }
    if (workspaceScope === 'task') {
      if (workspaceLeftTab === 'artifact') {
        return { title: 'Task 产物', records: taskArtifactRecords };
      }
      return {
        title: 'Task 摘要',
        records: (channelContext?.latest_summary_records ?? []).filter((record) => record.subject_type === 'task'),
      };
    }
    return {
      title: 'Channel 摘要',
      records: channelContext?.latest_summary_records ?? [],
    };
  }, [
    activeThought?.artifact_records,
    activeThought?.insight_records,
    activeThought?.summary_records,
    channelContext?.latest_summary_records,
    selectedThoughtNode?.id,
    selectedThoughtNode?.is_root,
    taskArtifactRecords,
    workspaceLeftTab,
    workspaceScope,
  ]);

  // Refetch tasks when opening the right-side task workspace.
  useEffect(() => {
    if (workspacePanelTab === 'task') refetchTasks();
  }, [refetchTasks, workspacePanelTab]);

  // ---- Agent member status tracking (SOLO-47-F) ----
  // SOLO-island PR2: removed thinkingAgentNames/typingAgentNames state and
  // the TypingIndicator badge. AgentIsland (mounted at the dashboard root)
  // is now the single source of truth for "agent is working". The member
  // list (right column) still updates its dots to reflect thinking/typing
  // /online so the avatar stays in sync, but with a much simpler model.
  //
  // PR-fix: the previous 5s inactivity heuristic was the same class of
  // bug as the 3s heuristic in useAgentChunks (fixed in PR0). It would
  // prematurely mark an agent as online during long-running tool calls.
  // Now we rely on agent.run.finished as the authoritative terminal signal and
  // treat thinking/typing as transient — cleared the moment we see
  // a done event (or message.new) for the same agent.
  const memberStatusTimersRef = useRef<Map<string, ReturnType<typeof setTimeout>>>(
    new Map(),
  );

  /** Cancel any pending revert timer for an agent. */
  const cancelMemberStatusRevert = (agentId: string) => {
    const t = memberStatusTimersRef.current.get(agentId);
    if (t) {
      clearTimeout(t);
      memberStatusTimersRef.current.delete(agentId);
    }
  };

  /**
   * Schedule a fallback revert to online after `ms` ms. This is a safety
   * net for paths that don't emit agent.run.finished (e.g. the legacy LLM
   * provider fallback). Primary terminal signal remains agent.run.finished.
   */
  const scheduleMemberStatusRevert = (agentId: string, ms: number) => {
    cancelMemberStatusRevert(agentId);
    const timer = setTimeout(() => {
      updateMemberStatus(agentId, 'online');
      memberStatusTimersRef.current.delete(agentId);
    }, ms);
    memberStatusTimersRef.current.set(agentId, timer);
  };

  const { onEvent } = useWebSocket();

  // Listen for agent status events from WS (works with both real and mock WS)
  useEffect(() => {
    const unsub = onEvent((event) => {
      if (event.type === 'agent.thinking' || event.type === 'agent.typing') {
        if (event.channel_id !== channel.id) return;

        const isThinking = event.type === 'agent.thinking';
        updateMemberStatus(event.agent_id, isThinking ? 'thinking' : 'typing');
        // Fallback only — agent.run.finished will cancel and immediately revert.
        scheduleMemberStatusRevert(event.agent_id, 30_000);
      }

      if (event.type === 'agent.error' && event.channel_id === channel.id && !event.thread_id) {
        showToast(t('agentRunFailedToast', {
          name: event.agent_name ?? t('agent'),
          reason: displayAgentErrorReason(event.error, event.detail),
        }), 'error');
      }

      // agent.run.finished is the authoritative terminal signal. Clear any
      // fallback timer and snap status back to online immediately.
      if (event.type === 'agent.run.finished' && event.channel_id === channel.id && event.agent_id) {
        cancelMemberStatusRevert(event.agent_id);
        updateMemberStatus(event.agent_id, 'online');
      }

      // When an agent sends a message, revert their member status to online.
      if (
        event.type === 'message.new' &&
        event.channel_id === channel.id &&
        event.sender_type === 'agent' &&
        event.sender_id
      ) {
        cancelMemberStatusRevert(event.sender_id);
        updateMemberStatus(event.sender_id, 'online');
      }
    });

    return () => {
      unsub();
      for (const timer of memberStatusTimersRef.current.values()) {
        clearTimeout(timer);
      }
      memberStatusTimersRef.current.clear();
    };
  }, [channel.id, onEvent, showToast, updateMemberStatus]);

  // ---- Handle initialThreadMessageId: watch messages list for the target ----
  useEffect(() => {
    if (dashboardUrlState.panel !== 'thread' || !initialThreadMessageId || !channel) return;

    // Check if the message is already in the loaded list
    const found = messages.find((m) => m.id === initialThreadMessageId);
    if (found) {
      const task = channelTasks.find((t) => t.message_id === initialThreadMessageId) ?? null;
      setThreadMessage(found);
      setThreadTask(task);
      setWorkspaceDetail(null);
      setMainPanel('thread');
    }
    // If not found yet, it will be caught when messages load (via the next effect)
  }, [dashboardUrlState.panel, initialThreadMessageId, channel, messages, channelTasks]);

  // Handle initialScrollToMessageId: scroll to a specific message on mount or URL change.
  // Waits for isLoading to become false so the message DOM exists.
  const lastScrollTarget = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (!initialScrollToMessageId || !channel || isLoading) return;
    if (lastScrollTarget.current === initialScrollToMessageId) return;
    lastScrollTarget.current = initialScrollToMessageId;
    setScrollToMessageId(initialScrollToMessageId);
    setScrollMsgKey((k) => k + 1);
  }, [initialScrollToMessageId, channel, isLoading]);

  // Sync threadTask when channelTasks change (e.g. after WS task.updated)
  useEffect(() => {
    if (!threadMessage) return;
    const task = channelTasks.find((t) => t.message_id === threadMessage.id);
    if (task) {
      setThreadTask((prev) => {
        // Only update if actually changed to avoid re-render loops
        if (!prev || prev.status !== task.status || prev.claimer_id !== task.claimer_id) {
          return task;
        }
        return prev;
      });
    }
  }, [channelTasks, threadMessage]);

  // ---- Task click in tasks tab: open ThreadPanel with the parent message ----
  const handleTaskClickInTab = useCallback(
    (task: Task) => {
      if (!task.message_id) return;
      const openTaskGraph = () => updateWorkspaceUrl({
        view: 'task.graph',
        panel: 'thread',
        taskId: task.id,
        threadId: task.message_id!,
        messageId: null,
        nodeId: null,
      });

      // Find message in the already-loaded channel messages
      const existingMsg = messages.find((m) => m.id === task.message_id);
      if (existingMsg) {
        setThreadMessage({
          ...existingMsg,
          display_name: task.creator_name || existingMsg.display_name,
        });
        setThreadTask(task);
        setWorkspaceDetail(null);
        setMainPanel('thread');
        openTaskGraph();
        return;
      }

      // Message not in current loaded set — ThreadPanel loads its own
      // data via useThread so a synthetic parent message is sufficient
      setThreadTask(task);
      setThreadMessage({
        id: task.message_id,
        channel_id: channel.id,
        user_id: task.creator_id,
        display_name: task.creator_name || task.creator_id.slice(0, 8),
        content: task.description || task.title,
        created_at: task.created_at,
        status: 'sent',
        sender_type: 'user',
        task_number: task.task_number,
        task_status: task.status,
        task_claimer_name: task.claimer_name || task.assignee_name,
      });
      setWorkspaceDetail(null);
      setMainPanel('thread');
      openTaskGraph();
    },
    [channel.id, messages, updateWorkspaceUrl],
  );

  const handleThoughtNodeSelect = useCallback((nodeId: string) => {
    setThreadMessage(null);
    setThreadTask(null);
    setWorkspaceDetail(null);
    setMainPanel('thought');
    updateWorkspaceUrl({
      view: workspaceRightMode === 'board' ? 'thought.board' : 'thought.map',
      panel: 'thought',
      nodeId,
      taskId: null,
      threadId: null,
      messageId: null,
    });
  }, [updateWorkspaceUrl, workspaceRightMode]);

  const handleTaskNodeSelect = useCallback((task: Task) => {
    handleTaskClickInTab(task);
  }, [handleTaskClickInTab]);

  const handleThreadClose = useCallback(() => {
    setThreadMessage(null);
    setThreadTask(null);
    setMainPanel(null);
    updateWorkspaceUrl({ panel: 'conversation', threadId: null });
  }, [updateWorkspaceUrl]);

  const handleAgentDetailClose = useCallback(() => {
    setWorkspaceDetail(null);
    setMainPanel(threadMessage ? 'thread' : null);
  }, [threadMessage]);

  const handleAgentDeleted = useCallback(() => {
    handleAgentDetailClose();
    void refetchWorkspace();
  }, [handleAgentDetailClose, refetchWorkspace]);

  const handleRemoveAgentFromChannel = useCallback(async (memberId: string) => {
    await removeMember('agent', memberId);
    await refetchWorkspace();
  }, [refetchWorkspace, removeMember]);

  // v1.5: Wrap onReply to also sync thread state to URL + pull latest task data
  const handleReply = useCallback(
    (message: Message) => {
      const task = channelTasks.find((item) => item.message_id === message.id) ?? null;
      refetchTasks();
      setThreadTask(task);
      setThreadMessage(message);
      setWorkspaceDetail(null);
      setMainPanel('thread');
      updateWorkspaceUrl({
        view: 'task.graph',
        panel: 'thread',
        taskId: task?.id ?? null,
        threadId: message.id,
        messageId: null,
        nodeId: null,
      });
    },
    [channelTasks, refetchTasks, updateWorkspaceUrl],
  );

  // P25-08-F: Called by ThreadPanel after successfully marking thread as read
  const handleThreadMarkRead = useCallback(() => {
    if (threadMessage) {
      markMessageThreadRead(threadMessage.id);
    }
  }, [threadMessage, markMessageThreadRead]);

  const handleViewThreadInChannel = useCallback(() => {
    if (!threadMessage) return;
    setScrollToMessageId(threadMessage.id);
    setScrollMsgKey((k) => k + 1);
    updateWorkspaceUrl({
      panel: 'conversation',
      threadId: null,
      messageId: threadMessage.id,
    });
  }, [threadMessage, updateWorkspaceUrl]);

  const handleViewThreadTask = useCallback(() => {
    const taskNumber = threadTask?.task_number ?? threadMessage?.task_number;
    if (!threadMessage || taskNumber == null) return;
    updateWorkspaceUrl({
      view: 'task.graph',
      panel: 'thread',
      taskId: threadTask?.id ?? null,
      nodeId: null,
      threadId: threadMessage.id,
    });
  }, [threadMessage, threadTask, updateWorkspaceUrl]);

  const existingAgentIds = agents.map((a) => a.member_id);
  const canAddAgents = !channel.name.startsWith('all-');

  // ---- Task quick-create handler (SOLO-128-F) ----

  // ---- AsTask handler: convert directly, no dialog ----
  const handleAsTaskOpen = useCallback(
    async (message: Message) => {
      const title = message.content.slice(0, 200);
      try {
        const task = await convertMessageToTask(
          message.channel_id,
          message.id,
          title,
        );
        showToast(t('taskConverted', { n: task.task_number ?? '?' }), 'success');
      } catch {
        showToast(t('taskConvertError'), 'error');
      }
    },
    [convertMessageToTask, showToast],
  );

  const handleCreateChannelFromCard = useCallback(
    async (message: Message, input: { channel_name: string; template: string }) => {
      try {
        const created = await apiClient.post<{ id: string }>(
          `/api/v1/channels/${channel.id}/messages/${message.id}/create-channel`,
          input,
        );
        await onChannelCreated?.();
        router.push(`/dashboard?channel=${created.id}`);
      } catch {
        showToast('Could not create channel from card.', 'error');
      }
    },
    [channel.id, onChannelCreated, router, showToast],
  );

  const handleStartWorkFromCard = useCallback(
    async (message: Message) => {
      try {
        await apiClient.post(`/api/v1/channels/${channel.id}/tasks/from-context`, {
          source_message_id: message.id,
        });
        setWorkspacePanelTab('task');
        await refetchMessages();
        await refetchTasks();
      } catch {
        showToast('Could not create tasks from context.', 'error');
      }
    },
    [channel.id, refetchMessages, refetchTasks, setWorkspacePanelTab, showToast],
  );

  const handleRequestThoughtReview = useCallback(
    async (thoughtId: string) => {
      try {
        await requestThoughtReview(thoughtId);
        await refetchMessages();
        await scopedMessages.refetch();
        await refetchThoughts();
        updateWorkspaceUrl({ view: 'thought.map', panel: 'thought' });
      } catch {
        showToast('Could not request thought review.', 'error');
      }
    },
    [refetchMessages, refetchThoughts, requestThoughtReview, scopedMessages, showToast, updateWorkspaceUrl],
  );

  const handleCompleteThoughtFromCard = useCallback(
    async (message: Message, thoughtId: string) => {
      try {
        await completeThought(thoughtId, { message_id: message.id });
        await apiClient.post(`/api/v1/channels/${channel.id}/tasks/from-context`, {
          source_message_id: message.id,
          source_thought_id: thoughtId,
        });
        setWorkspacePanelTab('task');
        await refetchMessages();
        await scopedMessages.refetch();
        await refetchWorkspace();
        await refetchThoughts();
        await refetchTasks();
      } catch {
        showToast('Could not finish thought review.', 'error');
      }
    },
    [channel.id, completeThought, refetchMessages, refetchTasks, refetchThoughts, refetchWorkspace, scopedMessages, setWorkspacePanelTab, showToast],
  );

  const handleTaskReviewAction = useCallback(async () => {
    setWorkspacePanelTab('task');
    await refetchMessages();
    await scopedMessages.refetch();
    await refetchTasks();
    await refetchWorkspace();
  }, [refetchMessages, refetchTasks, refetchWorkspace, scopedMessages, setWorkspacePanelTab]);

  const handleThoughtConversationSubmit = useCallback((content: string) => {
    if (!selectedThoughtNode) return;
    void scopedMessages.sendMessage(content, undefined, false, undefined, {
      workspaceScope: 'thought',
      subjectType: 'thought_node',
      subjectId: selectedThoughtNode.id,
    });
  }, [scopedMessages, selectedThoughtNode]);

  const handleTaskConversationSubmit = useCallback((content: string) => {
    if (!selectedTask) return;
    void scopedMessages.sendMessage(content, undefined, false, undefined, {
      workspaceScope: 'task',
      subjectType: 'task',
      subjectId: selectedTask.id,
    });
  }, [scopedMessages, selectedTask]);

  const handleTaskActionComplete = useCallback((updated: Task) => {
    setThreadTask((prev) => (prev?.id === updated.id ? updated : prev));
    refetchTasks();
  }, [refetchTasks]);

  useEffect(() => {
    if (!artifactPreview) return;

    const handleArtifactMessage = async (event: MessageEvent) => {
      if (event.source !== artifactFrameRef.current?.contentWindow) return;
      const data = event.data;
      if (!data || typeof data !== 'object' || data.type !== 'artifact.reviewAction') return;
      const taskId = typeof data.taskId === 'string' && data.taskId.trim() !== ''
        ? data.taskId.trim()
        : artifactPreview.task_id;
      if (artifactPreview.task_id && taskId && taskId !== artifactPreview.task_id) return;
      if (data.action !== 'accept' && data.action !== 'reject') return;
      if (artifactReviewBusy) return;

      const reason = typeof data.reason === 'string' ? data.reason.trim() : '';
      if (data.action === 'reject' && reason === '') {
        showToast('Reject comment is required.', 'error');
        return;
      }

      setArtifactReviewBusy(true);
      try {
        if (!taskId) throw new Error('missing task id');
        const path = `/api/v1/tasks/${taskId}/${data.action === 'accept' ? 'accept' : 'reject'}`;
        const updated = await apiClient.post<Task>(path, data.action === 'reject' ? { reason } : undefined);
        handleTaskActionComplete(updated);
        closeArtifactPreview();
        showToast(data.action === 'accept' ? 'Task accepted.' : 'Task rejected.', 'success');
      } catch {
        showToast(data.action === 'accept' ? 'Could not accept task.' : 'Could not reject task.', 'error');
      } finally {
        setArtifactReviewBusy(false);
      }
    };

    window.addEventListener('message', handleArtifactMessage);
    return () => window.removeEventListener('message', handleArtifactMessage);
  }, [artifactPreview, artifactReviewBusy, closeArtifactPreview, handleTaskActionComplete, showToast]);

  const handleGenerateArtifact = useCallback(async (task: Task) => {
    const action = getTaskArtifactAction(task, isGeneratingTask(task.id));
    if (action === 'hidden' || action === 'pending') return;
    artifactReturnFocusRef.current = document.activeElement instanceof HTMLElement
      ? document.activeElement
      : null;

    try {
      if (action === 'read') {
        await showExistingArtifact(task.id);
        return;
      }
      if (await showExistingArtifact(task.id)) return;
      const artifact = await generateArtifact(task.id);
      await refreshArtifactHistory(task.id);
      await showArtifactPreview(artifact);
    } catch (error) {
      if (error instanceof TaskArtifactStillPendingError) {
        const showedExisting = await showExistingArtifact(task.id);
        if (!showedExisting) {
          artifactReturnFocusRef.current = null;
          showToast('Artifact is still generating. Try again in a moment.', 'error');
        }
        return;
      }
      artifactReturnFocusRef.current = null;
      showToast('Could not generate artifact. Please try again.', 'error');
    }
  }, [generateArtifact, isGeneratingTask, refreshArtifactHistory, showArtifactPreview, showExistingArtifact, showToast]);

  const handleRegenerateArtifact = useCallback(async () => {
    if (!artifactPreview || isGeneratingTask(artifactPreview.task_id)) return;

    try {
      const artifact = await regenerateArtifact(artifactPreview.task_id);
      await refreshArtifactHistory(artifactPreview.task_id);
      await showArtifactPreview(artifact);
    } catch (error) {
      if (error instanceof TaskArtifactStillPendingError) {
        showToast('Artifact is still regenerating. Try again in a moment.', 'error');
        return;
      }
      showToast('Could not regenerate artifact. Please try again.', 'error');
    }
  }, [artifactPreview, regenerateArtifact, isGeneratingTask, refreshArtifactHistory, showArtifactPreview, showToast]);

  // SOLO-island PR2: removed agentActivities aggregation — the
  // TypingIndicator it fed is now replaced by AgentIsland, which
  // subscribes to agent.activity events directly.
  const leftHeaderLabel = mainPanel === 'thread'
    ? t('thread')
    : mainPanel === 'detail'
      ? (workspaceDetail?.agent ? t('agentDetailTitle') : t('relationshipEditorEdgeDetail'))
      : mainPanel === 'thought'
        ? 'Thought'
        : workspaceScope;
  const leftHeaderTitle = mainPanel === 'thread'
    ? (threadTask?.title ?? threadMessage?.content.slice(0, 80) ?? t('thread'))
    : mainPanel === 'detail'
      ? (workspaceDetail?.agent?.name ?? t('relationshipEditorEdgeDetail'))
      : mainPanel === 'thought'
        ? (selectedThoughtNode?.title ?? activeThought?.title ?? 'Thought')
        : workspaceTitle;
  const showOuterHeader = mainPanel !== 'thread' && mainPanel !== 'detail';

  return (
    <div className="flex flex-1 overflow-hidden">
      {/* Left: message area */}
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        {/* Channel header */}
        {showOuterHeader && (
        <div className="sidebar-collapse-offset flex h-14 flex-shrink-0 items-center border-b-2 border-black px-4">
          <div className="flex items-center gap-2 flex-1 min-w-0">
            <span className="font-mono text-[10px] font-black uppercase text-muted-foreground flex-shrink-0">
              {leftHeaderLabel}
            </span>
            <h2 className="font-bold text-foreground truncate">{leftHeaderTitle}</h2>
            {!mainPanel && (
              <>
                <div className="mx-2 h-4 w-px bg-border flex-shrink-0" />
                <div className="flex items-center gap-1">
                  {workspaceLeftTabs.map((tab) => (
                    <button
                      key={tab.key}
                      type="button"
                      onClick={() => {
                        updateWorkspaceUrl({ panel: tab.key as DashboardPanel, threadId: null });
                      }}
                      className={tabButtonClass(workspaceLeftTab === tab.key)}
                    >
                      {tab.label}
                    </button>
                  ))}
                </div>
              </>
            )}
          </div>
        </div>
        )}

        {mainPanel === 'detail' && workspaceDetail && (
          <div className="min-h-0 flex-1 overflow-hidden">
            <RelationshipDetailPanel
              relationship={workspaceDetail.relationship}
              agent={workspaceDetail.agent}
              onClose={handleAgentDetailClose}
              onUpdate={() => {
                void refetchWorkspace();
              }}
              onDelete={() => {
                setWorkspaceDetail(null);
                setMainPanel(null);
                void refetchWorkspace();
              }}
              onAgentDeleted={handleAgentDeleted}
              embedded
            />
          </div>
        )}

        {mainPanel === 'thread' && threadMessage && (
          <div className="min-h-0 flex-1 overflow-hidden">
            <Suspense
              fallback={
                <div className="flex h-full items-center justify-center">
                  <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
                </div>
              }
            >
              <ThreadPanel
                parentMessage={threadMessage}
                onClose={handleThreadClose}
                members={members}
                replyCount={threadMessage.reply_count ?? 0}
                task={threadTask ?? undefined}
                onMarkRead={handleThreadMarkRead}
                onViewInChannel={handleViewThreadInChannel}
                onViewTask={handleViewThreadTask}
                onOpenArtifactReference={handleOpenArtifactReference}
                onAgentClick={openAgentDetail}
              />
            </Suspense>
          </div>
        )}

        {mainPanel === 'thought' && (
          <ThoughtConversationPanel
            thought={activeThought}
            node={selectedThoughtNode}
            messages={thoughtConversationMessages}
            members={members}
            onRetry={scopedMessages.retryMessage}
            onCompleteThoughtFromCard={handleCompleteThoughtFromCard}
            onSubmit={handleThoughtConversationSubmit}
          />
        )}

        {!mainPanel && workspaceLeftTab === 'conversation' && workspaceScope === 'channel' && (
          <div className="flex flex-1 flex-col overflow-hidden bg-brutal-cream">
            <MessageList
              messages={messages}
              beforeItems={
                showOnboardingWizard ? (
                  <div role="listitem" className="px-6">
                    <WizardCard channelId={channel.id} />
                  </div>
                ) : null
              }
              isLoading={isLoading}
              error={error}
              onRetry={(id, content) => retryMessage(id, content)}
              onCancel={(id) => cancelMessage(id)}
              onReply={handleReply}
              onAsTask={handleAsTaskOpen}
              onCreateChannelFromCard={handleCreateChannelFromCard}
              onStartWorkFromCard={handleStartWorkFromCard}
              onCompleteThoughtFromCard={handleCompleteThoughtFromCard}
              onTaskReviewAction={handleTaskReviewAction}
              onViewTaskGraph={() => setWorkspacePanelTab('task')}
              hasMore={hasMore}
              isLoadingMore={isLoadingMore}
              loadMoreError={loadMoreError}
              onLoadMore={loadMore}
              scrollToMessageId={scrollToMessageId}
              scrollKey={scrollMsgKey}
              members={members}
              onOpenArtifactReference={handleOpenArtifactReference}
              onAgentClick={openAgentDetail}
            />
            <MessageInput
              onSend={async (content, _mentionedAgentIds, asTask, taskTitle, attachmentIds) => {
                if (asTask) {
                  const result = await sendMessage(content, _mentionedAgentIds, true, attachmentIds);
                  if (result && result.task_number !== undefined) {
                    showToast(t('taskCreatedToast', { n: result.task_number }), 'success');
                    updateWorkspaceUrl({
                      view: 'task.board',
                      panel: 'thread',
                      taskId: null,
                      nodeId: null,
                      threadId: result.id,
                      messageId: null,
                    });
                  }
                } else {
                  const result = await sendMessage(content, _mentionedAgentIds, undefined, attachmentIds);
                  if (result && result.task_number !== undefined) {
                    showToast(t('taskCreatedToast', { n: result.task_number }), 'success');
                  }
                }
              }}
              members={members}
              showAsTaskToggle
            />
          </div>
        )}

        {!mainPanel && workspaceLeftTab === 'conversation' && workspaceScope === 'thought' && (
          <ThoughtConversationPanel
            thought={activeThought}
            node={selectedThoughtNode}
            messages={thoughtConversationMessages}
            members={members}
            onRetry={scopedMessages.retryMessage}
            onCompleteThoughtFromCard={handleCompleteThoughtFromCard}
            onSubmit={handleThoughtConversationSubmit}
          />
        )}

        {!mainPanel && workspaceLeftTab === 'conversation' && workspaceScope === 'task' && (
          <TaskConversationPanel
            task={selectedTask}
            parentMessage={selectedTaskParentMessage}
            cardMessages={taskConversationMessages}
            members={members}
            onRetry={retryMessage}
            onClose={() => updateWorkspaceUrl({ view: 'overview', panel: 'conversation', taskId: null })}
            onMarkRead={() => {
              if (selectedTaskParentMessage) markMessageThreadRead(selectedTaskParentMessage.id);
            }}
            onViewInChannel={() => {
              if (!selectedTaskParentMessage) return;
              setScrollToMessageId(selectedTaskParentMessage.id);
              setScrollMsgKey((k) => k + 1);
              updateWorkspaceUrl({ view: 'overview', panel: 'conversation', taskId: null, messageId: selectedTaskParentMessage.id });
            }}
            onViewTaskGraph={() => setWorkspacePanelTab('task')}
            onTaskReviewAction={handleTaskReviewAction}
            onOpenArtifactReference={handleOpenArtifactReference}
            onAgentClick={openAgentDetail}
            onSubmitFallback={handleTaskConversationSubmit}
          />
        )}

        {!mainPanel && workspaceLeftTab !== 'conversation' && (
          <RecordTimelinePanel title={leftRecordView.title} records={leftRecordView.records} />
        )}

      </div>

      {/* Right workspace */}
      <div
        className="relative w-1/2 flex-shrink-0 overflow-hidden border-l-2 border-black bg-brutal-cream"
      >
        <ChannelWorkspacePanel
          activeTab={workspacePanelTab}
          mode={workspaceRightMode}
          channelId={channel.id}
          onTabChange={setWorkspacePanelTab}
          onModeChange={setWorkspaceRightMode}
          context={channelContext}
          team={channelTeam}
          tasks={channelTasks}
          tasksLoading={tasksLoading}
          tasksError={tasksError}
          selectedTask={selectedTask}
          selectedThoughtNodeId={selectedThoughtNode?.id}
          thought={activeThought}
          thoughts={thoughts}
          thoughtsLoading={thoughtsLoading}
          thoughtsError={thoughtsError}
          onThoughtRetry={refetchThoughts}
          onCompleteThought={handleRequestThoughtReview}
          onThoughtNodeSelect={handleThoughtNodeSelect}
          onTaskSelect={handleTaskNodeSelect}
          onTaskRetry={refetchTasks}
          onTaskActionComplete={handleTaskActionComplete}
          onGenerateTaskArtifact={handleGenerateArtifact}
          isTaskArtifactGenerating={(task) => isGeneratingTask(task.id)}
          onTeamDetailOpen={openWorkspaceDetail}
          onTeamDetailClose={handleAgentDetailClose}
          canAddAgents={canAddAgents}
          onAddAgent={() => setIsAddAgentModalOpen(true)}
          onOpenMembers={() => setIsMemberPopoverOpen(true)}
          isLoading={workspaceLoading}
          error={workspaceError}
          onRetry={refetchWorkspace}
        />
      </div>

      {artifactPreview && (
        <div
          role="dialog"
          aria-modal="true"
          aria-labelledby="channel-artifact-preview-title"
          className="fixed inset-4 z-50 flex flex-col border-4 border-black bg-white shadow-brutal-xl"
        >
          <div className="flex items-center justify-between border-b-4 border-black px-4 py-2">
            <div id="channel-artifact-preview-title" className="font-heading text-sm font-black uppercase">{artifactPreview.title}</div>
            <div className="flex items-center gap-2">
              <a
                ref={artifactOpenLinkRef}
                href={artifactPreview.previewUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="border-2 border-black bg-white px-2 py-1 font-mono text-xs font-bold uppercase shadow-brutal-sm"
              >
                Open
              </a>
              <button
                ref={artifactRegenerateButtonRef}
                type="button"
                onClick={handleRegenerateArtifact}
                disabled={isGeneratingTask(artifactPreview.task_id)}
                className="border-2 border-black bg-white px-2 py-1 font-mono text-xs font-bold uppercase shadow-brutal-sm disabled:opacity-50"
              >
                Regenerate
              </button>
              <button
                ref={artifactCloseButtonRef}
                type="button"
                onClick={closeArtifactPreview}
                className="border-2 border-black bg-white px-2 py-1 font-mono text-xs font-bold uppercase shadow-brutal-sm"
                aria-label="Close artifact preview"
              >
                Close
              </button>
            </div>
          </div>
          <iframe ref={artifactFrameRef} title={artifactPreview.title} src={artifactPreview.previewUrl} tabIndex={0} className="min-h-0 flex-1 bg-white" />
        </div>
      )}

      {/* Add Agent to Channel modal */}
      <AddAgentModal
        open={isAddAgentModalOpen}
        onOpenChange={setIsAddAgentModalOpen}
        existingAgentIds={existingAgentIds}
        onAdd={addAgentToChannel}
        onChanged={refetchWorkspace}
      />


      {/* Member popover */}
      <Dialog open={isMemberPopoverOpen} onOpenChange={setIsMemberPopoverOpen}>
        <DialogHeader>
          <DialogTitle>
            <div className="flex items-center gap-2">
              <Users className="h-4 w-4" />
              {t('channelMembers')}
              <span className="font-mono text-sm font-normal text-muted-foreground">
                ({users.length + agents.length})
              </span>
            </div>
          </DialogTitle>
          <div className="flex items-center gap-1">
            {canAddAgents && (
              <Button
                type="button"
                onClick={() => {
                  setIsMemberPopoverOpen(false);
                  setIsAddAgentModalOpen(true);
                }}
                variant="success"
                size="icon"
                className="h-7 w-7"
                aria-label={t('addAgentToChannel')}
              >
                <Plus className="h-3.5 w-3.5" />
              </Button>
            )}
            <DialogCloseButton onClick={() => setIsMemberPopoverOpen(false)} />
          </div>
        </DialogHeader>
        <div className="max-h-[60vh] overflow-y-auto">
          <MemberList
            users={users}
            agents={agents}
            isLoading={membersLoading}
            onAddAgent={() => {
              setIsMemberPopoverOpen(false);
              setIsAddAgentModalOpen(true);
            }}
            onRemoveAgent={handleRemoveAgentFromChannel}
            onAgentClick={openAgentDetail}
            showHeader={false}
            canAddAgent={canAddAgents}
          />
        </div>
      </Dialog>

      {/* Mobile: member button */}
      <div className="lg:hidden">
        <button
          type="button"
          onClick={() => setIsMemberPopoverOpen(true)}
          className="btn-brutal fixed bottom-4 right-4 z-40 flex h-10 w-10 items-center justify-center shadow-brutal"
          aria-label={t('members')}
        >
          <Users className="h-4 w-4" />
        </button>
      </div>
    </div>
  );
}
