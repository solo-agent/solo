// ============================================================================
// KnowledgeCreate — form to create/edit a knowledge entry
// - Title, content (Markdown), channel selector, tags
// - "Import from decisions" button
// - Uses Dialog for modal form
// ============================================================================

'use client';

import { useState, useMemo } from 'react';
import { BookOpen, X, Plus, FileText, Loader2, AlertTriangle } from 'lucide-react';
import { cn } from '@/lib/utils';
import { t } from '@/lib/i18n';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Select, type SelectOption } from '@/components/ui/select';
import { Dialog, DialogHeader, DialogTitle, DialogCloseButton, DialogFooter } from '@/components/ui/dialog';
import { useToast } from '@/components/ui/toast';
import { apiClient } from '@/lib/api-client';
import type { CreateKnowledgeInput, KnowledgeEntry } from '@/lib/types';

// Common tag suggestions grouped by category
const TAG_SUGGESTIONS = [
  'architecture', 'api', 'database', 'frontend', 'backend',
  'deployment', 'testing', 'security', 'performance', 'monitoring',
  'cli', 'configuration', 'migration', 'best-practice', 'gotcha',
  'onboarding', 'changelog', 'decision', 'troubleshooting',
];

interface KnowledgeCreateProps {
  /** Pre-selected channel (optional) */
  channelId?: string;
  /** Available channels */
  channels: SelectOption[];
  /** Called after successful creation */
  onCreated?: (entry: KnowledgeEntry) => void;
  /** Whether to show the dialog (controlled) */
  open?: boolean;
  /** Called when open state changes */
  onOpenChange?: (open: boolean) => void;
}

