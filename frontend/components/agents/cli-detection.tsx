// ============================================================================
// CliDetection — shows which provider CLIs are installed on the server
// Green dot = available, gray dot = not installed (with install hint)
// ============================================================================

'use client';

import { AlertTriangle, Loader2 } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useCliDetection } from '@/lib/hooks/use-cli-detection';

interface CliDetectionProps {
  /** If provided, highlight this runtime as "currently selected" */
  selectedRuntime?: string;
  className?: string;
}

export function CliDetection({ selectedRuntime, className }: CliDetectionProps) {
  const { results, isLoading, isLoaded, error } = useCliDetection();

  // Show nothing during initial load
  if (isLoading && !isLoaded) {
    return (
      <div className={cn('flex items-center gap-1.5 text-muted-foreground', className)}>
        <Loader2 className="h-3 w-3 animate-spin" />
        <span className="font-mono text-[11px]">检测 CLI 安装状态...</span>
      </div>
    );
  }

  // If error, show a quiet warning
  if (error) {
    return (
      <div className={cn('flex items-center gap-1.5 text-brutal-muted', className)}>
        <AlertTriangle className="h-3 w-3" />
        <span className="font-mono text-[11px]">无法检测 CLI 状态</span>
      </div>
    );
  }

  return (
    <div className={cn('space-y-1', className)}>
      {Object.values(results).map((item) => {
        const isSelected = selectedRuntime === item.type;
        const available = item.available;

        return (
          <div
            key={item.type}
            className={cn(
              'flex items-center gap-2 px-2 py-1',
              'border-2 border-black',
              isSelected ? 'bg-brutal-primary-light' : 'bg-white',
            )}
            style={isSelected ? { background: '#fffaef' } : undefined}
          >
            {/* Status dot */}
            <span
              className={cn(
                'inline-block h-2 w-2 flex-shrink-0 rounded-full',
                available ? 'bg-brutal-success' : 'bg-brutal-muted',
              )}
            />

            {/* Label */}
            <span className="font-mono text-[11px] font-bold flex-1">
              {item.display_name}
            </span>

            {/* Status text */}
            <span
              className={cn(
                'font-mono text-[10px] font-bold',
                available ? 'text-brutal-success' : 'text-muted-foreground',
              )}
            >
              {available ? 'Available' : 'Not installed'}
            </span>

            {/* Error / hint when not available */}
            {!available && item.error && (
              <span className="font-mono text-[10px] text-muted-foreground truncate max-w-[160px] hidden sm:inline">
                {item.error}
              </span>
            )}
          </div>
        );
      })}
    </div>
  );
}
