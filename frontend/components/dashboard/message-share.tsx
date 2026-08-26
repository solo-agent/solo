'use client';

import { useEffect, useState } from 'react';
import { Check, Copy, Download, Image as ImageIcon, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogCloseButton,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { t } from '@/lib/i18n';

export interface ShareableMessage {
  id: string;
  displayName: string;
  content: string;
  createdAt: string;
}

export async function copyMessageText(content: string) {
  await navigator.clipboard.writeText(content);
}

function wrapText(context: CanvasRenderingContext2D, text: string, maxWidth: number) {
  const lines: string[] = [];
  for (const paragraph of text.split('\n')) {
    if (!paragraph) {
      lines.push('');
      continue;
    }
    let line = '';
    for (const char of paragraph) {
      if (context.measureText(line + char).width > maxWidth && line) {
        lines.push(line);
        line = char;
      } else {
        line += char;
      }
    }
    lines.push(line);
  }
  return lines;
}

export async function createMessageShareImage(messages: ShareableMessage[], contextLabel: string) {
  const width = 760;
  const padding = 52;
  const bodyWidth = width - padding * 2;
  const canvas = document.createElement('canvas');
  const context = canvas.getContext('2d');
  if (!context) throw new Error('Canvas is not available');

  context.font = '28px "Noto Sans SC", system-ui, sans-serif';
  const blocks = messages.map((message) => ({
    message,
    lines: wrapText(context, message.content, bodyWidth),
  }));
  const height = 150 + blocks.reduce((sum, block) => sum + 70 + block.lines.length * 42, 0) + 46;
  const scale = 2;
  canvas.width = width * scale;
  canvas.height = height * scale;
  context.scale(scale, scale);

  context.fillStyle = '#f7f2ea';
  context.fillRect(0, 0, width, height);
  context.fillStyle = '#3f3832';
  context.font = '700 24px "Noto Sans SC", system-ui, sans-serif';
  context.fillText('Solo', padding, 54);
  context.font = '500 18px "Noto Sans SC", system-ui, sans-serif';
  context.fillStyle = '#766d65';
  context.fillText(contextLabel, padding, 88);
  context.strokeStyle = '#c8bdb2';
  context.lineWidth = 2;
  context.beginPath();
  context.moveTo(padding, 112);
  context.lineTo(width - padding, 112);
  context.stroke();

  let y = 154;
  for (const block of blocks) {
    const createdAt = new Date(block.message.createdAt);
    const time = Number.isNaN(createdAt.getTime())
      ? ''
      : createdAt.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
    context.fillStyle = '#3f3832';
    context.font = '700 20px "Noto Sans SC", system-ui, sans-serif';
    context.fillText(block.message.displayName, padding, y);
    const displayNameWidth = context.measureText(block.message.displayName).width;
    context.fillStyle = '#8b8178';
    context.font = '14px ui-monospace, monospace';
    context.fillText(time, padding + displayNameWidth + 18, y);
    y += 42;
    context.fillStyle = '#3f3832';
    context.font = '28px "Noto Sans SC", system-ui, sans-serif';
    for (const line of block.lines) {
      context.fillText(line, padding, y);
      y += 42;
    }
    y += 28;
  }

  return await new Promise<Blob>((resolve, reject) => {
    canvas.toBlob((blob) => blob ? resolve(blob) : reject(new Error('Image generation failed')), 'image/png');
  });
}

export function MessageSelectionToolbar({
  count,
  onCancel,
  onCreateImage,
}: {
  count: number;
  onCancel: () => void;
  onCreateImage: () => void;
}) {
  if (!count) return null;
  return (
    <div
      data-message-selection-toolbar
      className="absolute bottom-4 left-1/2 z-20 flex -translate-x-1/2 items-center gap-3 rounded-xl border-2 border-brutal-border bg-brutal-cream px-3 py-2 shadow-brutal-lg"
    >
      <span className="font-body text-sm font-semibold">{t('selectedMessages', { n: count })}</span>
      <Button type="button" size="sm" variant="outline" onClick={onCancel}>
        <X className="mr-1 h-3.5 w-3.5" />
        {t('cancelSelection')}
      </Button>
      <Button type="button" size="sm" onClick={onCreateImage}>
        <ImageIcon className="mr-1 h-3.5 w-3.5" />
        {t('createShareImage')}
      </Button>
    </div>
  );
}

export function MessageSelectMark({ selected }: { selected: boolean }) {
  return (
    <span
      aria-hidden="true"
      className={`mt-1 flex h-5 w-5 shrink-0 items-center justify-center rounded border-2 ${selected ? 'border-brutal-accent bg-brutal-primary-light' : 'border-brutal-border bg-white'}`}
    >
      {selected && <Check className="h-3.5 w-3.5" />}
    </span>
  );
}

export function ShareMessagesDialog({
  open,
  onOpenChange,
  messages,
  contextLabel,
  onError,
  onCopied,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  messages: ShareableMessage[];
  contextLabel: string;
  onError: () => void;
  onCopied: () => void;
}) {
  const [blob, setBlob] = useState<Blob | null>(null);
  const [previewUrl, setPreviewUrl] = useState('');

  useEffect(() => {
    if (!open || !messages.length) return;
    let cancelled = false;
    void createMessageShareImage(messages, contextLabel)
      .then((nextBlob) => {
        if (cancelled) return;
        const url = URL.createObjectURL(nextBlob);
        setBlob(nextBlob);
        setPreviewUrl(url);
      })
      .catch(onError);
    return () => { cancelled = true; };
  }, [contextLabel, messages, onError, open]);

  useEffect(() => () => {
    if (previewUrl) URL.revokeObjectURL(previewUrl);
  }, [previewUrl]);

  const download = () => {
    if (!previewUrl) return;
    const link = document.createElement('a');
    link.href = previewUrl;
    link.download = `solo-messages-${Date.now()}.png`;
    link.click();
  };

  const copyImage = async () => {
    if (!blob || !window.ClipboardItem) return onError();
    try {
      await navigator.clipboard.write([new ClipboardItem({ 'image/png': blob })]);
      onCopied();
    } catch {
      onError();
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange} width="lg">
      <DialogHeader>
        <DialogTitle>{t('shareImageTitle')}</DialogTitle>
        <DialogCloseButton onClick={() => onOpenChange(false)} />
      </DialogHeader>
      <div className="overflow-hidden rounded-xl border-2 border-brutal-border bg-brutal-cream">
        {previewUrl ? (
          // Blob previews are local-only and should not pass through Next image optimization.
          // eslint-disable-next-line @next/next/no-img-element
          <img data-share-image-preview src={previewUrl} alt={t('shareImagePreviewAlt')} className="max-h-[55vh] w-full object-contain" />
        ) : (
          <div className="h-64 animate-pulse bg-brutal-muted/30" />
        )}
      </div>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={copyImage} disabled={!blob}>
          <Copy className="mr-1.5 h-4 w-4" />
          {t('copyImage')}
        </Button>
        <Button data-share-image-download type="button" onClick={download} disabled={!blob}>
          <Download className="mr-1.5 h-4 w-4" />
          {t('downloadImage')}
        </Button>
      </DialogFooter>
    </Dialog>
  );
}
