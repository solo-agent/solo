// ============================================================================
// HistoryTab — Agent execution history list
// - card-brutal style list items
// - Timestamp + channel + status badge + duration
// - Empty state: Bot icon + "暂无执行记录"
// - Loading state: skeleton rows
// - All neubrutalism, zero rounding
// ============================================================================

'use client';

import { Bot, Clock, Hash } from 'lucide-react';
import { cn } from '@/lib/utils';

// ---- Types ----

export interface ExecutionRecord {
  id: string;
  /** The channel or DM where the agent was invoked */
  channel_name: string;
  status: 'success' | 'failed' | 'running';
  /** Execution duration in milliseconds */
  duration_ms: number;
  /** ISO timestamp of when execution started */
  started_at: string;
}

interface HistoryTabProps {
  records?: ExecutionRecord[];
  isLoading?: boolean;
  error?: string | null;
  onRetry?: () => void;
}

// ---- Helpers ----

function formatTime(iso: string): string {
  try {
    const d = new Date(iso);
    const pad = (n: number) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
  } catch {
    return iso;
  }
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
  const min = Math.floor(ms / 60000);
  const sec = Math.round((ms % 60000) / 1000);
  return `${min}m ${sec}s`;
}

const STATUS_BADGE: Record<
  ExecutionRecord['status'],
  { label: string; className: string }
> = {
  success: {
    label: '成功',
    className: 'bg-brutal-lime text-black',
  },
  failed: {
    label: '失败',
    className: 'bg-brutal-red text-white',
  },
  running: {
    label: '运行中',
    className: 'bg-brutal-pink text-black',
  },
};

// ---- Skeleton row ----

function SkeletonRow() {
  return (
    <div className="flex items-center gap-4 border-b-2 border-brutal-muted px-5 py-4 last:border-b-0">
      <div className="h-3 w-16 animate-pulse bg-muted" />
      <div className="h-3 w-24 animate-pulse bg-muted" />
      <div className="h-5 w-12 animate-pulse bg-muted" />
      <div className="ml-auto h-3 w-10 animate-pulse bg-muted" />
    </div>
  );
}

// ---- Component ----

export function HistoryTab({
  records = [],
  isLoading = false,
  error = null,
  onRetry,
}: HistoryTabProps) {
  // Loading state
  if (isLoading) {
    return (
      <div className="card-brutal divide-y-2 divide-black overflow-hidden">
        <div className="flex items-center gap-2 border-b-2 border-black bg-black/5 px-5 py-3">
          <Clock className="h-4 w-4" />
          <span className="font-heading text-sm font-bold">执行记录</span>
        </div>
        {[1, 2, 3, 4, 5].map((i) => (
          <SkeletonRow key={i} />
        ))}
      </div>
    );
  }

  // Error state
  if (error) {
    return (
      <div className="card-brutal p-6">
        <div className="flex items-center gap-3 border-2 border-brutal-red bg-brutal-red-light p-4">
          <span className="font-body text-sm text-foreground">{error}</span>
          {onRetry && (
            <button
              type="button"
              onClick={onRetry}
              className="btn-brutal btn-brutal-sm ml-auto"
            >
              重试
            </button>
          )}
        </div>
      </div>
    );
  }

  // Empty state
  if (records.length === 0) {
    return (
      <div className="card-brutal p-8">
        <div className="flex flex-col items-center justify-center py-12">
          <div className="mb-4 flex h-14 w-14 items-center justify-center border-2 border-black bg-brutal-pink shadow-brutal-sm">
            <Bot className="h-7 w-7 text-white" />
          </div>
          <h3 className="font-heading font-bold text-base text-foreground">
            暂无执行记录
          </h3>
          <p className="mt-1.5 font-body text-sm text-muted-foreground">
            在频道中 @提及该 Agent 或将 Agent 加入频道即可触发执行
          </p>
        </div>
      </div>
    );
  }

  // Records list
  return (
    <div className="card-brutal overflow-hidden">
      {/* Header */}
      <div className="flex items-center gap-2 border-b-2 border-black bg-black/5 px-5 py-3">
        <Clock className="h-4 w-4" />
        <span className="font-heading text-sm font-bold">
          执行记录 ({records.length})
        </span>
      </div>

      {/* List */}
      <div className="divide-y-2 divide-black">
        {records.map((record) => {
          const badge = STATUS_BADGE[record.status];
          return (
            <div
              key={record.id}
              className="flex items-center gap-4 px-5 py-3.5 transition-colors hover:bg-black/[0.02]"
            >
              {/* Timestamp */}
              <span className="min-w-[120px] font-mono text-[11px] text-muted-foreground">
                {formatTime(record.started_at)}
              </span>

              {/* Channel */}
              <div className="flex items-center gap-1.5 min-w-0 flex-1">
                <Hash className="h-3 w-3 flex-shrink-0 text-muted-foreground" />
                <span className="truncate font-body text-sm text-foreground">
                  {record.channel_name}
                </span>
              </div>

              {/* Status badge */}
              <span
                className={cn(
                  'badge-brutal min-w-[48px] text-center',
                  badge.className,
                )}
              >
                {badge.label}
              </span>

              {/* Duration */}
              <span className="min-w-[56px] text-right font-mono text-[11px] text-muted-foreground">
                {formatDuration(record.duration_ms)}
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
