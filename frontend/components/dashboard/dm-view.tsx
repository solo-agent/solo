// ============================================================================
// DM View — DM conversation view with neubrutalist styling
// - Header: card-brutal style + online status dot
// - MessageList (already brutalist)
// - System message: "这是你与 {name} 的私信"
// - Messages/Tasks dual tab (v1.2 Phase 2+3)
// - Input: MessageInput (already brutalist)
// - States: loading, error with retry, normal
// ============================================================================

'use client';

import { useState, useMemo, useCallback } from 'react';
import { Bot, User, AlertCircle, RefreshCw, MessageSquare, Circle, ClipboardList, Plus } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useStreamingMessages } from '@/lib/hooks/use-streaming-messages';
import { MessageList } from './message-list';
import { MessageInput } from './message-input';
import { TaskBoard } from '@/components/tasks/task-board';
import {
  Dialog,
  DialogHeader,
  DialogTitle,
  DialogCloseButton,
} from '@/components/ui/dialog';
import type { DMChannel, ChannelMember, Message, Task, TaskStatus } from '@/lib/types';

interface DMViewProps {
  dm: DMChannel;
  messages: Message[];
  isLoading: boolean;
  error: string | null;
  sendMessage: (content: string, mentionedAgentIds?: string[], asTask?: boolean, attachmentIds?: string[]) => Promise<{ id: string; task_number?: number } | null>;
  retryMessage: (messageId: string, content: string) => Promise<void>;
  cancelMessage?: (messageId: string) => void;
  editMessage?: (messageId: string, content: string) => Promise<void>;
  deleteMessage?: (messageId: string) => Promise<void>;
  onAsTask?: (message: Message) => void;
  hasMore: boolean;
  isLoadingMore: boolean;
  loadMoreError: string | null;
  loadMore: () => Promise<void>;
  refetch: () => void;
  // ---- DM Tasks (v1.2 Phase 2+3) ----
  tasks?: Task[];
  tasksLoading?: boolean;
  tasksError?: string | null;
  refetchTasks?: () => void;
  onTaskStatusChange?: (task: Task, newStatus: TaskStatus) => Promise<void>;
  onClaimTask?: (task: Task) => void;
  onUnclaimTask?: (task: Task) => void;
  onCreateTask?: (title: string) => Promise<void>;
}

// ---- Helpers ----

function getDisplayName(dm: DMChannel): string {
  if (dm.other_user) return dm.other_user.display_name;
  if (dm.other_agent) return dm.other_agent.name;
  return '未知用户';
}

function isAgentDM(dm: DMChannel): boolean {
  return !!dm.other_agent;
}

