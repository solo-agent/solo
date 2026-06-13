// ============================================================================
// KnowledgePanel — channel sidebar panel for knowledge
// - Recent knowledge entries for the current channel
// - Quick search within channel
// - Link to full knowledge search
// ============================================================================

'use client';

import { useState, useEffect, useCallback, useMemo } from 'react';
import { BookOpen, Loader2, ChevronRight, Tag } from 'lucide-react';
import { cn } from '@/lib/utils';
import { t } from '@/lib/i18n';
import { KnowledgeSearch } from './knowledge-search';
import { KnowledgeCreate } from './knowledge-create';
import { KnowledgeDetail } from './knowledge-detail';
import { Button } from '@/components/ui/button';
import { apiClient } from '@/lib/api-client';
import type { KnowledgeEntry } from '@/lib/types';

interface KnowledgePanelProps {
  channelId: string;
  /** Called when a knowledge entry is clicked (for full detail view — if not provided, uses internal modal) */
  onEntryClick?: (entry: KnowledgeEntry) => void;
  /** Compact mode for sidebar usage */
  compact?: boolean;
  /** Available channels for create form (optional) */
  channels?: { value: string; label: string }[];
}

export function KnowledgePanel({ channelId, onEntryClick, compact, channels = [] }: KnowledgePanelProps) {
  const [recent, setRecent] = useState<KnowledgeEntry[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showSearch, setShowSearch] = useState(false);
  const [activeTag, setActiveTag] = useState<string | null>(null);
  const [detailEntry, setDetailEntry] = useState<KnowledgeEntry | null>(null);
  const [detailOpen, setDetailOpen] = useState(false);

  const fetchRecent = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const params: Record<string, string> = { q: '', channel_id: channelId, top_k: '5' };
      if (activeTag) params.tags = activeTag;
      const data = await apiClient.get<{ results: KnowledgeEntry[]; total: number }>(
        '/api/v1/knowledge/search',
        params,
      );
      setRecent(data.results || []);
    } catch {
      // Fallback: try list endpoint
      try {
        const params: Record<string, string> = { channel_id: channelId, limit: '5' };
        if (activeTag) params.tags = activeTag;
        const data = await apiClient.get<{ results: KnowledgeEntry[] }>(
          '/api/v1/knowledge',
          params,
        );
        setRecent(data.results || []);
      } catch {
        setError(t('knowledgeSearchError'));
      }
    } finally {
      setIsLoading(false);
    }
  }, [channelId, activeTag]);

  useEffect(() => {
    fetchRecent();
  }, [fetchRecent]);

  // Extract unique tags from all entries
  const allTags = useMemo(() => {
    const tagSet = new Set<string>();
    recent.forEach((entry) => {
      entry.tags?.forEach((tag) => tagSet.add(tag));
    });
    return Array.from(tagSet).sort();
  }, [recent]);

  const handleEntryClick = (entry: KnowledgeEntry) => {
    if (onEntryClick) {
      onEntryClick(entry);
    } else {
      setDetailEntry(entry);
      setDetailOpen(true);
    }
  };

  const formatDate = (iso?: string) => {
    if (!iso) return '';
    try {
      const d = new Date(iso);
      const now = new Date();
      const diffDays = Math.floor((now.getTime() - d.getTime()) / (1000 * 60 * 60 * 24));
      if (diffDays === 0) return t('today');
      if (diffDays === 1) return t('yesterday');
      if (diffDays < 7) return `${diffDays} ${t('daysAgo', { n: diffDays })}`;
      const pad = (n: number) => String(n).padStart(2, '0');
      return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
    } catch {
      return iso;
    }
  };

  return (
    <div className={cn('space-y-3 p-4', compact && 'p-3')}>
      {/* Header */}
      <div className="flex items-center justify-between">
        <h3 className="font-heading text-xs font-bold uppercase tracking-wider text-muted-foreground">
          <BookOpen className="inline h-3.5 w-3.5 mr-1 -mt-0.5" />
          {t('knowledgeChannelPanelTitle')}
        </h3>
        <KnowledgeCreate
          channelId={channelId}
          channels={channels}
          onCreated={() => fetchRecent()}
        />
      </div>

      {/* Quick search toggle */}
      <div className="flex items-center gap-2">
        {!showSearch && (
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setShowSearch(true)}
            className="text-xs flex-1 justify-start"
          >
            {t('knowledgeQuickSearch')}
          </Button>
        )}
        <Button
          variant="outline"
          size="sm"
          onClick={() => setShowSearch((v) => !v)}
          className="text-xs"
        >
          {showSearch ? t('knowledgeRecentEntries') : t('search')}
        </Button>
      </div>

      {/* Search mode */}
      {showSearch && (
        <KnowledgeSearch
          channelId={channelId}
          onResultClick={handleEntryClick}
          compact
        />
      )}

      {/* Tag filter bar (T4.4.1) */}
      {!showSearch && allTags.length > 0 && (
        <div className="flex items-center gap-1 flex-wrap">
          <Tag className="h-3 w-3 text-muted-foreground flex-shrink-0" />
          {allTags.map((tag) => (
            <button
              key={tag}
              type="button"
              onClick={() => setActiveTag(activeTag === tag ? null : tag)}
              className={cn(
                'px-1.5 py-0.5 font-heading text-[10px] font-bold border-2 border-black transition-all duration-100',
                'active:translate-x-0.5 active:translate-y-0.5',
                activeTag === tag
                  ? 'bg-brutal-accent text-white shadow-brutal-sm'
                  : 'bg-brutal-muted-light hover:bg-brutal-primary-light',
              )}
            >
              {tag}
            </button>
          ))}
          {activeTag && (
            <button
              type="button"
              onClick={() => setActiveTag(null)}
              className="font-mono text-[10px] text-muted-foreground hover:text-black underline px-1"
            >
              {t('clearFilter')}
            </button>
          )}
        </div>
      )}

      {/* Recent entries list */}
      {!showSearch && (
        <>
          {isLoading && (
            <div className="flex items-center justify-center py-4">
              <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
            </div>
          )}

          {error && !isLoading && (
            <div className="border-2 border-black bg-brutal-danger-light px-2 py-1.5">
              <p className="font-mono text-[10px] text-brutal-danger">{error}</p>
              <button
                type="button"
                onClick={fetchRecent}
                className="font-mono text-[10px] font-bold underline"
              >
                {t('retry')}
              </button>
            </div>
          )}

          {!isLoading && !error && recent.length === 0 && (
            <p className="font-mono text-xs text-muted-foreground py-2">
              {t('noResults')}
            </p>
          )}

          {!isLoading && recent.length > 0 && (
            <div className="space-y-1.5">
              {recent.map((entry) => (
                <button
                  key={entry.id}
                  type="button"
                  onClick={() => handleEntryClick(entry)}
                  className={cn(
                    'w-full text-left border-2 border-black bg-white p-2 transition-all',
                    'hover:-translate-y-px hover:shadow-brutal-sm',
                    'active:translate-x-0.5 active:translate-y-0.5 active:shadow-none',
                  )}
                >
                  <div className="flex items-start justify-between gap-1">
                    <h4 className="font-heading text-xs font-bold text-foreground line-clamp-1">
                      {entry.title}
                    </h4>
                    <ChevronRight className="h-3 w-3 text-muted-foreground flex-shrink-0 mt-0.5" />
                  </div>
                  <div className="mt-0.5 flex items-center gap-2 text-[10px] text-muted-foreground">
                    {entry.author_name && (
                      <span>{entry.author_name}</span>
                    )}
                    <span>{formatDate(entry.created_at)}</span>
                  </div>
                </button>
              ))}
            </div>
          )}
        </>
      )}

      {/* Detail modal (T4.4.2) */}
      <KnowledgeDetail
        entry={detailEntry}
        open={detailOpen}
        onOpenChange={setDetailOpen}
        onMutate={fetchRecent}
        channels={channels}
      />
    </div>
  );
}
