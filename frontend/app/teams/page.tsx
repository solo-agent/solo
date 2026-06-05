// ============================================================================
// SOLO-233-F: Agent Team Management page — agents grouped by inferred role
//   from system_prompt keywords. Collapsible groups with AgentCard grid.
// ============================================================================

'use client';

import { useEffect, useState, useCallback, useMemo } from 'react';
import { useRouter } from 'next/navigation';
import { Users, ChevronDown, AlertCircle, List, GitBranch, ArrowRight, Bot, Plus } from 'lucide-react';
import { useAuth } from '@/lib/auth-context';
import { AppFrame } from '@/components/layout/app-frame';
import { useAgents } from '@/lib/hooks/use-agents';
import { AgentCard } from '@/components/agents/agent-card';
import { Skeleton } from '@/components/ui/skeleton';
import { useToast } from '@/components/ui/toast';
import {
  Dialog,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  DialogCloseButton,
} from '@/components/ui/dialog';
import { cn } from '@/lib/utils';
import { AgentDetailPanel } from '@/components/agents/agent-detail-panel';
import type { Agent } from '@/lib/types';

// ---- Group inference ----

type TeamGroup =
  | '统筹'
  | '产品/项目管理'
  | '后端开发'
  | '前端开发'
  | '测试/QA'
  | '自定义角色';

const GROUP_ORDER: TeamGroup[] = [
  '统筹',
  '产品/项目管理',
  '后端开发',
  '前端开发',
  '测试/QA',
  '自定义角色',
];

const GROUP_DESCRIPTIONS: Record<TeamGroup, string> = {
  '统筹': '负责全局规划、协调和决策',
  '产品/项目管理': '负责产品定义、需求管理和项目推进',
  '后端开发': '负责后端服务、API、数据库和架构',
  '前端开发': '负责前端 UI、组件和用户体验',
  '测试/QA': '负责质量保障、测试和缺陷管理',
  '自定义角色': '未归类或自定义角色的 Agent',
};

function inferAgentGroup(systemPrompt: string): TeamGroup {
  const sp = systemPrompt.toLowerCase();
  if (sp.includes('leader') || sp.includes('统筹')) return '统筹';
  if (sp.includes('pm') || sp.includes('产品') || sp.includes('项目管理')) return '产品/项目管理';
  if (sp.includes('rd') || sp.includes('后端') || sp.includes('架构')) return '后端开发';
  if (sp.includes('fe') || sp.includes('前端') || sp.includes('ui')) return '前端开发';
  if (sp.includes('qa') || sp.includes('测试') || sp.includes('质量')) return '测试/QA';
  return '自定义角色';
}

// ---- Page ----

