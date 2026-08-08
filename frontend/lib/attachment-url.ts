import { useEffect, useState } from 'react';
import { apiClient } from '@/lib/api-client';

const DEFAULT_API_BASE_URL = 'http://localhost:8080';

export function resolveAttachmentUrl(url: string): string {
  if (!url) return url;
  if (/^(https?:|data:|blob:)/i.test(url)) return url;

  const baseUrl = process.env.NEXT_PUBLIC_API_URL ?? DEFAULT_API_BASE_URL;
  return new URL(url, baseUrl).toString();
}

function attachmentAPIPath(url: string): string | null {
  if (!url || /^(data:|blob:)/i.test(url)) return null;
  const baseUrl = process.env.NEXT_PUBLIC_API_URL ?? DEFAULT_API_BASE_URL;
  const resolved = new URL(url, baseUrl);
  if (resolved.origin !== new URL(baseUrl).origin) return null;
  return `${resolved.pathname}${resolved.search}`;
}

export function useAuthenticatedAttachmentUrl(url: string): string {
  const [objectUrl, setObjectUrl] = useState('');

  useEffect(() => {
    const path = attachmentAPIPath(url);
    if (!path) {
      setObjectUrl(resolveAttachmentUrl(url));
      return;
    }
    let disposed = false;
    let created = '';
    setObjectUrl('');
    apiClient.getBlob(path).then((blob) => {
      if (disposed) return;
      created = URL.createObjectURL(blob);
      setObjectUrl(created);
    }).catch(() => {
      if (!disposed) setObjectUrl('');
    });
    return () => {
      disposed = true;
      if (created) URL.revokeObjectURL(created);
    };
  }, [url]);

  return objectUrl;
}

export async function downloadAuthenticatedAttachment(url: string, filename: string): Promise<void> {
  const path = attachmentAPIPath(url);
  if (!path) {
    window.open(resolveAttachmentUrl(url), '_blank', 'noopener,noreferrer');
    return;
  }
  const objectUrl = URL.createObjectURL(await apiClient.getBlob(path));
  const anchor = document.createElement('a');
  anchor.href = objectUrl;
  anchor.download = filename;
  anchor.click();
  URL.revokeObjectURL(objectUrl);
}
