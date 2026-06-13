// ============================================================================
// WatchdogPanel — task watchdog status and alerts
// - Show overdue tasks with escalation level
// - Visual indicators: green (on track), yellow (warning), red (escalated)
// - Time since last check + escalation chain
// ============================================================================

'use client';

import { useState, useEffect, useCallback } from 'react';
import {
  Shield, ShieldAlert, ShieldCheck, ShieldX,
  Clock, User, Loader2, ArrowRight, ChevronRight,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { t } from '@/lib/i18n';
import { Button } from '@/components/ui/button';
import { BrutalAlert } from '@/components/ui/brutal-alert';
import { apiClient } from '@/lib/api-client';
import type { WatchdogItem } from '@/lib/types';

interface WatchdogPanelProps {
  /** Filter by channel */
  channelId?: string;
  /** Compact mode */
  compact?: boolean;
}

const LEVEL_CONFIG = {
  green: {
    icon: ShieldCheck,
    color: 'text-brutal-success',
    bg: 'bg-brutal-success-light',
    label: 'watchdogLevelGreen',
  },
  yellow: {
    icon: ShieldAlert,
    color: 'text-brutal-warning',
    bg: 'bg-brutal-warning-light',
    label: 'watchdogLevelYellow',
  },
  red: {
    icon: ShieldX,
    color: 'text-brutal-danger',
    bg: 'bg-brutal-danger-light',
    label: 'watchdogLevelRed',
  },
} as const;

export function WatchdogPanel({ channelId, compact }: WatchdogPanelProps) {
  const [watchdogs, setWatchdogs] = useState<WatchdogItem[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchWatchdogs = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const params: Record<string, string> = {};
      if (channelId) params.channel_id = channelId;
      const data = await apiClient.get<WatchdogItem[]>(
        '/api/v1/tasks/stale',
        params,
      );
      setWatchdogs(data || []);
    } catch (err: unknown) {
      const apiErr = err as { status?: number };
      if (apiErr.status === 404) {
        // No stale tasks — that's fine
        setWatchdogs([]);
      } else {
        setError(t('watchdogLoadError'));
      }
    } finally {
      setIsLoading(false);
    }
  }, [channelId]);

  useEffect(() => {
    fetchWatchdogs();
  }, [fetchWatchdogs]);

  // Auto-refresh every 60s
  useEffect(() => {
    const interval = setInterval(fetchWatchdogs, 60000);
    return () => clearInterval(interval);
  }, [fetchWatchdogs]);

  const formatHoursAgo = (iso?: string) => {
    if (!iso) return '';
    try {
      const diffMs = Date.now() - new Date(iso).getTime();
      const hours = Math.floor(diffMs / (1000 * 60 * 60));
      if (hours < 1) return `< 1h`;
      return t('watchdogHours', { n: hours });
    } catch {
      return '';
    }
  };

  const warningCount = watchdogs.filter((w) => w.escalation_level === 'yellow').length;
  const escalatedCount = watchdogs.filter((w) => w.escalation_level === 'red').length;
  const totalWarnings = warningCount + escalatedCount;

  return (
    <div className={cn('space-y-3', compact && 'space-y-2')}>
      {/* Header */}
      <div className="flex items-center justify-between">
        <h3 className="font-heading text-sm font-bold uppercase tracking-wider text-foreground">
          <Shield className="inline h-4 w-4 mr-1.5 -mt-0.5" />
          {t('watchdogPanelTitle')}
          {totalWarnings > 0 && (
            <span className={cn(
              'ml-2 inline-flex items-center justify-center h-5 min-w-[20px] px-1.5',
              'font-mono text-[10px] font-bold border-2 border-black',
              escalatedCount > 0 ? 'bg-brutal-danger text-white' : 'bg-brutal-warning text-black',
            )}>
              {totalWarnings}
            </span>
          )}
        </h3>
        <Button
          variant="ghost"
          size="sm"
          onClick={fetchWatchdogs}
          className="text-xs"
        >
          {t('refresh')}
        </Button>
      </div>

      {/* Loading */}
      {isLoading && (
        <div className="flex items-center justify-center py-8">
          <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        </div>
      )}

      {/* Error */}
      {error && !isLoading && (
        <BrutalAlert variant="error" className="text-xs">
          {error}
          <button type="button" onClick={fetchWatchdogs} className="ml-2 underline font-bold">
            {t('retry')}
          </button>
        </BrutalAlert>
      )}

      {/* Summary bar */}
      {!isLoading && !error && (
        <div className="flex items-center gap-2">
          <span className={cn(
            'inline-flex items-center gap-1 px-2 py-0.5',
            'font-heading text-[10px] font-bold border-2 border-black',
            LEVEL_CONFIG.green.bg,
            LEVEL_CONFIG.green.color,
          )}>
            <ShieldCheck className="h-3 w-3" />
            {watchdogs.filter((w) => w.escalation_level === 'green').length} {t('watchdogLevelGreen')}
          </span>
          {warningCount > 0 && (
            <span className={cn(
              'inline-flex items-center gap-1 px-2 py-0.5',
              'font-heading text-[10px] font-bold border-2 border-black',
              LEVEL_CONFIG.yellow.bg,
              LEVEL_CONFIG.yellow.color,
            )}>
              <ShieldAlert className="h-3 w-3" />
              {warningCount} {t('watchdogLevelYellow')}
            </span>
          )}
          {escalatedCount > 0 && (
            <span className={cn(
              'inline-flex items-center gap-1 px-2 py-0.5',
              'font-heading text-[10px] font-bold border-2 border-black',
              LEVEL_CONFIG.red.bg,
              LEVEL_CONFIG.red.color,
            )}>
              <ShieldX className="h-3 w-3" />
              {escalatedCount} {t('watchdogLevelRed')}
            </span>
          )}
        </div>
      )}

      {/* No issues */}
      {!isLoading && !error && watchdogs.length === 0 && (
        <div className="border-2 border-black bg-brutal-success-light p-4 text-center">
          <ShieldCheck className="mx-auto h-6 w-6 text-brutal-success mb-1" />
          <p className="font-heading text-xs font-bold text-brutal-success">
            {t('watchdogNoWarnings')}
          </p>
          <p className="mt-0.5 font-mono text-[10px] text-muted-foreground">
            {t('watchdogNoWarningsDesc')}
          </p>
        </div>
      )}

      {/* Watchdog items */}
      {!isLoading && watchdogs.length > 0 && (
        <div className={cn('space-y-2', compact && 'max-h-[50vh] overflow-y-auto')}>
          {watchdogs.map((w) => {
            const level = LEVEL_CONFIG[w.escalation_level] || LEVEL_CONFIG.green;
            const LevelIcon = level.icon;

            return (
              <div
                key={w.task_id}
                className={cn(
                  'border-2 border-black bg-white p-3 shadow-brutal-sm',
                  w.escalation_level === 'red' && 'border-brutal-danger shadow-brutal-danger',
                )}
              >
                {/* Level indicator + task info */}
                <div className="flex items-start gap-2">
                  <LevelIcon className={cn('h-4 w-4 mt-0.5 flex-shrink-0', level.color)} />
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2 flex-wrap">
                      <span className="font-heading text-sm font-bold text-foreground">
                        {w.task_number ? `#${w.task_number}` : ''}
                      </span>
                      <span className="font-body text-sm text-foreground truncate">
                        {w.task_title || w.task_id.slice(0, 8)}
                      </span>
                    </div>

                    {/* Meta */}
                    <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-[10px] text-muted-foreground">
                      <span className="flex items-center gap-0.5">
                        <User className="h-2.5 w-2.5" />
                        {w.claimer_name || w.claimer_id.slice(0, 8)}
                      </span>
                      <span className={cn(
                        'flex items-center gap-0.5',
                        w.escalation_level === 'red' ? 'text-brutal-danger font-bold' : 'text-muted-foreground',
                      )}>
                        <Clock className="h-2.5 w-2.5" />
                        {t('watchdogLastActivity')}: {formatHoursAgo(w.last_activity)}
                      </span>
                      {w.escalation_level === 'red' && w.escalate_to_name && (
                        <span className="flex items-center gap-0.5 text-brutal-danger font-bold">
                          <ArrowRight className="h-2.5 w-2.5" />
                          {t('watchdogEscalateTo')}: {w.escalate_to_name}
                        </span>
                      )}
                    </div>

                    {/* Action bar — compact */}
                    {!compact && (
                      <div className="mt-2 flex items-center gap-1">
                        <Button
                          variant="outline"
                          size="sm"
                          className="h-6 text-[10px]"
                          onClick={() => window.open(`/tasks/${w.task_id}`, '_blank')}
                        >
                          <ChevronRight className="h-2.5 w-2.5 mr-0.5" />
                          {t('viewAgentDetail', { name: '' })}
                        </Button>
                      </div>
                    )}
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