export function KnowledgeCreate({
  channelId,
  channels,
  onCreated,
  open: controlledOpen,
  onOpenChange,
}: KnowledgeCreateProps) {
  const [uncontrolledOpen, setUncontrolledOpen] = useState(false);
  const open = controlledOpen !== undefined ? controlledOpen : uncontrolledOpen;
  const setOpen = onOpenChange || setUncontrolledOpen;

  const [title, setTitle] = useState('');
  const [content, setContent] = useState('');
  const [selectedChannelId, setSelectedChannelId] = useState(channelId || '');
  const [tags, setTags] = useState<string[]>([]);
  const [tagInput, setTagInput] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [validationErrors, setValidationErrors] = useState<string[]>([]);
  const { showToast } = useToast();

  // Filter tag suggestions based on what hasn't been added yet
  const availableSuggestions = useMemo(
    () => TAG_SUGGESTIONS.filter((t) => !tags.includes(t)),
    [tags],
  );

  // Filter suggestions based on current input
  const filteredSuggestions = useMemo(() => {
    if (!tagInput.trim()) return availableSuggestions.slice(0, 5);
    const lower = tagInput.toLowerCase();
    return availableSuggestions.filter((s) => s.includes(lower)).slice(0, 5);
  }, [tagInput, availableSuggestions]);

  const resetForm = () => {
    setTitle('');
    setContent('');
    setSelectedChannelId(channelId || '');
    setTags([]);
    setTagInput('');
  };

  const validate = (): string[] => {
    const errors: string[] = [];
    if (!selectedChannelId) errors.push(t('knowledgeNoChannel'));
    if (!title.trim()) errors.push(t('knowledgeTitleRequired'));
    else if (title.trim().length < 3) errors.push(t('knowledgeTitleMinLength'));
    if (!content.trim()) errors.push(t('knowledgeContentRequired'));
    else if (content.trim().length < 10) errors.push(t('knowledgeContentMinLength'));
    return errors;
  };

  const handleSubmit = async () => {
    const errors = validate();
    setValidationErrors(errors);
    if (errors.length > 0) return;
    setIsSubmitting(true);
    try {
      const body: CreateKnowledgeInput = {
        channel_id: selectedChannelId,
        title: title.trim(),
        content: content.trim(),
        tags: tags.length > 0 ? tags : undefined,
        source: 'manual',
      };
      const entry = await apiClient.post<KnowledgeEntry>('/api/v1/knowledge', body);
      showToast(t('knowledgeImportSuccess'), 'success');
      resetForm();
      setOpen(false);
      onCreated?.(entry);
    } catch {
      showToast(t('knowledgeImportSuccess'), 'error');
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleImportFromDecisions = async () => {
    if (!selectedChannelId) return;
    setIsSubmitting(true);
    try {
      // Import from decisions: pre-fill from the channel's decisions.md
      await apiClient.post('/api/v1/knowledge/import', {
        channel_id: selectedChannelId,
        source: 'decisions.md',
      });
      showToast(t('knowledgeImportSuccess'), 'success');
      setOpen(false);
      onCreated?.(null as unknown as KnowledgeEntry);
    } catch {
      showToast(t('knowledgeImportSuccess'), 'error');
    } finally {
      setIsSubmitting(false);
    }
  };

  const addTag = (value: string) => {
    const trimmed = value.trim().toLowerCase();
    if (trimmed && !tags.includes(trimmed)) {
      setTags([...tags, trimmed]);
      setTagInput('');
    }
  };

  const removeTag = (tag: string) => {
    setTags(tags.filter((t) => t !== tag));
  };

  const handleTagKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      addTag(tagInput);
    } else if (e.key === 'Backspace' && !tagInput && tags.length > 0) {
      setTags(tags.slice(0, -1));
    }
  };

  const handleOpen = () => {
    resetForm();
    setValidationErrors([]);
    setOpen(true);
  };

  return (
    <>
      {/* Trigger button */}
      <Button
        variant="primary"
        size="sm"
        onClick={handleOpen}
        className="text-xs"
      >
        <Plus className="h-3 w-3 mr-1" />
        {t('knowledgeCreateTitle')}
      </Button>

      {/* Modal */}
      <Dialog open={open} onOpenChange={setOpen} width="lg">
        <DialogHeader>
          <DialogTitle>
            <BookOpen className="inline h-4 w-4 mr-1.5 -mt-0.5" />
            {t('knowledgeCreateTitle')}
          </DialogTitle>
          <DialogCloseButton onClick={() => setOpen(false)} />
        </DialogHeader>

        <div className="space-y-4">
          {/* Validation errors */}
          {validationErrors.length > 0 && (
            <div className="border-2 border-black bg-brutal-danger-light px-3 py-2">
              {validationErrors.map((err, i) => (
                <div key={i} className="flex items-start gap-1.5">
                  <AlertTriangle className="h-3.5 w-3.5 text-brutal-danger flex-shrink-0 mt-px" />
                  <p className="font-mono text-xs text-brutal-danger">{err}</p>
                </div>
              ))}
            </div>
          )}

          {/* Channel selector */}
          <div>
            <label className="block font-heading text-sm font-bold mb-1.5">
              {t('knowledgeChannelLabel')} *
            </label>
            <Select
              options={channels}
              value={selectedChannelId}
              onChange={setSelectedChannelId}
              placeholder={t('knowledgeChannelPlaceholder')}
              size="md"
              className="w-full"
            />
          </div>

          {/* Title */}
          <div>
            <label className="block font-heading text-sm font-bold mb-1.5">
              {t('knowledgeTitleLabel')} *
            </label>
            <Input
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder={t('knowledgeTitlePlaceholder')}
              maxLength={500}
            />
          </div>

          {/* Content */}
          <div>
            <label className="block font-heading text-sm font-bold mb-1.5">
              {t('knowledgeContentLabel')} *
            </label>
            <Textarea
              value={content}
              onChange={(e) => setContent(e.target.value)}
              placeholder={t('knowledgeContentPlaceholder')}
              className="min-h-[160px]"
            />
          </div>

          {/* Tags */}
          <div>
            <label className="block font-heading text-sm font-bold mb-1.5">
              {t('knowledgeTagsLabel')}
            </label>
            <div className="flex flex-wrap items-center gap-1.5 border-2 border-black bg-white p-2 min-h-[44px]">
              {tags.map((tag) => (
                <span
                  key={tag}
                  className={cn(
                    'inline-flex items-center gap-1 px-1.5 py-0.5',
                    'font-heading text-[10px] font-bold',
                    'border-2 border-black bg-brutal-primary-light text-black',
                  )}
                >
                  {tag}
                  <button
                    type="button"
                    onClick={() => removeTag(tag)}
                    className="hover:text-brutal-danger"
                    aria-label={t('remove')}
                  >
                    <X className="h-3 w-3" />
                  </button>
                </span>
              ))}
              <input
                value={tagInput}
                onChange={(e) => setTagInput(e.target.value)}
                onKeyDown={handleTagKeyDown}
                placeholder={t('knowledgeTagsPlaceholder')}
                className="flex-1 min-w-[120px] border-none outline-none bg-transparent font-mono text-xs text-foreground placeholder:text-muted-foreground"
              />
            </div>
            {/* Tag suggestions */}
            {filteredSuggestions.length > 0 && (
              <div className="mt-1.5">
                <span className="font-mono text-[10px] text-muted-foreground mr-1">
                  {t('knowledgeTagSuggestions')}
                </span>
                <div className="flex flex-wrap gap-1 mt-0.5">
                  {filteredSuggestions.map((suggestion) => (
                    <button
                      key={suggestion}
                      type="button"
                      onClick={() => addTag(suggestion)}
                      className={cn(
                        'px-1.5 py-0.5 border-2 border-black bg-brutal-muted-light',
                        'font-mono text-[10px] hover:bg-brutal-primary-light',
                        'active:translate-x-0.5 active:translate-y-0.5 transition-all',
                      )}
                    >
                      + {suggestion}
                    </button>
                  ))}
                </div>
              </div>
            )}
          </div>

          {/* Import from decisions */}
          <div className="pt-1 border-t-2 border-black">
            <Button
              variant="outline"
              size="sm"
              onClick={handleImportFromDecisions}
              disabled={!selectedChannelId || isSubmitting}
              className="text-xs"
            >
              <FileText className="h-3 w-3 mr-1" />
              {t('knowledgeImportFromDecisions')}
            </Button>
          </div>
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setOpen(false)}
          >
            {t('cancel')}
          </Button>
          <Button
            variant="primary"
            size="sm"
            onClick={handleSubmit}
            disabled={
              !title.trim() || !content.trim() || !selectedChannelId || isSubmitting
            }
          >
            {isSubmitting ? (
              <>
                <Loader2 className="h-3 w-3 mr-1 animate-spin" />
                {t('submitting')}
              </>
            ) : (
              t('create')
            )}
          </Button>
        </DialogFooter>
      </Dialog>
    </>
  );
}
