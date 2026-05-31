// ============================================================================
// SOLO-236-F: CMD+K Global Search Panel
// - Full-screen overlay triggered by CMD+K / Ctrl+K
// - Debounced search (300ms) calling GET /api/v1/search?q={query}&limit=20
// - Results show highlighted snippet, channel name, sender, time
// - Keyboard navigation: ArrowUp/ArrowDown/Enter/Escape
// - Click result → navigate to channel + message
// ============================================================================

'use client';

import { useState, useEffect, useRef, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { Search, X, Clock, Hash } from 'lucide-react';
import { apiClient } from '@/lib/api-client';
import { cn } from '@/lib/utils';
import type { SearchResult, SearchResponse } from '@/lib/types';

interface GlobalSearchProps {
  open: boolean;
  onClose: () => void;
}

// ---- Helpers ----

/** Strip all HTML tags except <mark> for safe rendering */
function sanitizeMarkHtml(html: string): string {
  return html
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/&lt;mark&gt;/g, '<mark>')
    .replace(/&lt;\/mark&gt;/g, '</mark>');
}

function formatTime(iso: string): string {
  try {
    const d = new Date(iso);
    return d.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' });
  } catch {
    return '';
  }
}

function formatDate(iso: string): string {
  try {
    const d = new Date(iso);
    const now = new Date();
    const diffMs = now.getTime() - d.getTime();
    const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));
    if (diffDays === 0) return '今天';
    if (diffDays === 1) return '昨天';
    return d.toLocaleDateString('zh-CN', { month: 'short', day: 'numeric' });
  } catch {
    return '';
  }
}

// ---- Component ----