export default function TeamsPage() {
  const router = useRouter();
  const { isAuthenticated, isLoading: authLoading } = useAuth();
  const { agents, isLoading, error, refetch, deleteAgent, createAgent } = useAgents();
  const { showToast } = useToast();

  // Which groups are collapsed. All expanded by default.
  const [collapsedGroups, setCollapsedGroups] = useState<Set<TeamGroup>>(new Set());

  // View mode: list or structure
  const [viewMode, setViewMode] = useState<'list' | 'structure'>('list');

  // Delete confirmation state
  const [deleteTarget, setDeleteTarget] = useState<Agent | null>(null);
  const [isDeleting, setIsDeleting] = useState(false);

  // v1.5: Create Agent modal state
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [createName, setCreateName] = useState('');
  const [createDescription, setCreateDescription] = useState('');
  const [isCreating, setIsCreating] = useState(false);

  // Expanded groups in structure view
  const [expandedInStructure, setExpandedInStructure] = useState<Set<TeamGroup>>(new Set());

  // v1.5: Agent detail panel state
  const [panelAgentId, setPanelAgentId] = useState<string | null>(null);

  // Auth guard
  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      router.push('/auth/login');
    }
  }, [authLoading, isAuthenticated, router]);

  // Group agents
  const grouped = useMemo(() => {
    const map = new Map<TeamGroup, Agent[]>();
    for (const g of GROUP_ORDER) {
      map.set(g, []);
    }
    for (const agent of agents) {
      const group = inferAgentGroup(agent.system_prompt);
      map.get(group)!.push(agent);
    }
    return map;
  }, [agents]);

  const toggleGroup = useCallback((group: TeamGroup) => {
    setCollapsedGroups((prev) => {
      const next = new Set(prev);
      if (next.has(group)) {
        next.delete(group);
      } else {
        next.add(group);
      }
      return next;
    });
  }, []);

  // Delete handler
  const handleDeleteConfirm = useCallback(async () => {
    if (!deleteTarget) return;
    setIsDeleting(true);
    try {
      await deleteAgent(deleteTarget.id);
    } finally {
      setIsDeleting(false);
      setDeleteTarget(null);
    }
  }, [deleteTarget, deleteAgent]);

  // v1.5: Create Agent modal handler
  const handleCreateAgent = useCallback(async () => {
    if (!createName.trim() || isCreating) return;
    setIsCreating(true);
    try {
      await createAgent({
        name: createName.trim(),
        description: createDescription.trim() || undefined,
        model_provider: '', // Will be set via detail panel
      });
      showToast('Agent 创建成功', 'success');
      setIsCreateModalOpen(false);
      setCreateName('');
      setCreateDescription('');
    } catch {
      showToast('创建 Agent 失败，请稍后再试', 'error');
    } finally {
      setIsCreating(false);
    }
  }, [createName, createDescription, isCreating, createAgent, showToast]);

  if (authLoading || !isAuthenticated) {
    return (
      <div className="flex h-screen items-center justify-center bg-brutal-cream">
        <div className="flex flex-col items-center gap-3">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-brutal-pink border-t-transparent" />
          <p className="font-mono text-sm text-muted-foreground">加载中...</p>
        </div>
      </div>
    );
  }

  return (
    <AppFrame>
      <div className="mx-auto w-full max-w-5xl px-6 py-8 overflow-y-auto flex-1">
      {/* Page header */}
      <div className="mb-8 flex items-center gap-4">
        <div className="flex-1">
          <h1 className="text-2xl font-heading font-bold text-foreground">
            团队管理
          </h1>
          <p className="mt-1 font-body text-sm text-muted-foreground">
            按角色分组查看 Agent 团队配置
          </p>
        </div>
        <button
          type="button"
          onClick={() => setIsCreateModalOpen(true)}
          className="btn-brutal btn-brutal-pink"
        >
          <Plus className="mr-1.5 h-4 w-4" />
          创建 Agent
        </button>
      </div>

      {/* Error state */}
      {error && (
        <div className="mb-6 flex items-center gap-3 border-2 border-brutal-orange bg-brutal-orange-light p-4 shadow-brutal-sm">
          <AlertCircle className="h-5 w-5 flex-shrink-0 text-brutal-orange" />
          <span className="flex-1 font-body text-sm text-foreground">
            {error}
          </span>
          <button
            type="button"
            onClick={refetch}
            className="btn-brutal btn-brutal-sm"
          >
            重试
          </button>
        </div>
      )}

      {/* View mode toggle */}
      {!isLoading && !error && agents.length > 0 && (
        <div className="mb-6 flex items-center gap-2">
          <button
            type="button"
            onClick={() => setViewMode('list')}
            className={cn(
              'inline-flex items-center gap-2 border-2 border-black px-4 py-2 font-heading text-sm font-bold shadow-brutal-sm transition-all',
              viewMode === 'list'
                ? 'bg-brutal-pink text-white'
                : 'bg-white hover:shadow-brutal active:translate-x-0.5 active:translate-y-0.5 active:shadow-none',
            )}
            aria-pressed={viewMode === 'list'}
          >
            <List className="h-4 w-4" />
            列表
          </button>
          <button
            type="button"
            onClick={() => setViewMode('structure')}
            className={cn(
              'inline-flex items-center gap-2 border-2 border-black px-4 py-2 font-heading text-sm font-bold shadow-brutal-sm transition-all',
              viewMode === 'structure'
                ? 'bg-brutal-pink text-white'
                : 'bg-white hover:shadow-brutal active:translate-x-0.5 active:translate-y-0.5 active:shadow-none',
            )}
            aria-pressed={viewMode === 'structure'}
          >
            <GitBranch className="h-4 w-4" />
            结构
          </button>
        </div>
      )}

      {/* Loading skeleton */}
      {isLoading && (
        <div className="space-y-8">
          {GROUP_ORDER.map((group) => (
            <div key={group}>
              <div className="mb-3 flex items-center gap-3">
                <Skeleton className="h-7 w-32 rounded-none" />
                <Skeleton className="h-5 w-8 rounded-full" />
              </div>
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
                {[1, 2].map((i) => (
                  <div
                    key={i}
                    className="border-2 border-black bg-white p-6 shadow-brutal"
                  >
                    <div className="flex items-center gap-3">
                      <Skeleton className="h-12 w-12 rounded-none" />
                      <div className="flex-1 space-y-2">
                        <Skeleton className="h-4 w-24 rounded-none" />
                        <Skeleton className="h-3 w-32 rounded-none" />
                      </div>
                    </div>
                    <div className="mt-4 space-y-2">
                      <Skeleton className="h-3 w-full rounded-none" />
                      <Skeleton className="h-3 w-3/4 rounded-none" />
                    </div>
                    <div className="mt-3">
                      <Skeleton className="h-12 w-full rounded-none" />
                    </div>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Empty state — no agents at all */}
      {!isLoading && !error && agents.length === 0 && (
        <div className="flex flex-col items-center justify-center border-2 border-dashed border-black py-20">
          <div className="mb-4 flex h-16 w-16 items-center justify-center border-2 border-black bg-brutal-pink shadow-brutal">
            <Users className="h-8 w-8 text-white" />
          </div>
          <h2 className="text-xl font-heading font-bold text-foreground">
            还没有 Agent
          </h2>
          <p className="mt-2 font-body text-sm text-muted-foreground">
            先创建 Agent，系统会根据其 system prompt 自动划分团队
          </p>
          <button
            type="button"
            className="btn-brutal mt-6"
            onClick={() => setIsCreateModalOpen(true)}
          >
            创建第一个 Agent
          </button>
        </div>
      )}

      {/* Grouped agent cards — List view */}
      {!isLoading && !error && agents.length > 0 && viewMode === 'list' && (
        <div className="space-y-6">
          {GROUP_ORDER.map((group) => {
            const groupAgents = grouped.get(group) ?? [];
            const isCollapsed = collapsedGroups.has(group);

            return (
              <section key={group}>
                {/* Group header — clickable to toggle collapse */}
                <button
                  type="button"
                  onClick={() => toggleGroup(group)}
                  className="mb-3 flex w-full items-center gap-3 text-left"
                  aria-expanded={!isCollapsed}
                  aria-label={`${isCollapsed ? '展开' : '收起'} ${group} 分组`}
                >
                  <h2 className="font-heading font-bold text-lg text-foreground">
                    {group}
                  </h2>
                  <span className="inline-flex items-center justify-center rounded-full border-2 border-black bg-white px-2.5 py-0.5 font-mono text-xs font-bold shadow-brutal-sm">
                    {groupAgents.length}
                  </span>
                  <p className="hidden text-sm text-muted-foreground sm:block">
                    — {GROUP_DESCRIPTIONS[group]}
                  </p>
                  <ChevronDown
                    className={`ml-auto h-5 w-5 text-muted-foreground transition-transform ${
                      isCollapsed ? '' : 'rotate-180'
                    }`}
                  />
                </button>

                {/* Collapsible content */}
                {!isCollapsed && (
                  <>
                    {groupAgents.length > 0 ? (
                      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
                        {groupAgents.map((agent) => (
                          <AgentCard
                            key={agent.id}
                            agent={agent}
                            onClick={(id) => setPanelAgentId(id)}
                            onEdit={(id) => setPanelAgentId(id)}
                            onDelete={(_id) => {
                              const a = groupAgents.find((g) => g.id === _id);
                              if (a) setDeleteTarget(a);
                            }}
                          />
                        ))}
                      </div>
                    ) : (
                      <div className="border-2 border-dashed border-black bg-white p-6 text-center shadow-brutal-sm">
                        <p className="font-body text-sm italic text-muted-foreground">
                          暂无此分组的 Agent
                        </p>
                        <button
                          type="button"
                          onClick={() => setIsCreateModalOpen(true)}
                          className="btn-brutal btn-brutal-sm mt-3"
                        >
                          <Plus className="mr-1 h-3.5 w-3.5" />
                          创建 Agent
                        </button>
                      </div>
                    )}
                  </>
                )}
              </section>
            );
          })}
        </div>
      )}

      {/* Delete confirmation dialog */}
      <Dialog
        open={!!deleteTarget}
        onOpenChange={(opened) => { if (!opened) setDeleteTarget(null); }}
      >
        <DialogHeader>
          <DialogTitle>确认删除</DialogTitle>
          <DialogCloseButton onClick={() => setDeleteTarget(null)} />
        </DialogHeader>
        <div className="mt-4 px-5 pb-5">
          <DialogDescription>
            确定要删除 Agent「{deleteTarget?.name}」吗？此操作不可撤销。
          </DialogDescription>
          <DialogFooter>
            <button
              type="button"
              onClick={() => setDeleteTarget(null)}
              disabled={isDeleting}
              className="btn-brutal btn-brutal-sm"
            >
              取消
            </button>
            <button
              type="button"
              onClick={handleDeleteConfirm}
              disabled={isDeleting}
              className="btn-brutal btn-brutal-sm bg-brutal-red-light"
            >
              {isDeleting ? '删除中...' : '确认删除'}
            </button>
          </DialogFooter>
        </div>
      </Dialog>

      {/* v1.5: Create Agent Modal */}
      <Dialog
        open={isCreateModalOpen}
        onOpenChange={(opened) => {
          if (!opened) {
            setIsCreateModalOpen(false);
            setCreateName('');
            setCreateDescription('');
          }
        }}
      >
        <DialogHeader>
          <DialogTitle>创建 Agent</DialogTitle>
          <DialogCloseButton onClick={() => {
            setIsCreateModalOpen(false);
            setCreateName('');
            setCreateDescription('');
          }} />
        </DialogHeader>
        <div className="mt-4 px-5 pb-5">
          <DialogDescription>
            创建新的 AI Agent，之后可在详情中配置 Runtime、System Prompt 等。
          </DialogDescription>
          <div className="mt-4 space-y-4">
            <div>
              <label htmlFor="create-name" className="block font-heading text-sm font-bold mb-1">
                名称 <span className="text-brutal-red">*</span>
              </label>
              <input
                id="create-name"
                type="text"
                value={createName}
                onChange={(e) => setCreateName(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && !e.shiftKey) {
                    e.preventDefault();
                    handleCreateAgent();
                  }
                }}
                placeholder="例如：代码审查员"
                disabled={isCreating}
                className="input-brutal w-full"
                autoFocus
              />
            </div>
            <div>
              <label htmlFor="create-description" className="block font-heading text-sm font-bold mb-1">
                描述 <span className="text-xs text-muted-foreground font-normal">(可选)</span>
              </label>
              <input
                id="create-description"
                type="text"
                value={createDescription}
                onChange={(e) => setCreateDescription(e.target.value)}
                placeholder="简要描述 Agent 的职责"
                disabled={isCreating}
                className="input-brutal w-full"
              />
            </div>
          </div>
          <DialogFooter>
            <button
              type="button"
              onClick={() => {
                setIsCreateModalOpen(false);
                setCreateName('');
                setCreateDescription('');
              }}
              disabled={isCreating}
              className="btn-brutal btn-brutal-sm"
            >
              取消
            </button>
            <button
              type="button"
              onClick={handleCreateAgent}
              disabled={isCreating || !createName.trim()}
              className="btn-brutal btn-brutal-sm btn-brutal-pink"
            >
              {isCreating ? '创建中...' : '创建 Agent'}
            </button>
          </DialogFooter>
        </div>
      </Dialog>

      {/* Structure view — flow layout with connected cards */}
      {!isLoading && !error && agents.length > 0 && viewMode === 'structure' && (
        <div className="overflow-x-auto">
          <div className="flex flex-col items-center gap-4 px-4 md:flex-row md:items-start md:justify-center md:gap-0 min-w-max py-4">
            {GROUP_ORDER.filter((group) => (grouped.get(group) ?? []).length > 0).map((group, index, filtered) => {
              const groupAgents = grouped.get(group) ?? [];
              const isExpanded = expandedInStructure.has(group);

              const toggleExpand = () => {
                setExpandedInStructure((prev) => {
                  const next = new Set(prev);
                  if (next.has(group)) {
                    next.delete(group);
                  } else {
                    next.add(group);
                  }
                  return next;
                });
              };

              return (
                <div key={group} className="flex flex-col items-center">
                  <div className="flex flex-col items-center md:flex-row">
                    {/* Card */}
                    <div className={cn(
                      'card-brutal w-60 px-4 py-4 transition-all',
                      isExpanded && 'border-2 !border-brutal-pink shadow-brutal',
                    )}>
                      <button
                        type="button"
                        onClick={toggleExpand}
                        className="w-full text-left"
                      >
                        {/* Group name */}
                        <h3 className="font-heading text-sm font-bold text-foreground">
                          {group}
                        </h3>
                        <p className="mt-1 font-mono text-xs text-muted-foreground">
                          {groupAgents.length} agent{groupAgents.length !== 1 ? 's' : ''}
                        </p>

                        {/* Agent mini-list */}
                        <div className="mt-3 space-y-1.5">
                          {(isExpanded ? groupAgents : groupAgents.slice(0, 3)).map((agent) => (
                            <div key={agent.id} className="flex items-center gap-2">
                              <div className="flex h-6 w-6 flex-shrink-0 items-center justify-center border-2 border-black bg-brutal-pink">
                                <Bot className="h-3.5 w-3.5 text-white" />
                              </div>
                              <span className="truncate font-body text-xs">
                                {agent.name}
                              </span>
                            </div>
                          ))}
                          {!isExpanded && groupAgents.length > 3 && (
                            <p className="pl-8 font-mono text-xs text-muted-foreground">
                              +{groupAgents.length - 3} 更多...
                            </p>
                          )}
                        </div>

                        {/* Expand hint */}
                        <div className="mt-3 border-t border-black/10 pt-2">
                          <p className="font-mono text-[10px] text-muted-foreground">
                            {isExpanded ? '点击收起详情' : '点击展开详情'}
                          </p>
                        </div>
                      </button>
                    </div>

                    {/* Arrow connector to next card */}
                    {index < filtered.length - 1 && (
                      <div className="flex items-center justify-center py-2 md:px-3 md:py-0">
                        <ArrowRight className="h-6 w-6 rotate-90 text-brutal-stone md:rotate-0" />
                      </div>
                    )}
                  </div>

                  {/* Expanded: show agent cards below */}
                  {isExpanded && (
                    <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 w-full max-w-2xl">
                      {groupAgents.map((agent) => (
                        <AgentCard
                          key={agent.id}
                          agent={agent}
                          onClick={(id) => setPanelAgentId(id)}
                          onEdit={(id) => setPanelAgentId(id)}
                          onDelete={(_id) => {
                                    const a = groupAgents.find((g) => g.id === _id);
                                    if (a) setDeleteTarget(a);
                                  }}
                        />
                      ))}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </div>
      )}
      </div>

      {/* v1.5: Agent Detail Panel */}
      {panelAgentId && (
        <AgentDetailPanel
          agentId={panelAgentId}
          onClose={() => setPanelAgentId(null)}
        />
      )}
    </AppFrame>
  );
}
