// ============================================================================
// MessageInput — bottom message composition with brutalist styling
// - input-brutal textarea with Space Mono placeholder
// - Send button: btn-brutal-success circular icon button
// - Enter/Shift+Enter handling
// - @mention autocomplete (SOLO-51-F)
// - File upload: drag & drop + paste (SOLO-247-F)
// ============================================================================

'use client';

import {
  useState,
  useRef,
  useCallback,
  useEffect,
  type KeyboardEvent,
  type DragEvent,
  type ClipboardEvent,
  type ChangeEvent,
} from 'react';
import {
  Send,
  MessageSquare,
  SquareCheckBig,
  Upload,
  Paperclip,
  X,
  Check,
  AlertTriangle,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { useMentions } from '@/lib/hooks/use-mentions';
import { MentionDropdown, type DropdownAnchor } from './mention-dropdown';
import { useToast } from '@/components/ui/toast';
import { Spinner } from '@/components/ui/spinner';
import { t } from '@/lib/i18n';
import { resolveAttachmentUrl } from '@/lib/attachment-url';
import type { ChannelMember } from '@/lib/types';

// ---- Types ----

/** A single file being uploaded or already uploaded */
export interface UploadItem {
  id: string;
  filename: string;
  url: string;
  mimeType: string;
  size: number;
  status: 'uploading' | 'done' | 'error';
}

interface MessageInputProps {
  onSend: (
    content: string,
    mentionedAgentIds?: string[],
    asTask?: boolean,
    taskTitle?: string,
    attachmentIds?: string[],
  ) => Promise<unknown> | void;
  placeholder?: string;
  /** All channel members (users + agents) for @mention filtering */
  members: ChannelMember[];
  /** Show the "As Task" toggle (for channel views only) */
  showAsTaskToggle?: boolean;
}

// ---- Upload helper ----

let uploadCounter = 0;

async function uploadSingleFile(file: File): Promise<UploadItem> {
  // Validate size (max 50MB)
  if (file.size > 50 * 1024 * 1024) {
    throw new Error(t('fileSizeExceeded', { name: file.name }));
  }

  const formData = new FormData();
  formData.append('file', file);

  // Use apiClient.postFormData for multipart — browser sets Content-Type with boundary.
  // Dynamic import avoids potential module-level circular dependency issues.
  const { apiClient } = await import('@/lib/api-client');
  const res = await apiClient.postFormData<{
    id: string;
    url: string;
    mime_type: string;
  }>('/api/v1/attachments/upload', formData);

  return {
    id: res.id,
    filename: file.name,
    url: resolveAttachmentUrl(res.url),
    mimeType: res.mime_type,
    size: file.size,
    status: 'done' as const,
  };
}

// ---- Main component ----

export function MessageInput({
  onSend,
  placeholder = t('messagePlaceholder'),
  members,
  showAsTaskToggle = false,
}: MessageInputProps) {
  const [content, setContent] = useState('');
  const [isSending, setIsSending] = useState(false);
  const [cursorPosition, setCursorPosition] = useState(0);
  const [asTask, setAsTask] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const isSendingRef = useRef(false);

  // ---- Upload state ----
  const [isDragging, setIsDragging] = useState(false);
  const [uploads, setUploads] = useState<UploadItem[]>([]);
  const dragCounterRef = useRef(0);
  const { showToast } = useToast();

  const {
    suggestions,
    showSuggestions,
    selectedIndex,
    searchQuery,
    handleKeyDown: mentionHandleKeyDown,
    selectSuggestion: mentionSelectSuggestion,
    resetMention,
    mentionedAgentIds,
  } = useMentions(members, content, cursorPosition);

  const mentionActive = showSuggestions || searchQuery !== '';

  // ---- Dropdown anchor calculation ----

  const [dropdownAnchor, setDropdownAnchor] = useState<DropdownAnchor | null>(
    null,
  );

  const updateDropdownPosition = useCallback(() => {
    const el = textareaRef.current;
    if (!el) return;
    const rect = el.getBoundingClientRect();
    setDropdownAnchor({
      top: rect.top - 8,
      left: rect.left + 16,
      width: rect.width - 32,
    });
  }, []);

  useEffect(() => {
    if (mentionActive) {
      updateDropdownPosition();
    } else {
      setDropdownAnchor(null);
    }
  }, [mentionActive, updateDropdownPosition]);

  // Recalculate on scroll/resize while dropdown is open
  useEffect(() => {
    if (!mentionActive) return;
    const handleUpdate = () => updateDropdownPosition();
    window.addEventListener('scroll', handleUpdate, true);
    window.addEventListener('resize', handleUpdate);
    return () => {
      window.removeEventListener('scroll', handleUpdate, true);
      window.removeEventListener('resize', handleUpdate);
    };
  }, [mentionActive, updateDropdownPosition]);

  // ---- Click-outside handler ----

  useEffect(() => {
    if (!mentionActive) return;

    const handleClick = (e: MouseEvent) => {
      const target = e.target as Node;
      const dropdownEl = document.querySelector(
        '[role="listbox"][aria-label="Select member to mention"]',
      );
      if (
        dropdownEl &&
        !dropdownEl.contains(target) &&
        textareaRef.current &&
        !textareaRef.current.contains(target)
      ) {
        resetMention();
      }
    };

    const timer = setTimeout(() => {
      document.addEventListener('mousedown', handleClick);
    }, 0);

    return () => {
      clearTimeout(timer);
      document.removeEventListener('mousedown', handleClick);
    };
  }, [mentionActive, resetMention]);

  // ---- Upload logic ----

  /** Remove a single upload by ID */
  const removeUpload = useCallback((id: string) => {
    setUploads((prev) => prev.filter((u) => u.id !== id));
  }, []);

  /** Upload one or more files and add them to the uploads list */
  const uploadFiles = useCallback(async (files: File[]) => {
    if (files.length === 0) return;

    // Add placeholder entries for each file
    const placeholders: UploadItem[] = files.map((file) => ({
      id: `upload-${++uploadCounter}-${Date.now()}`,
      filename: file.name,
      url: '',
      mimeType: file.type,
      size: file.size,
      status: 'uploading',
    }));
    setUploads((prev) => [...prev, ...placeholders]);

    // Upload each file sequentially
    for (let i = 0; i < files.length; i++) {
      const file = files[i];
      const placeholder = placeholders[i];

      // Validate size
      if (file.size > 50 * 1024 * 1024) {
        showToast(t('fileSizeExceeded', { name: file.name }), 'error');
        setUploads((prev) =>
          prev.map((u) =>
            u.id === placeholder.id ? { ...u, status: 'error' as const } : u,
          ),
        );
        continue;
      }

      try {
        const result = await uploadSingleFile(file);
        // Replace placeholder with real result
        setUploads((prev) =>
          prev.map((u) =>
            u.id === placeholder.id
              ? { ...result, id: result.id, filename: file.name }
              : u,
          ),
        );
      } catch (err) {
        const msg =
          err instanceof Error ? err.message : `${file.name}: Upload failed`;
        showToast(msg, 'error');
        setUploads((prev) =>
          prev.map((u) =>
            u.id === placeholder.id ? { ...u, status: 'error' as const } : u,
          ),
        );
      }
    }

  }, [showToast]);

  const openFilePicker = useCallback(() => {
    fileInputRef.current?.click();
  }, []);

  const handleFileInputChange = useCallback(
    async (e: ChangeEvent<HTMLInputElement>) => {
      const files = e.target.files;
      if (files && files.length > 0) {
        await uploadFiles(Array.from(files));
      }
      e.target.value = '';
    },
    [uploadFiles],
  );

  // ---- Drag & drop handlers ----

  const handleDragOver = useCallback((e: DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
  }, []);

  const handleDragEnter = useCallback((e: DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    dragCounterRef.current += 1;
    if (e.dataTransfer?.types?.includes('Files')) {
      setIsDragging(true);
    }
  }, []);

  const handleDragLeave = useCallback((e: DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    dragCounterRef.current -= 1;
    if (dragCounterRef.current <= 0) {
      dragCounterRef.current = 0;
      setIsDragging(false);
    }
  }, []);

  const handleDrop = useCallback(
    async (e: DragEvent) => {
      e.preventDefault();
      e.stopPropagation();
      dragCounterRef.current = 0;
      setIsDragging(false);

      const files = e.dataTransfer?.files;
      if (files && files.length > 0) {
        await uploadFiles(Array.from(files));
      }
    },
    [uploadFiles],
  );

  // ---- Paste handler ----

  const handlePaste = useCallback(
    async (e: ClipboardEvent<HTMLTextAreaElement>) => {
      const items = e.clipboardData?.items;
      if (!items) return;

      const imageFiles: File[] = [];

      for (let i = 0; i < items.length; i++) {
        const item = items[i];
        if (item.type.startsWith('image/')) {
          e.preventDefault();
          const file = item.getAsFile();
          if (file) {
            imageFiles.push(file);
          }
        }
      }

      if (imageFiles.length > 0) {
        await uploadFiles(imageFiles);
      }
    },
    [uploadFiles],
  );

  // ---- Send logic ----

  const trimmed = content.trim();
  const doneUploads = uploads.filter((u) => u.status === 'done');
  const hasUploading = uploads.some((u) => u.status === 'uploading');
  const canSend = (trimmed.length > 0 || (!asTask && doneUploads.length > 0)) && !isSending && !hasUploading;

  const handleSend = useCallback(async () => {
    if (!canSend || isSendingRef.current) return;

    isSendingRef.current = true;
    setIsSending(true);
    try {
      const attachmentIds =
        doneUploads.length > 0
          ? doneUploads.map((u) => u.id)
          : undefined;

      await onSend(
        trimmed,
        mentionedAgentIds,
        asTask,
        undefined,
        attachmentIds,
      );
      setContent('');
      setAsTask(false);
      setUploads([]);
      resetMention();
      if (textareaRef.current) {
        textareaRef.current.style.height = 'auto';
      }
    } finally {
      isSendingRef.current = false;
      setIsSending(false);
      textareaRef.current?.focus();
    }
  }, [canSend, trimmed, onSend, mentionedAgentIds, asTask, doneUploads, resetMention]);

  // ---- Keyboard handling ----

  const handleKeyDown = useCallback(
    (e: KeyboardEvent<HTMLTextAreaElement>) => {
      // Arrow up/down with mention active: navigate suggestions
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        if (mentionActive) {
          e.preventDefault();
          mentionHandleKeyDown(e);
        }
        return;
      }

      // Escape: close mention dropdown
      if (e.key === 'Escape') {
        if (mentionActive) {
          e.preventDefault();
          resetMention();
        }
        return;
      }

      // Enter: select suggestion OR send message
      if (e.key === 'Enter' && !e.shiftKey) {
        if (showSuggestions) {
          e.preventDefault();
          const newValue = mentionSelectSuggestion(selectedIndex);
          if (newValue !== null) {
            setContent(newValue);
          }
          return;
        }

        e.preventDefault();
        handleSend();
        return;
      }
    },
    [
      mentionActive,
      mentionHandleKeyDown,
      resetMention,
      showSuggestions,
      mentionSelectSuggestion,
      selectedIndex,
      handleSend,
    ],
  );

  // ---- Cursor tracking ----

  const handleCursorMove = useCallback(() => {
    if (textareaRef.current) {
      setCursorPosition(textareaRef.current.selectionStart);
    }
  }, []);

  const handleInput = useCallback(
    (value: string) => {
      setContent(value);
      const el = textareaRef.current;
      if (el) {
        setCursorPosition(el.selectionStart);
        el.style.height = 'auto';
        el.style.height = `${Math.min(el.scrollHeight, 200)}px`;
      }
    },
    [],
  );

  // ---- Drag overlay container events ----
  // Attach drag enter/leave to the outer wrapper so the overlay covers all input area

  const dragContainerRef = useRef<HTMLDivElement>(null);

  // ---- Format file size ----

  const formatSize = (bytes: number): string => {
    if (bytes < 1024) return `${bytes}B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)}KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)}MB`;
  };

  // ---- Image MIME check ----

  const IMAGE_MIME_TYPES = ['image/jpeg', 'image/png', 'image/gif', 'image/webp', 'image/svg+xml',
    'image/jpg', 'image/bmp', 'image/tiff'];

  const isImageMime = (mime: string): boolean =>
    IMAGE_MIME_TYPES.includes(mime) || mime.startsWith('image/');

  // ---- Render ----

  return (
    <div
      ref={dragContainerRef}
      className="relative border-t-2 border-black bg-brutal-cream px-6 py-4"
      onDragEnter={handleDragEnter}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
    >
      {/* Drag overlay */}
      {isDragging && (
        <div
          className={cn(
            'absolute inset-0 z-50 flex flex-col items-center justify-center gap-2',
            'border-2 border-dashed border-black bg-brutal-primary-light/60',
          )}
          aria-live="polite"
        >
          <Upload className="h-8 w-8 text-brutal-black opacity-60" aria-hidden />
          <p className="font-heading text-base font-bold text-foreground">
            {t('dragDropHint')}
          </p>
          <p className="font-mono text-xs text-muted-foreground">
            {t('maxFileSize')}
          </p>
        </div>
      )}

      <div className="relative flex flex-col">
        {/* Mention dropdown via portal */}
        {mentionActive && dropdownAnchor && (
          <MentionDropdown
            suggestions={suggestions}
            selectedIndex={selectedIndex}
            searchQuery={searchQuery}
            anchor={dropdownAnchor}
            onSelect={(index) => {
              const newValue = mentionSelectSuggestion(index);
              if (newValue !== null) {
                setContent(newValue);
                textareaRef.current?.focus();
              }
            }}
          />
        )}

        {/* Upload previews */}
        {uploads.length > 0 && (
          <div className="mb-2 flex flex-wrap gap-2" aria-label={t('uploadPreview')}>
            {uploads.map((upload) => (
              <div
                key={upload.id}
                className={cn(
                  'relative flex items-start gap-2 border-2 border-black bg-white px-2.5 py-1.5',
                  'shadow-brutal-sm',
                  upload.status === 'error' && 'border-brutal-danger',
                )}
              >
                {/* Uploading: indeterminate progress bar */}
                {upload.status === 'uploading' && (
                  <div className="absolute inset-x-0 bottom-1 left-1 right-1 h-1 overflow-hidden bg-brutal-muted/30">
                    <div className="h-full w-1/3 animate-indeterminate-progress bg-brutal-primary" />
                  </div>
                )}

                {/* Image thumbnail for done image uploads */}
                {upload.status === 'done' && isImageMime(upload.mimeType) ? (
                  <img
                    src={upload.url}
                    alt={upload.filename}
                    className="h-10 w-10 flex-shrink-0 border border-black object-cover bg-brutal-cream"
                  />
                ) : (
                  /* Status icon for non-image files */
                  <div className="flex h-6 w-6 flex-shrink-0 items-center justify-center">
                    {upload.status === 'uploading' && (
                      <Spinner size="sm" className="text-muted-foreground" label={t('uploading')} />
                    )}
                    {upload.status === 'done' && (
                      <Check className="h-3.5 w-3.5 text-brutal-success" aria-label={t('uploadedDone')} />
                    )}
                    {upload.status === 'error' && (
                      <AlertTriangle className="h-3.5 w-3.5 text-brutal-danger" aria-label={t('uploadFailed')} />
                    )}
                  </div>
                )}

                {/* Filename + size */}
                <div className="flex min-w-0 flex-col">
                  <span className="truncate font-mono text-xs font-bold text-foreground">
                    {upload.filename}
                  </span>
                  <span className="font-mono text-[10px] text-muted-foreground">
                    {upload.status === 'uploading'
                      ? t('uploading')
                      : upload.status === 'done'
                        ? formatSize(upload.size)
                        : t('uploadFailed')}
                  </span>
                </div>

                {/* Remove button */}
                <button
                  type="button"
                  onClick={() => removeUpload(upload.id)}
                  disabled={upload.status === 'uploading'}
                  className={cn(
                    'flex-shrink-0 p-0.5 text-muted-foreground hover:text-foreground transition-colors',
                    'disabled:opacity-30 disabled:pointer-events-none',
                  )}
                  aria-label={t('removeUpload', { filename: upload.filename })}
                >
                  <X className="h-3.5 w-3.5" />
                </button>
              </div>
            ))}
          </div>
        )}

        {/* Message / Task mode */}
        {showAsTaskToggle && (
          <div className="mb-2 flex w-fit items-center gap-2" role="group" aria-label={`${t('messages')} / ${t('tasks')}`}>
            <button
              type="button"
              onClick={() => setAsTask(false)}
              className={cn(
                'btn-brutal btn-brutal-sm flex items-center gap-1.5 px-2.5 py-1 font-mono text-[11px] font-bold',
                !asTask ? 'btn-brutal-primary' : 'bg-white text-muted-foreground hover:text-foreground',
              )}
              aria-pressed={!asTask}
            >
              <MessageSquare className="h-3.5 w-3.5" />
              {t('messages')}
            </button>
            <button
              type="button"
              onClick={() => setAsTask(true)}
              className={cn(
                'btn-brutal btn-brutal-sm flex items-center gap-1.5 px-2.5 py-1 font-mono text-[11px] font-bold',
                asTask ? 'btn-brutal-primary' : 'bg-white text-muted-foreground hover:text-foreground',
              )}
              aria-pressed={asTask}
            >
              <SquareCheckBig className="h-3.5 w-3.5" />
              {t('tasks')}
            </button>
          </div>
        )}

        <div className="relative flex items-end gap-2">
          <input
            ref={fileInputRef}
            type="file"
            multiple
            className="hidden"
            onChange={handleFileInputChange}
            aria-hidden="true"
            tabIndex={-1}
          />
          <textarea
            ref={textareaRef}
            value={content}
            onChange={(e) => handleInput(e.target.value)}
            onKeyDown={handleKeyDown}
            onSelect={handleCursorMove}
            onClick={handleCursorMove}
            onPaste={handlePaste}
            placeholder={asTask ? t('taskMessagePlaceholder') : placeholder}
            rows={1}
            autoFocus
            disabled={isSending}
            aria-label={asTask ? t('taskDescriptionInput') : t('messageInput')}
            aria-autocomplete="list"
            aria-controls={mentionActive ? 'mention-listbox' : undefined}
            aria-expanded={mentionActive}
            aria-haspopup="listbox"
            className={cn(
              'input-brutal min-h-[44px] resize-none font-mono text-sm leading-relaxed',
              'placeholder:font-mono placeholder:text-muted-foreground/60',
              'disabled:opacity-50',
              asTask ? 'pr-36' : 'pr-24',
              asTask && 'border-brutal-primary',
            )}
          />
          <button
            type="button"
            onClick={openFilePicker}
            disabled={isSending || hasUploading}
            className={cn(
              'absolute bottom-2 flex h-8 w-8 items-center justify-center',
              'btn-brutal bg-white text-foreground hover:bg-brutal-primary-light',
              'disabled:opacity-40 disabled:pointer-events-none',
              asTask ? 'right-[112px]' : 'right-12',
            )}
            aria-label={t('attachFiles')}
            title={t('attachFiles')}
          >
            <Paperclip className="h-4 w-4" />
          </button>
          <button
            type="button"
            onClick={handleSend}
            disabled={!canSend}
            className={cn(
              'absolute bottom-2 right-2 flex h-8 items-center justify-center gap-1.5 px-3',
              'btn-brutal btn-brutal-success',
              !canSend && 'opacity-40 pointer-events-none',
              // v3.1: when the user has typed something and the send is
              // armed, a slow 2s pulse draws the eye without being frantic.
              // Killed by prefers-reduced-motion. Disabled state is already
              // handled above (opacity-40 + pointer-events-none), so the
              // pulse only runs on the active, ready-to-send state.
              canSend && 'animate-pulse-brutal',
              asTask ? 'w-auto' : 'w-8 p-0',
            )}
            aria-label={asTask ? t('createTask') : t('sendMessage')}
          >
            {asTask ? (
              <span className="font-mono text-[11px] font-bold whitespace-nowrap">{t('createTask')}</span>
            ) : (
              <Send className="h-4 w-4" />
            )}
          </button>
        </div>
      </div>
      <p className="mt-1.5 text-center font-mono text-[10px] text-muted-foreground">
        {asTask ? t('taskInputHint') : t('messageInputHint')}
      </p>
    </div>
  );
}