export function GlobalSearch({ open, onClose }: GlobalSearchProps) {
  const router = useRouter();
  const inputRef = useRef<HTMLInputElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const [query, setQuery] = useState('');
  const [results, setResults] = useState<SearchResult[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [activeIndex, setActiveIndex] = useState(-1);

  // Focus input on open
  useEffect(() => {
    if (open) {
      setQuery('');
      setResults([]);
      setError(null);
      setActiveIndex(-1);
      // Small delay to let the overlay render before focusing
      requestAnimationFrame(() => inputRef.current?.focus());
    }
  }, [open]);

  // Debounced search
  useEffect(() => {
    if (debounceRef.current) {
      clearTimeout(debounceRef.current);
    }

    if (!query.trim()) {
      setResults([]);
      setError(null);
      setActiveIndex(-1);
      return;
    }

    setIsLoading(true);
    setError(null);

    debounceRef.current = setTimeout(async () => {
      try {
        const q = query.trim();
        const data = await apiClient.get<SearchResponse>(
          `/api/v1/search?q=${encodeURIComponent(q)}&limit=20`,
        );
        // Backend returns { results, next_cursor, has_more, total_approx }
        const items = data.results ?? [];
        setResults(Array.isArray(items) ? items : []);
        setActiveIndex(items.length > 0 ? 0 : -1);
      } catch (err: unknown) {
        const message =
          err instanceof Error ? err.message : '搜索请求失败';
        setError(message);
        setResults([]);
        setActiveIndex(-1);
      } finally {
        setIsLoading(false);
      }
    }, 300);

    return () => {
      if (debounceRef.current) {
        clearTimeout(debounceRef.current);
      }
    };
  }, [query]);

  // Keyboard navigation
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      switch (e.key) {
        case 'Escape':
          e.preventDefault();
          onClose();
          break;
        case 'ArrowDown':
          e.preventDefault();
          setActiveIndex((prev) =>
            prev < results.length - 1 ? prev + 1 : 0,
          );
          break;
        case 'ArrowUp':
          e.preventDefault();
          setActiveIndex((prev) =>
            prev > 0 ? prev - 1 : results.length - 1,
          );
          break;
        case 'Enter':
          e.preventDefault();
          if (activeIndex >= 0 && activeIndex < results.length) {
            navigateToResult(results[activeIndex]);
          }
          break;
      }
    },
    [results, activeIndex, onClose],
  );

  // Navigate to channel + message
  const navigateToResult = useCallback(
    (result: SearchResult) => {
      onClose();
      // Navigate to dashboard with channel and message params
      const params = new URLSearchParams();
      params.set('channel', result.channel_id);
      params.set('message', result.id);
      router.push(`/dashboard?${params.toString()}`);
    },
    [router, onClose],
  );

  // Click outside to close
  const handleBackdropClick = useCallback(
    (e: React.MouseEvent) => {
      if (e.target === e.currentTarget) {
        onClose();
      }
    },
    [onClose],
  );

  // Scroll active item into view
  const activeItemRef = useRef<HTMLButtonElement | null>(null);
  useEffect(() => {
    activeItemRef.current?.scrollIntoView({ block: 'nearest' });
  }, [activeIndex]);

  if (!open) return null;

  return (
    // eslint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions
    <div
      className="fixed inset-0 z-50 flex items-start justify-center bg-black/50 pt-[15vh]"
      onClick={handleBackdropClick}
      onKeyDown={() => {}} // Suppress keyboard events on backdrop
      role="dialog"
      aria-modal="true"
      aria-label="全局搜索"
    >
      <div
        ref={panelRef}
        className="w-full max-w-2xl border-2 border-black bg-white shadow-brutal-lg"
        role="search"
      >
        {/* Search header */}
        <div className="flex items-center border-b-2 border-black px-4 py-0">
          <Search className="mr-3 h-5 w-5 flex-shrink-0 text-muted-foreground" />
          <input
            ref={inputRef}
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="输入关键词搜索所有消息..."
            className="flex-1 bg-transparent py-3 text-lg font-body outline-none placeholder:text-muted-foreground"
            aria-label="搜索关键词"
            autoComplete="off"
            spellCheck={false}
          />
          <button
            type="button"
            onClick={onClose}
            className="ml-2 flex h-8 w-8 flex-shrink-0 items-center justify-center border-2 border-black bg-white shadow-brutal-sm hover:shadow-brutal active:translate-x-0.5 active:translate-y-0.5 active:shadow-none transition-all"
            aria-label="关闭搜索"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* Results area */}
        <div className="max-h-96 overflow-y-auto">
          {/* Loading */}
          {isLoading && (
            <div className="flex items-center justify-center py-12">
              <div className="h-6 w-6 animate-spin rounded-full border-3 border-brutal-pink border-t-transparent" />
              <span className="ml-3 font-body text-sm text-muted-foreground">
                搜索中...
              </span>
            </div>
          )}

          {/* Error */}
          {error && !isLoading && (
            <div className="p-6 text-center">
              <p className="font-body text-sm text-brutal-red">{error}</p>
            </div>
          )}

          {/* Empty query — show hint */}
          {!isLoading && !error && query.trim() === '' && (
            <div className="flex flex-col items-center py-12 text-muted-foreground">
              <Search className="mb-3 h-8 w-8 opacity-30" />
              <p className="font-body text-sm">
                输入关键词搜索所有频道中的消息
              </p>
              <p className="mt-1 font-mono text-xs text-muted-foreground/60">
                <kbd className="inline-block rounded border border-black/20 bg-muted px-1.5 py-0.5 font-mono text-xs">
                  Esc
                </kbd>{' '}
                关闭搜索
              </p>
            </div>
          )}

          {/* No results */}
          {!isLoading && !error && query.trim() !== '' && results.length === 0 && (
            <div className="flex flex-col items-center py-12 text-muted-foreground">
              <p className="font-body text-sm">未找到匹配的消息</p>
            </div>
          )}

          {/* Results list */}
          {!isLoading && results.length > 0 && (
            <ul role="listbox" aria-label="搜索结果">
              {results.map((result, index) => (
                <li key={result.id} role="option" aria-selected={index === activeIndex}>
                  <button
                    type="button"
                    ref={index === activeIndex ? activeItemRef : undefined}
                    onClick={() => navigateToResult(result)}
                    onMouseEnter={() => setActiveIndex(index)}
                    className={cn(
                      'flex w-full flex-col gap-1 border-b border-black/10 px-4 py-3 text-left transition-colors',
                      index === activeIndex
                        ? 'bg-brutal-pink-light'
                        : 'hover:bg-muted',
                    )}
                  >
                    {/* Meta: channel + sender + time */}
                    <div className="flex items-center gap-2">
                      <Hash className="h-3.5 w-3.5 flex-shrink-0 text-muted-foreground" />
                      <span className="font-heading text-xs font-bold text-foreground">
                        {result.channel_name}
                      </span>
                      <span className="font-mono text-xs text-muted-foreground">
                        · {result.sender_name}
                      </span>
                      <span className="ml-auto flex items-center gap-1 font-mono text-xs text-muted-foreground">
                        <Clock className="h-3 w-3" />
                        {formatDate(result.created_at)} {formatTime(result.created_at)}
                      </span>
                    </div>

                    {/* Content snippet with highlighted marks */}
                    <p
                      className="line-clamp-2 font-body text-sm text-foreground leading-relaxed [&_mark]:bg-brutal-pink [&_mark]:text-foreground [&_mark]:px-0.5"
                      dangerouslySetInnerHTML={{
                        __html: result.highlight
                          ? sanitizeMarkHtml(result.highlight)
                          : result.content,
                      }}
                    />
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>

        {/* Footer with result count */}
        {!isLoading && results.length > 0 && (
          <div className="border-t-2 border-black px-4 py-2">
            <p className="font-mono text-xs text-muted-foreground">
              找到 {results.length} 条结果
            </p>
          </div>
        )}
      </div>
    </div>
  );
}
