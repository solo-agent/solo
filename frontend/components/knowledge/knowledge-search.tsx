// ============================================================================
// KnowledgeSearch — semantic search over knowledge base
// - Search bar with debounced API calls
// - Filter by channel
// - Result cards with title, snippet, author, date, similarity
// ============================================================================

'use client';

import { useState, useCallback, useRef, useEffect } from 'react';
import { Search, X, Filter, Loader2, BookOpen, User, Calendar, Tag } from 'lucide-react';
import { cn } from '@/lib/utils';
import { t } from '@/lib/i18n';
import { Input } from '@/components/ui/input';
import { Select, type SelectOption } from '@/components/ui/select';
import { Button } from '@/components/ui/button';
import { apiClient } from '@/lib/api-client';
import type { KnowledgeEntry, KnowledgeSearchResponse } from '@/lib/types';

interface KnowledgeSearchProps {
  /** Pre-filter to a specific channel */
  channelId?: string;
  /** Called when a result is clicked */
  onResultClick?: (entry: KnowledgeEntry) => void;
  /** Available channels for filter (optional) */
  channels?: SelectOption[];
  /** Compact mode for sidebar panels */
  compact?: boolean;
}

export function KnowledgeSearch({ channelId, onResultClick, channels, compact }: KnowledgeSearchProps) {
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<KnowledgeEntry[]>([]);
  const [totalResults, setTotalResults] = useState(0);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [filterChannelId, setFilterChannelId] = useState<string>(channelId || '');
  const [hasSearched, setHasSearched] = useState(false);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const searchIdRef = useRef(0);

  const doSearch = useCallback(async (q: string, chId: string) => {
    if (!q.trim()) {
      setResults([]);
      setTotalResults(0);
      setHasSearched(false);
      setError(null);
      return;
    }

    const thisSearchId = ++searchIdRef.current;
    setIsLoading(true);
    setError(null);

    try {
      const params: Record<string, string> = { q: q.trim(), top_k: '10' };
      if (chId) params.channel_id = chId;

      const data = await apiClient.get<KnowledgeSearchResponse>(
        '/api/v1/knowledge/search',
        params,
      );

      // Stale response guard
      if (thisSearchId !== searchIdRef.current) return;

      setResults(data.results || []);
      setTotalResults(data.total || 0);
      setHasSearched(true);
    } catch {
      if (thisSearchId !== searchIdRef.current) return;
      setError(t('knowledgeSearchError'));
    } finally {
      if (thisSearchId === searchIdRef.current) {
        setIsLoading(false);
      }
    }
  }, []);

  const handleQueryChange = useCallback(
    (value: string) => {
      setQuery(value);
      if (debounceRef.current) clearTimeout(debounceRef.current);
      debounceRef.current = setTimeout(() => {
        doSearch(value, filterChannelId);
      }, 300);
    },
    [filterChannelId, doSearch],
  );

  // Re-search when channel filter changes
  useEffect(() => {
    if (query.trim()) {
      doSearch(query, filterChannelId);
    }
  }, [filterChannelId]); // eslint-disable-line react-hooks/exhaustive-deps

  const handleClear = () => {
    setQuery('');
    setResults([]);
    setTotalResults(0);
    setHasSearched(false);
    setError(null);
  };

  const formatDate = (iso?: string) => {
    if (!iso) return '';
    try {
      const d = new Date(iso);
      const pad = (n: number) => String(n).padStart(2, '0');
      return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
    } catch {
      return iso;
    }
  };

  const formatSimilarity = (sim?: number) => {
    if (sim === undefined || sim === null) return '';
    return t('knowledgeSimilarity', { pct: Math.round(sim * 100) });
  };

  const resultCard = (entry: KnowledgeEntry) => (
    <div
      key={entry.id}
      role="button"
      tabIndex={0}
      onClick={() => onResultClick?.(entry)}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onResultClick?.(entry);
        }
      }}
      className={cn(
        'w-full text-left border-2 border-black bg-white p-3 transition-all cursor-pointer',
        'hover:-translate-y-0.5 hover:shadow-brutal-sm active:translate-x-0.5 active:translate-y-0.5 active:shadow-none',
        compact ? 'shadow-brutal-sm' : 'shadow-brutal',
      )}
    >
      {/* Title + similarity */}
      <div className="flex items-start justify-between gap-2">
        <h4 className="font-heading text-sm font-bold text-foreground leading-snug line-clamp-1">
          <BookOpen className="inline h-3.5 w-3.5 mr-1 -mt-0.5" />
          {entry.title}
        </h4>
        {entry.similarity !== undefined && (
          <span className="flex-shrink-0 font-mono text-[10px] font-bold text-brutal-info bg-brutal-info-light px-1.5 py-0.5 border border-black">
            {formatSimilarity(entry.similarity)}
          </span>
        )}
      </div>

      {/* Content preview */}
      {entry.content_preview ? (
        <p className="mt-1 font-body text-xs text-muted-foreground line-clamp-2">
          {entry.content_preview}
        </p>
      ) : entry.content ? (
        <p className="mt-1 font-body text-xs text-muted-foreground line-clamp-2">
          {entry.content.slice(0, 200)}
        </p>
      ) : null}

      {/* Meta row */}
      <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-[10px] text-muted-foreground">
        {entry.channel_name && (
          <span className="flex items-center gap-0.5 font-mono">
            <span className="font-bold">#</span>
            {entry.channel_name}
          </span>
        )}
        {entry.author_name && (
          <span className="flex items-center gap-1">
            <User className="h-2.5 w-2.5" />
            {entry.author_name}
          </span>
        )}
        {entry.created_at && (
          <span className="flex items-center gap-1">
            <Calendar className="h-2.5 w-2.5" />
            {formatDate(entry.created_at)}
          </span>
        )}
        {entry.tags && entry.tags.length > 0 && (
          <span className="flex items-center gap-1">
            <Tag className="h-2.5 w-2.5" />
            {entry.tags.slice(0, 3).join(', ')}
          </span>
        )}
      </div>
    </div>
  );

  return (
    <div className={cn('space-y-3', compact && 'space-y-2')}>
      {/* Search bar */}
      <div className="flex items-center gap-2">
        <div className="relative flex-1">
          <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground pointer-events-none" />
          <Input
            value={query}
            onChange={(e) => handleQueryChange(e.target.value)}
            placeholder={t('knowledgeSearchPlaceholder')}
            className="pl-8 pr-8"
          />
          {query && (
            <button
              type="button"
              onClick={handleClear}
              className="absolute right-2.5 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground hover:text-black"
              aria-label={t('clear')}
            >
              <X className="h-4 w-4" />
            </button>
          )}
        </div>

        {channels && channels.length > 0 && !channelId && !compact && (
          <Select
            options={[{ value: '', label: t('all') }, ...channels]}
            value={filterChannelId}
            onChange={setFilterChannelId}
            placeholder={t('knowledgeFilterChannel')}
            size="sm"
            className="w-36"
            aria-label={t('knowledgeFilterChannel')}
          />
        )}
      </div>

      {/* Loading */}
      {isLoading && (
        <div className="flex items-center justify-center py-8">
          <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        </div>
      )}

      {/* Error */}
      {error && !isLoading && (
        <div className="border-2 border-black bg-brutal-danger-light px-3 py-2">
          <p className="font-mono text-xs text-brutal-danger">{error}</p>
        </div>
      )}

      {/* Empty state */}
      {!isLoading && !error && !hasSearched && !query && (
        <div className="flex flex-col items-center justify-center py-8 text-center">
          <Search className="h-8 w-8 text-muted-foreground mb-2" />
          <p className="font-mono text-xs text-muted-foreground">
            {t('knowledgeSearchEmpty')}
          </p>
        </div>
      )}

      {/* No results */}
      {!isLoading && !error && hasSearched && results.length === 0 && (
        <div className="flex flex-col items-center justify-center py-8 text-center">
          <BookOpen className="h-8 w-8 text-muted-foreground mb-2" />
          <p className="font-heading text-sm font-bold text-foreground">
            {t('knowledgeNoResults')}
          </p>
        </div>
      )}

      {/* Results */}
      {!isLoading && results.length > 0 && (
        <>
          {!compact && (
            <p className="font-mono text-[10px] text-muted-foreground">
              {t('knowledgeSearchResultsCount', { n: totalResults })}
            </p>
          )}
          <div className={cn('space-y-2', compact && 'max-h-[40vh] overflow-y-auto')}>
            {results.map(resultCard)}
          </div>
        </>
      )}
    </div>
  );
}
