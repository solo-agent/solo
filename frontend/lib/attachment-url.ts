const DEFAULT_API_BASE_URL = 'http://localhost:8080';

export function resolveAttachmentUrl(url: string): string {
  if (!url) return url;
  if (/^(https?:|data:|blob:)/i.test(url)) return url;

  const baseUrl = process.env.NEXT_PUBLIC_API_URL ?? DEFAULT_API_BASE_URL;
  return new URL(url, baseUrl).toString();
}
