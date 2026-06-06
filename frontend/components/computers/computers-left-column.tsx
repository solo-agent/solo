// ============================================================================
// ComputersLeftColumn — 220px-wide left navigation column on /computers.
// - Static "Computers" label at the top (matches Sidebar / Tasks / Teams).
// - All Computers section: collapsible, default expanded. Header has
//   chevron + UPPERCASE name + plain count (no badge, no icon — the
//   chevron is the marker).
// - Each item shows a small status dot (green online / gray pulsing offline)
//   + computer name. The status dot is functional (not decorative) so it
//   stays. Click emits onComputerClick.
// - Selection + data are owned by the parent. Expand/collapse is the only
//   internal state.
// ============================================================================

'use client';

import { useState, useCallback } from 'react';
import { ChevronDown, AlertCircle, RefreshCw } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { Computer } from '@/lib/types';

interface ComputersLeftColumnProps {
  computers: Computer[];
  isLoading: boolean;
  error: string | null;
  onRetry: () => void;
  selectedComputerId: string | null;
  onComputerClick: (computerId: string) => void;
}

type SectionKey = 'all';

const SECTION_HEADER =
  'flex w-full items-center gap-1.5 px-3 py-2 text-left text-xs font-bold uppercase tracking-wider font-heading text-muted-foreground hover:bg-brutal-pink/40';
const SECTION_COUNT = 'ml-auto text-xs tabular-nums opacity-50';

export function ComputersLeftColumn({
  computers,
  isLoading,
  error,
  onRetry,
  selectedComputerId,
  onComputerClick,
}: ComputersLeftColumnProps) {
  // Default: section expanded
  const [expanded, setExpanded] = useState<Set<SectionKey>>(
    () => new Set<SectionKey>(['all']),
  );

  const toggle = useCallback((key: SectionKey) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }, []);

  const isAllExpanded = expanded.has('all');

  return (
    <div className="flex h-full flex-col overflow-hidden border-r-2 border-black bg-brutal-cream">
      {/* Page label — matches Sidebar / Tasks / Teams top label style */}
      <div className="border-b-2 border-black px-4 py-3">
        <span className="font-heading text-lg font-bold">Computers</span>
      </div>

      {/* Section */}
      <div className="flex-1 overflow-y-auto py-2">
        <button
          type="button"
          onClick={() => toggle('all')}
          className={SECTION_HEADER}
          aria-label="展开或折叠 电脑列表"
          aria-expanded={isAllExpanded}
        >
          <ChevronDown
            aria-hidden="true"
            className={cn(
              'h-3 w-3 transition-transform',
              isAllExpanded ? 'rotate-0' : '-rotate-90',
            )}
          />
          <span>All Computers</span>
          <span className={SECTION_COUNT}>{computers.length}</span>
        </button>
        {isAllExpanded && (
          <div>
            {isLoading && computers.length === 0 ? (
              <p className="px-6 py-2 font-mono text-[10px] italic text-muted-foreground">
                加载中...
              </p>
            ) : error ? (
              <div className="flex flex-col items-center gap-2 px-3 py-3">
                <div className="flex items-center gap-1.5 text-brutal-red">
                  <AlertCircle className="h-4 w-4 flex-shrink-0" />
                  <span className="font-body text-xs">{error}</span>
                </div>
                <button
                  type="button"
                  onClick={onRetry}
                  className="btn-brutal btn-brutal-sm"
                >
                  <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
                  重试
                </button>
              </div>
            ) : computers.length === 0 ? (
              <p className="px-6 py-2 font-mono text-[10px] italic text-muted-foreground">
                暂无电脑
              </p>
            ) : (
              computers.map((computer) => {
                const isOnline = computer.status === 'online';
                return (
                  <button
                    key={computer.id}
                    type="button"
                    onClick={() => onComputerClick(computer.id)}
                    className={cn(
                      'flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm border-2',
                      computer.id === selectedComputerId
                        ? 'border-black bg-brutal-pink text-black shadow-brutal-sm'
                        : 'border-transparent hover:bg-brutal-pink/60',
                    )}
                    aria-current={computer.id === selectedComputerId ? 'true' : undefined}
                  >
                    <span
                      className={cn(
                        'inline-block h-2 w-2 flex-shrink-0 rounded-full border border-black',
                        isOnline ? 'bg-green-500' : 'bg-gray-400 animate-pulse',
                      )}
                      aria-hidden="true"
                    />
                    <span className="truncate font-body">{computer.name}</span>
                  </button>
                );
              })
            )}
          </div>
        )}
      </div>
    </div>
  );
}
