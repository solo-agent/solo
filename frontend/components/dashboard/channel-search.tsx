// ============================================================================
// ChannelSearch — inline search bar that searches within the current channel
// SOLO-237-F: Debounced search with dropdown results, Escape/click-outside dismiss
// ============================================================================

'use client';

import { useState, useEffect, useRef, useCallback } from 'react';
import { Search, Loader2, X, Hash } from 'lucide-react';
import { apiClient } from '@/lib/api-client';
import { t } from '@/lib/i18n';
import type { SearchResult, SearchResponse } from '@/lib/types';
import { sanitizeHtml } from '@/lib/sanitize';

// ---- Helpers ----

/** Strip all HTML tags except <mark> for safe rendering (consistent with global-search) */
function sanitizeMarkHtml(html: string): string {
  return html
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/&lt;mark&gt;/g, '<mark>')
    .replace(/&lt;\/mark&gt;/g, '</mark>');
}

interface ChannelSearchProps {
  channelId: string;
  channelName: string;
  /** Called when a search result is clicked — parent should scroll to the message */
  onResultClick: (messageId: string) => void;
}

export function ChannelSearch({ channelId, channelName, onResultClick }: ChannelSearchProps) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<SearchResult[]>([]);
  const [searching, setSearching] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const containerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  // ---- Debounced search ----
  useEffect(() => {
    if (!query.trim()) {
      setResults([]);
      setError(null);
      return;
    }

    const timer = setTimeout(async () => {
      setSearching(true);
      setError(null);
      try {
        const res = await apiClient.get<SearchResponse>(
          `/api/v1/search?q=${encodeURIComponent(query.trim())}&channel_id=${channelId}&limit=10`,
        );
        setResults(Array.isArray(res.results) ? res.results : []);
      } catch {
        setError(t('searchError'));
        setResults([]);
      } finally {
        setSearching(false);
      }
    }, 300);

    return () => clearTimeout(timer);
  }, [query, channelId]);

  // ---- Focus input when opened ----
  useEffect(() => {
    if (open && inputRef.current) {
      inputRef.current.focus();
    }
  }, [open]);

  // ---- Click outside handler ----
  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        handleClose();
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [open]);

  // ---- Keyboard: Escape to close ----
  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        handleClose();
      }
    };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [open]);

  const handleClose = useCallback(() => {
    setOpen(false);
    setQuery('');
    setResults([]);
    setError(null);
  }, []);

  const handleOpen = useCallback(() => {
    setOpen(true);
  }, []);

  const handleResultClick = useCallback(
    (messageId: string) => {
      onResultClick(messageId);
      handleClose();
    },
    [onResultClick, handleClose],
  );

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Escape') {
        handleClose();
      }
    },
    [handleClose],
  );

  // Format time for display
  const formatTime = (iso: string) => {
    try {
      return new Date(iso).toLocaleString('zh-CN', {
        hour: '2-digit',
        minute: '2-digit',
      });
    } catch {
      return '';
    }
  };

  return (
    <div ref={containerRef} className="relative flex items-center">
      {/* Search trigger button */}
      {!open ? (
        <button
          type="button"
          onClick={handleOpen}
          className="flex h-8 w-8 items-center justify-center border-2 border-black bg-white shadow-brutal-sm hover:bg-brutal-cream transition-colors"
          aria-label={t('channelSearch', { channel: channelName })}
          title={t('channelSearch', { channel: channelName })}
        >
          <Search className="h-4 w-4 text-muted-foreground" />
        </button>
      ) : (
        /* Search dropdown */
        <div className="absolute right-0 top-full z-50 mt-1 w-80 border-2 border-black bg-white shadow-brutal">
          {/* Search input */}
          <div className="flex items-center gap-2 border-b-2 border-black px-3 py-2">
            <Search className="h-4 w-4 flex-shrink-0 text-muted-foreground" />
            <input
              ref={inputRef}
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder={t('channelSearchPlaceholder', { channel: channelName })}
              className="flex-1 border-none bg-transparent font-body text-sm text-foreground outline-none placeholder:text-muted-foreground"
              aria-label={t('search')}
            />
            {searching && <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />}
            <button
              type="button"
              onClick={handleClose}
              className="flex-shrink-0 p-0.5 hover:bg-brutal-primary-light transition-colors"
              aria-label={t('channelSearchClose')}
            >
              <X className="h-4 w-4 text-muted-foreground" />
            </button>
          </div>

          {/* Results area */}
          <div className="max-h-80 overflow-y-auto">
            {/* Loading state */}
            {searching && results.length === 0 && !error && (
              <div className="flex items-center justify-center py-6">
                <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
                <span className="ml-2 font-body text-sm text-muted-foreground">
                  {t('searchLoading')}
                </span>
              </div>
            )}

            {/* Error state */}
            {error && (
              <div className="px-4 py-4 text-center">
                <p className="font-body text-sm text-brutal-danger">{error}</p>
              </div>
            )}

            {/* Empty state — typed but no results */}
            {!searching && !error && query.trim() && results.length === 0 && (
              <div className="px-4 py-6 text-center">
                <p className="font-body text-sm text-muted-foreground">
                  {t('noResults')}
                </p>
              </div>
            )}

            {/* Empty state — no query yet */}
            {!searching && !error && !query.trim() && (
              <div className="px-4 py-6 text-center">
                <p className="font-body text-sm text-muted-foreground">
                  {t('searchEmpty')}
                </p>
              </div>
            )}

            {/* Results list */}
            {results.length > 0 && (
              <ul className="py-1" role="listbox" aria-label={t('searchResults')}>
                {results.map((result) => (
                  <li key={result.id}>
                    <button
                      type="button"
                      onClick={() => handleResultClick(result.id)}
                      className="w-full border-b-2 border-black px-4 py-3 text-left transition-colors hover:bg-brutal-primary last:border-b-0"
                      role="option"
                      aria-label={t('jumpToMessage', { name: result.sender_name })}
                    >
                      <div className="flex items-center gap-1.5">
                        <span className="font-heading text-sm font-bold text-foreground truncate">
                          {result.sender_name}
                        </span>
                      </div>
                      <div className="mt-0.5">
                        <span
                          className="font-body text-sm text-muted-foreground line-clamp-2 [&_mark]:bg-brutal-primary [&_mark]:text-black [&_mark]:font-bold"
                          dangerouslySetInnerHTML={{ __html: sanitizeHtml(sanitizeMarkHtml(result.highlight || result.content)) }}
                        />
                      </div>
                      <div className="mt-1 flex items-center gap-1.5 font-mono text-[11px] text-muted-foreground">
                        <Hash className="h-3 w-3" />
                        <span>{result.channel_name || channelName}</span>
                        <span className="mx-0.5 opacity-40">·</span>
                        <span>{formatTime(result.created_at)}</span>
                      </div>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>

          {/* Footer */}
          {results.length > 0 && (
            <div className="border-t-2 border-black px-3 py-1.5">
              <p className="font-mono text-[11px] text-muted-foreground">
                {t('searchResultsCount', { n: results.length })}
              </p>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