export function DMView({
  dm,
  messages,
  isLoading,
  error,
  sendMessage,
  retryMessage,
  cancelMessage,
  editMessage,
  deleteMessage,
  onAsTask,
  hasMore,
  isLoadingMore,
  loadMoreError,
  loadMore,
  refetch,
  tasks,
  tasksLoading,
  tasksError,
  refetchTasks,
  onTaskStatusChange,
  onClaimTask,
  onUnclaimTask,
  onCreateTask,
}: DMViewProps) {
  const name = getDisplayName(dm);
  const isAgent = isAgentDM(dm);
  const [viewTab, setViewTab] = useState<'messages' | 'tasks'>('messages');
  const [isCreateTaskOpen, setIsCreateTaskOpen] = useState(false);
  const [createTaskTitle, setCreateTaskTitle] = useState('');
  const [isCreatingTask, setIsCreatingTask] = useState(false);

  const { activeStreamingAgentIds } = useStreamingMessages(dm.id);

  const members = useMemo<ChannelMember[]>(() => {
    const result: ChannelMember[] = [
      {
        channel_id: dm.id,
        member_type: 'user',
        member_id: 'user-1',
        role: 'member',
        display_name: '我',
        status: 'online',
      },
    ];
    if (isAgent && dm.other_agent) {
      result.push({
        channel_id: dm.id,
        member_type: 'agent',
        member_id: dm.other_agent.id,
        role: 'member',
        display_name: dm.other_agent.name,
        status: 'online',
      });
    } else if (dm.other_user) {
      result.push({
        channel_id: dm.id,
        member_type: 'user',
        member_id: dm.other_user.id,
        role: 'member',
        display_name: dm.other_user.display_name,
        status: 'online',
      });
    }
    return result;
  }, [dm, isAgent]);

  const agentActivities = useMemo(() => {
    if (!isAgent) return [];
    return activeStreamingAgentIds.map((id) => {
      const agentName = dm.other_agent?.name ?? id;
      return { agentId: id, name: agentName, state: 'streaming' as const };
    });
  }, [isAgent, activeStreamingAgentIds, dm.other_agent?.name]);

  const handleRetry = useCallback(() => {
    refetch();
  }, [refetch]);

  return (
    <div className="flex flex-1 overflow-hidden">
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        {/* Header — card-brutal style */}
        <div className="card-brutal mx-4 mt-4 p-4">
          <div className="flex items-center gap-3">
            {/* Icon */}
            <div className={`
              flex h-10 w-10 flex-shrink-0 items-center justify-center border-2 border-black
              ${isAgent ? 'bg-brutal-lime' : 'bg-brutal-cyan'}
            `}>
              {isAgent ? (
                <Bot className="h-5 w-5 text-black" />
              ) : (
                <User className="h-5 w-5 text-black" />
              )}
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <h2 className="font-heading font-bold text-foreground">
                  {name}
                </h2>
                {/* Online status dot */}
                <Circle className="h-2.5 w-2.5 fill-brutal-lime text-brutal-lime" />
              </div>
              <span className="font-mono text-[11px] text-muted-foreground">
                {isAgent ? 'AI Agent' : '用户'}
              </span>
            </div>
            {/* Tab bar (v1.2 Phase 2+3) */}
            <div className="flex items-center gap-1 flex-shrink-0">
              <button
                type="button"
                onClick={() => setViewTab('messages')}
                className={cn(
                  'flex items-center gap-1.5 px-3 py-1.5 text-xs font-heading font-bold transition-colors',
                  viewTab === 'messages'
                    ? 'bg-brutal-pink text-black border-2 border-black'
                    : 'text-muted-foreground hover:text-foreground border-2 border-transparent',
                )}
              >
                <MessageSquare className="h-3.5 w-3.5" />
                消息
              </button>
              <button
                type="button"
                onClick={() => setViewTab('tasks')}
                className={cn(
                  'flex items-center gap-1.5 px-3 py-1.5 text-xs font-heading font-bold transition-colors',
                  viewTab === 'tasks'
                    ? 'bg-brutal-pink text-black border-2 border-black'
                    : 'text-muted-foreground hover:text-foreground border-2 border-transparent',
                )}
              >
                <ClipboardList className="h-3.5 w-3.5" />
                任务
              </button>
            </div>
          </div>
        </div>

        {/* Messages tab */}
        {viewTab === 'messages' && (
          <div className="flex flex-1 flex-col overflow-hidden bg-brutal-cream">
            {/* System message banner */}
            <div className="flex-shrink-0 border-b-2 border-black bg-brutal-cream px-6 py-3">
              <div className="flex items-center gap-2 font-body text-sm text-muted-foreground">
                <MessageSquare className="h-4 w-4" />
                <span>
                  这是你与 <strong className="text-foreground">{name}</strong> 的私信
                </span>
              </div>
            </div>

            {/* Error state */}
            {error && !isLoading && (
              <div className="flex flex-1 items-center justify-center">
                <div className="text-center space-y-3">
                  <AlertCircle className="mx-auto h-8 w-8 text-brutal-red" />
                  <p className="font-mono text-sm text-brutal-red">{error}</p>
                  <button
                    type="button"
                    onClick={handleRetry}
                    className="btn-brutal btn-brutal-sm gap-2"
                  >
                    <RefreshCw className="h-4 w-4" />
                    重试
                  </button>
                </div>
              </div>
            )}

            {/* Messages */}
            {(!error || isLoading) && (
              <MessageList
                messages={messages}
                isLoading={isLoading}
                error={error}
                onRetry={(id, content) => retryMessage(id, content)}
                onCancel={cancelMessage}
                onEdit={editMessage}
                onDelete={deleteMessage}
                onAsTask={onAsTask}
                hasMore={hasMore}
                isLoadingMore={isLoadingMore}
                loadMoreError={loadMoreError}
                onLoadMore={loadMore}
                agentActivities={agentActivities}
              />
            )}

            {/* Input */}
            <MessageInput
              onSend={(content, mentionedAgentIds, asTask, _taskTitle, attachmentIds) =>
                sendMessage(content, mentionedAgentIds, asTask, attachmentIds)
              }
              members={members}
              placeholder={`发送消息给 ${name}...`}
            />
          </div>
        )}

        {/* Tasks tab (v1.2 Phase 2+3) */}
        {viewTab === 'tasks' && (
          <div className="flex flex-1 flex-col overflow-hidden">
            <div className="flex-shrink-0 border-b-2 border-black bg-brutal-cream px-6 py-3">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2 font-body text-sm text-muted-foreground">
                  <ClipboardList className="h-4 w-4" />
                  <span>
                    与 <strong className="text-foreground">{name}</strong> 的任务
                  </span>
                </div>
                {onCreateTask && (
                  <button
                    type="button"
                    onClick={() => {
                      setCreateTaskTitle('');
                      setIsCreateTaskOpen(true);
                    }}
                    className="btn-brutal btn-brutal-sm flex items-center gap-1"
                  >
                    <Plus className="h-3.5 w-3.5" />
                    快速创建
                  </button>
                )}
              </div>
            </div>
            <div className="flex-1 overflow-y-auto px-4 py-4">
              <TaskBoard
                tasks={tasks ?? []}
                isLoading={tasksLoading ?? false}
                error={tasksError ?? null}
                onTaskClick={() => {}}
                onStatusChange={(task, newStatus) => onTaskStatusChange?.(task, newStatus) ?? Promise.resolve()}
                onRefetch={refetchTasks ?? (() => {})}
                onClaim={onClaimTask}
                onUnclaim={onUnclaimTask}
              />
            </div>
          </div>
        )}
      </div>

      {/* Quick-create task dialog (SOLO-231-F) */}
      <Dialog
        open={isCreateTaskOpen}
        onOpenChange={setIsCreateTaskOpen}
      >
        <DialogHeader>
          <DialogTitle>快速创建任务</DialogTitle>
          <DialogCloseButton onClick={() => setIsCreateTaskOpen(false)} />
        </DialogHeader>
        <div className="mt-4 px-5 pb-5">
          <label
            htmlFor="dm-create-task-title"
            className="mb-2 block font-heading text-sm font-bold text-foreground"
          >
            任务标题
          </label>
          <input
            id="dm-create-task-title"
            type="text"
            value={createTaskTitle}
            onChange={(e) => setCreateTaskTitle(e.target.value)}
            onKeyDown={async (e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                if (!createTaskTitle.trim() || isCreatingTask) return;
                setIsCreatingTask(true);
                try {
                  await onCreateTask?.(createTaskTitle.trim());
                  setIsCreateTaskOpen(false);
                } finally {
                  setIsCreatingTask(false);
                }
              }
            }}
            placeholder={`为与 ${name} 的对话创建任务...`}
            disabled={isCreatingTask}
            className="input-brutal w-full"
          />
          <div className="mt-4 flex justify-end gap-2">
            <button
              type="button"
              onClick={() => setIsCreateTaskOpen(false)}
              disabled={isCreatingTask}
              className="btn-brutal btn-brutal-sm"
            >
              取消
            </button>
            <button
              type="button"
              onClick={async () => {
                if (!createTaskTitle.trim() || isCreatingTask) return;
                setIsCreatingTask(true);
                try {
                  await onCreateTask?.(createTaskTitle.trim());
                  setIsCreateTaskOpen(false);
                } finally {
                  setIsCreatingTask(false);
                }
              }}
              disabled={isCreatingTask || !createTaskTitle.trim()}
              className="btn-brutal btn-brutal-sm btn-brutal-pink"
            >
              {isCreatingTask ? '创建中...' : '创建任务'}
            </button>
          </div>
        </div>
      </Dialog>
    </div>
  );
}
