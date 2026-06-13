// ============================================================================
// ChannelMemoryPanel — Show/edit CHANNEL.md + decisions.md (Step 2)
// - Displays channel shared memory for the current channel
// - Edit CHANNEL.md with save functionality
// - View decisions.md (read-only append list)
// - Positioned as a collapsible panel in the channel view
// ============================================================================

'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { Loader2, FileText, ListChecks, Save, Plus, ChevronDown, ChevronUp } from 'lucide-react';
import { cn } from '@/lib/utils';
import { apiClient, ApiError } from '@/lib/api-client';
import { t } from '@/lib/i18n';

// ---- Types ----

interface MemoryState {
  content: string;
  updatedAt?: string;
}

interface Decision {
  content: string;
  created_at?: string;
  created_by_name?: string;
}

// ---- Panel component ----

interface ChannelMemoryPanelProps {
  channelId: string;
  /** Whether the panel is expanded */
  expanded?: boolean;
  /** Toggle callback */
  onToggle?: () => void;
}

export function ChannelMemoryPanel({ channelId, expanded = false, onToggle }: ChannelMemoryPanelProps) {
  const [memory, setMemory] = useState<MemoryState | null>(null);
  const [decisions, setDecisions] = useState<Decision[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isEditing, setIsEditing] = useState(false);
  const [editContent, setEditContent] = useState('');
  const [isSaving, setIsSaving] = useState(false);
  const [saveStatus, setSaveStatus] = useState<'idle' | 'saved' | 'error'>('idle');
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  // ---- Load ----

  const load = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const [memRes, decRes] = await Promise.all([
        apiClient.get<{ content: string; updated_at?: string }>(`/api/v1/channels/${channelId}/memory`),
        apiClient.get<{ decisions: Decision[] }>(`/api/v1/channels/${channelId}/memory/decisions`),
      ]);
      setMemory({ content: memRes.content || '', updatedAt: memRes.updated_at });
      setDecisions(decRes.decisions || []);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('channelMemoryLoadError'));
    } finally {
      setIsLoading(false);
    }
  }, [channelId]);

  useEffect(() => { load(); }, [load]);

  // ---- Edit CHANNEL.md ----

  const startEditing = useCallback(() => {
    setEditContent(memory?.content || '');
    setIsEditing(true);
    setSaveStatus('idle');
  }, [memory]);

  const cancelEditing = useCallback(() => {
    setIsEditing(false);
    setEditContent('');
    setSaveStatus('idle');
  }, []);

  const saveMemory = useCallback(async () => {
    setIsSaving(true);
    setSaveStatus('idle');
    try {
      await apiClient.put(`/api/v1/channels/${channelId}/memory`, { content: editContent });
      setMemory({ content: editContent, updatedAt: new Date().toISOString() });
      setIsEditing(false);
      setSaveStatus('saved');
      setTimeout(() => setSaveStatus('idle'), 2000);
    } catch {
      setSaveStatus('error');
    } finally {
      setIsSaving(false);
    }
  }, [channelId, editContent]);

  // ---- Auto-focus textarea ----

  useEffect(() => {
    if (isEditing && textareaRef.current) {
      textareaRef.current.focus();
    }
  }, [isEditing]);

  // ---- Loading ----

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-8">
        <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
      </div>
    );
  }

  // ---- Error ----

  if (error && !memory) {
    return (
      <div className="px-4 py-6">
        <p className="font-mono text-xs text-brutal-danger">{error}</p>
        <button
          type="button"
          onClick={load}
          className="mt-2 btn-brutal-sm px-3 py-1 text-xs"
        >
          {t('retry')}
        </button>
      </div>
    );
  }

  // ---- Render ----

  return (
    <div className="flex flex-col h-full overflow-hidden border-t-2 border-black bg-brutal-cream">
      {/* Header: toggle */}
      <button
        type="button"
        onClick={onToggle}
        className="flex items-center gap-2 h-10 px-4 text-left font-heading text-xs font-bold uppercase tracking-wider hover:bg-white/50 transition-colors"
      >
        <FileText className="h-3.5 w-3.5" />
        {t('channelMemory')}
        <span className="ml-auto">
          {expanded ? <ChevronDown className="h-3 w-3" /> : <ChevronUp className="h-3 w-3" />}
        </span>
      </button>

      {expanded && (
        <div className="flex-1 overflow-y-auto">
          {/* CHANNEL.md section */}
          <div className="border-b-2 border-black">
            <div className="flex items-center justify-between px-4 py-2">
              <span className="font-mono text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                {t('channelMemoryContext')}
              </span>
              {!isEditing && (
                <button
                  type="button"
                  onClick={startEditing}
                  className="font-mono text-[10px] font-bold uppercase tracking-wider underline hover:text-foreground"
                >
                  {t('edit')}
                </button>
              )}
            </div>

            {isEditing ? (
              <div className="px-4 pb-3">
                <textarea
                  ref={textareaRef}
                  value={editContent}
                  onChange={(e) => setEditContent(e.target.value)}
                  className="w-full min-h-[120px] border-2 border-black bg-white p-2 font-mono text-xs resize-y focus:bg-brutal-primary-light focus:outline-none"
                  placeholder={t('channelMemoryEditPlaceholder')}
                />
                <div className="flex items-center gap-2 mt-2">
                  <button
                    type="button"
                    onClick={saveMemory}
                    disabled={isSaving}
                    className="btn-brutal-xs px-3 py-1 bg-brutal-success text-black"
                  >
                    {isSaving ? (
                      <Loader2 className="h-3 w-3 animate-spin" />
                    ) : (
                      <>
                        <Save className="h-3 w-3 mr-1" />
                        {t('save')}
                      </>
                    )}
                  </button>
                  <button
                    type="button"
                    onClick={cancelEditing}
                    disabled={isSaving}
                    className="btn-flat text-xs"
                  >
                    {t('cancel')}
                  </button>
                  {saveStatus === 'saved' && (
                    <span className="font-mono text-[10px] text-brutal-success">{t('channelMemorySaveSuccess')}</span>
                  )}
                  {saveStatus === 'error' && (
                    <span className="font-mono text-[10px] text-brutal-danger">{t('channelMemorySaveError')}</span>
                  )}
                </div>
              </div>
            ) : (
              <div className="px-4 pb-3">
                {memory?.content ? (
                  <pre className="font-mono text-xs whitespace-pre-wrap break-words text-muted-foreground max-h-60 overflow-y-auto">
                    {memory.content}
                  </pre>
                ) : (
                  <p className="font-mono text-xs text-muted-foreground italic">
                    {t('channelMemoryEmpty')}
                  </p>
                )}
              </div>
            )}
          </div>

          {/* Decisions section */}
          <div>
            <div className="flex items-center justify-between px-4 py-2">
              <span className="font-mono text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                <ListChecks className="inline h-3 w-3 mr-1" />
                {t('channelMemoryDecisions')}
              </span>
            </div>

            <div className="px-4 pb-3">
              {decisions.length === 0 ? (
                <p className="font-mono text-xs text-muted-foreground italic">
                  {t('channelMemoryNoDecisions')}
                </p>
              ) : (
                <ul className="space-y-2">
                  {decisions.map((d, i) => (
                    <li key={i} className="border-l-2 border-brutal-muted pl-3">
                      <p className="font-mono text-xs">{d.content}</p>
                      <div className="flex items-center gap-2 mt-0.5">
                        {d.created_by_name && (
                          <span className="font-mono text-[10px] text-muted-foreground">{d.created_by_name}</span>
                        )}
                        {d.created_at && (
                          <span className="font-mono text-[10px] text-muted-foreground">
                            {new Date(d.created_at).toLocaleDateString()}
                          </span>
                        )}
                      </div>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
