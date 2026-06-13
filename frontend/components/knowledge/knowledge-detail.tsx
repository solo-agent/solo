// ============================================================================
// KnowledgeDetail — modal for viewing a knowledge entry's full content
// - Shows title, content (Markdown rendered), tags, metadata
// - Edit button opens the KnowledgeCreate modal in edit mode
// - Delete button with confirmation
// ============================================================================

'use client';

import { useState, useCallback } from 'react';
import { BookOpen, Tag, User, Calendar, Loader2, Trash2, Pencil } from 'lucide-react';
import { cn } from '@/lib/utils';
import { t } from '@/lib/i18n';
import { Dialog, DialogHeader, DialogTitle, DialogCloseButton, DialogFooter } from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { KnowledgeCreate } from './knowledge-create';
import { apiClient } from '@/lib/api-client';
import type { KnowledgeEntry } from '@/lib/types';

interface KnowledgeDetailProps {
  entry: KnowledgeEntry | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Called after successful edit or delete */
  onMutate: () => void;
  /** Available channels for edit form */
  channels?: { value: string; label: string }[];
}

export function KnowledgeDetail({ entry, open, onOpenChange, onMutate, channels = [] }: KnowledgeDetailProps) {
  const [isEditing, setIsEditing] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);

  const handleDelete = useCallback(async () => {
    if (!entry) return;
    setIsDeleting(true);
    try {
      await apiClient.delete(`/api/v1/knowledge/${entry.id}`);
      onMutate();
      onOpenChange(false);
    } catch {
      // fail silently — toast handled by parent
    } finally {
      setIsDeleting(false);
    }
  }, [entry, onMutate, onOpenChange]);

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

  if (!entry) return null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange} width="lg">
      <DialogHeader>
        <DialogTitle>
          <BookOpen className="inline h-4 w-4 mr-1.5 -mt-0.5" />
          {isEditing ? t('knowledgeEditTitle') : entry.title}
        </DialogTitle>
        <DialogCloseButton onClick={() => onOpenChange(false)} />
      </DialogHeader>

      {isEditing ? (
        <KnowledgeCreate
          channelId={entry.channel_id}
          channels={channels}
          editEntry={entry}
          onCreated={() => {
            setIsEditing(false);
            onMutate();
          }}
          open={true}
          onOpenChange={(v) => {
            if (!v) setIsEditing(false);
          }}
          hideTrigger
        />
      ) : (
        <>
          {/* Metadata row */}
          <div className="flex flex-wrap items-center gap-x-4 gap-y-1 mb-4 text-xs text-muted-foreground">
            {entry.author_name && (
              <span className="flex items-center gap-1 font-mono">
                <User className="h-3 w-3" />
                {entry.author_name}
              </span>
            )}
            {entry.channel_name && (
              <span className="flex items-center gap-1 font-mono">
                <span className="font-bold">#</span>
                {entry.channel_name}
              </span>
            )}
            {entry.created_at && (
              <span className="flex items-center gap-1 font-mono">
                <Calendar className="h-3 w-3" />
                {formatDate(entry.created_at)}
              </span>
            )}
            {entry.source && (
              <span className={cn(
                'px-1.5 py-0.5 font-mono text-[10px] font-bold border border-black',
                entry.source === 'manual' ? 'bg-brutal-primary-light' : 'bg-brutal-muted-light',
              )}>
                {entry.source}
              </span>
            )}
          </div>

          {/* Tags */}
          {entry.tags && entry.tags.length > 0 && (
            <div className="flex flex-wrap items-center gap-1.5 mb-4">
              <Tag className="h-3 w-3 text-muted-foreground" />
              {entry.tags.map((tag) => (
                <span
                  key={tag}
                  className="px-1.5 py-0.5 font-heading text-[10px] font-bold border-2 border-black bg-brutal-muted-light"
                >
                  {tag}
                </span>
              ))}
            </div>
          )}

          {/* Content */}
          <div className="border-2 border-black bg-white p-4 max-h-[50vh] overflow-y-auto">
            <pre className="font-mono text-sm text-foreground whitespace-pre-wrap break-words leading-relaxed">
              {entry.content}
            </pre>
          </div>

          {/* Footer actions */}
          <DialogFooter>
            <Button
              variant="outline"
              size="sm"
              onClick={handleDelete}
              disabled={isDeleting}
              className="text-xs text-brutal-danger hover:bg-brutal-danger-light"
            >
              {isDeleting ? (
                <Loader2 className="h-3 w-3 mr-1 animate-spin" />
              ) : (
                <Trash2 className="h-3 w-3 mr-1" />
              )}
              {t('delete')}
            </Button>
            <div className="flex-1" />
            <Button
              variant="primary"
              size="sm"
              onClick={() => setIsEditing(true)}
              className="text-xs"
            >
              <Pencil className="h-3 w-3 mr-1" />
              {t('edit')}
            </Button>
          </DialogFooter>
        </>
      )}
    </Dialog>
  );
}
