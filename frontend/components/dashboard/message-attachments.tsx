// ============================================================================
// MessageAttachments — inline attachment display for messages (SOLO-249-F)
// - Image previews: border-2 border-black, max-w-[300px], cursor-zoom-in
// - File cards: border-2, shadow-brutal-sm, icon + info + download button
// - ImageLightbox: full-screen overlay for image enlargement
// - Grid: flex wrap, max 2 per row, gap-2
// ============================================================================

'use client';

import { useState, useCallback, useEffect } from 'react';
import { File, Download, ImageOff, X } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { Attachment } from '@/lib/types';

// ---- Helpers ----

const IMAGE_MIME_TYPES = ['image/jpeg', 'image/png', 'image/gif', 'image/webp', 'image/svg+xml'];

const IMAGE_EXTENSIONS = ['.jpg', '.jpeg', '.png', '.gif', '.webp', '.svg'];

function isImageAttachment(att: Attachment): boolean {
  if (att.mime_type && IMAGE_MIME_TYPES.includes(att.mime_type)) return true;
  const lower = att.filename.toLowerCase();
  return IMAGE_EXTENSIONS.some((ext) => lower.endsWith(ext));
}

export function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

// ---- Image lightbox ----

interface ImageLightboxProps {
  /** The attachment to display full-size */
  attachment: Attachment;
  /** Called when the lightbox should close */
  onClose: () => void;
}

export function ImageLightbox({ attachment, onClose }: ImageLightboxProps) {
  const [loadState, setLoadState] = useState<'loading' | 'loaded' | 'error'>('loading');

  // Close on Escape
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
      }
    };
    document.addEventListener('keydown', handler);
    // Prevent body scroll while lightbox is open
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.removeEventListener('keydown', handler);
      document.body.style.overflow = prevOverflow;
    };
  }, [onClose]);

  const handleBackdropClick = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      if (e.target === e.currentTarget) {
        onClose();
      }
    },
    [onClose],
  );

  return (
    <div
      className="fixed inset-0 z-50 flex flex-col items-center justify-center bg-black/80"
      onClick={handleBackdropClick}
      role="dialog"
      aria-label={`预览图片: ${attachment.filename}`}
      aria-modal="true"
    >
      {/* Close button */}
      <button
        type="button"
        onClick={onClose}
        className="absolute top-4 right-4 flex h-10 w-10 items-center justify-center border-2 border-white bg-black text-white hover:bg-brutal-red transition-colors"
        aria-label="关闭预览"
      >
        <X className="h-5 w-5" />
      </button>

      {/* Image */}
      <div className="relative flex max-w-[90vw] max-h-[85vh] items-center justify-center">
        {/* Loading skeleton */}
        {loadState === 'loading' && (
          <div className="flex h-64 w-96 items-center justify-center border-2 border-white/20 bg-white/5">
            <div className="h-8 w-8 animate-spin rounded-full border-2 border-white border-t-transparent" />
          </div>
        )}

        {/* Error state */}
        {loadState === 'error' && (
          <div className="flex flex-col items-center gap-3 p-12 text-white">
            <ImageOff className="h-10 w-10 text-brutal-red" />
            <p className="font-mono text-sm">图片加载失败</p>
          </div>
        )}

        <img
          src={attachment.url}
          alt={attachment.filename}
          onLoad={() => setLoadState('loaded')}
          onError={() => setLoadState('error')}
          className={cn(
            'max-w-full max-h-[85vh] object-contain',
            loadState === 'loaded' ? 'block' : 'hidden',
          )}
        />
      </div>

      {/* Filename */}
      {loadState === 'loaded' && (
        <p className="mt-3 font-mono text-sm text-white/80">
          {attachment.filename}
        </p>
      )}
    </div>
  );
}

// ---- Image preview (inline in message) ----

interface InlineImageProps {
  attachment: Attachment;
  onClick: (att: Attachment) => void;
}

function InlineImage({ attachment, onClick }: InlineImageProps) {
  const [loadState, setLoadState] = useState<'loading' | 'loaded' | 'error'>('loading');

  return (
    <div
      className="relative flex-shrink-0 border-2 border-black cursor-zoom-in overflow-hidden bg-brutal-cream"
      style={{ maxWidth: 300 }}
      onClick={() => {
        if (loadState === 'loaded') onClick(attachment);
      }}
      onKeyDown={(e) => {
        if (e.key === 'Enter' && loadState === 'loaded') onClick(attachment);
      }}
      role="button"
      tabIndex={0}
      aria-label={`查看大图: ${attachment.filename}`}
    >
      {/* Loading skeleton */}
      {loadState === 'loading' && (
        <div className="flex h-40 w-full items-center justify-center bg-brutal-stone/20">
          <div className="h-6 w-6 animate-spin rounded-full border-2 border-brutal-stone border-t-transparent" />
        </div>
      )}

      {/* Error state */}
      {loadState === 'error' && (
        <div className="flex flex-col items-center gap-1 p-4">
          <ImageOff className="h-6 w-6 text-brutal-stone" />
          <span className="font-mono text-[10px] text-muted-foreground">
            图片加载失败
          </span>
        </div>
      )}

      <img
        src={attachment.url}
        alt={attachment.filename}
        onLoad={() => setLoadState('loaded')}
        onError={() => setLoadState('error')}
        className={cn(
          'w-full max-w-[300px] object-cover',
          loadState === 'loaded' ? 'block' : 'hidden',
        )}
        loading="lazy"
      />
    </div>
  );
}

// ---- File card (non-image files) ----

interface FileCardProps {
  attachment: Attachment;
}

function FileCard({ attachment }: FileCardProps) {
  const downloadUrl = attachment.url;
  const filename = attachment.filename;
  // Truncate long filenames — show first N chars + extension
  const extIndex = filename.lastIndexOf('.');
  const displayName =
    extIndex > 0 && filename.length > 24
      ? filename.slice(0, 18) + '...' + filename.slice(extIndex)
      : filename;

  return (
    <a
      href={downloadUrl}
      target="_blank"
      rel="noopener noreferrer"
      className={cn(
        'flex items-center gap-3 p-3',
        'border-2 border-black bg-white',
        'shadow-brutal-sm',
        'hover:-translate-x-px hover:-translate-y-px hover:shadow-brutal',
        'transition-all cursor-pointer',
        'no-underline text-foreground min-w-0',
      )}
      aria-label={`下载 ${filename}`}
    >
      <div className="flex h-9 w-9 flex-shrink-0 items-center justify-center border-2 border-black bg-brutal-cream">
        <File className="h-4 w-4 text-muted-foreground" />
      </div>
      <div className="min-w-0 flex-1">
        <p className="truncate font-mono text-xs font-bold text-foreground" title={filename}>
          {displayName}
        </p>
        <p className="font-mono text-[10px] text-muted-foreground">
          {formatFileSize(attachment.size)}
        </p>
      </div>
      <Download className="h-4 w-4 flex-shrink-0 text-muted-foreground" />
    </a>
  );
}

// ---- Main attachment grid ----

interface MessageAttachmentsProps {
  attachments: Attachment[];
}

export function MessageAttachments({ attachments }: MessageAttachmentsProps) {
  const [lightboxAttachment, setLightboxAttachment] = useState<Attachment | null>(null);

  if (!attachments || attachments.length === 0) return null;

  return (
    <>
      <div
        className="mt-2 flex flex-wrap gap-2"
        role="group"
        aria-label={`${attachments.length} 个附件`}
      >
        {attachments.map((att) =>
          isImageAttachment(att) ? (
            <InlineImage
              key={att.id}
              attachment={att}
              onClick={setLightboxAttachment}
            />
          ) : (
            <FileCard key={att.id} attachment={att} />
          ),
        )}
      </div>

      {/* Lightbox portal — rendered at the attachment grid level but overlays everything */}
      {lightboxAttachment && (
        <ImageLightbox
          attachment={lightboxAttachment}
          onClose={() => setLightboxAttachment(null)}
        />
      )}
    </>
  );
}
