import { t } from '@/lib/i18n';

/**
 * Returns a human-readable relative time string.
 * Examples: "just now", "3 min ago", "2 hr", "3 days ago", "2026-05-19"
 */
export function relativeTime(
  iso: string | null | undefined,
  /** When false, omits the " ago" suffix for offline/elapsed contexts. Default true. */
  suffix = true,
): string {
  if (!iso) return t('never');

  const now = Date.now();
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return t('unknown');

  const diffMs = now - then;
  const diffSeconds = Math.floor(diffMs / 1000);
  const diffMinutes = Math.floor(diffSeconds / 60);
  const diffHours = Math.floor(diffMinutes / 60);
  const diffDays = Math.floor(diffHours / 24);

  if (diffSeconds < 60) return t('justNow');
  if (diffMinutes < 60) return suffix ? t('minutesAgo', { n: diffMinutes }) : t('minutes', { n: diffMinutes });
  if (diffHours < 24) return suffix ? t('hoursAgo', { n: diffHours }) : t('hours', { n: diffHours });
  if (diffDays < 7) return suffix ? t('daysAgo', { n: diffDays }) : t('days', { n: diffDays });

  // Fallback: YYYY-MM-DD
  const d = new Date(iso);
  const pad = (n: number) => n.toString().padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

/**
 * Returns a full Chinese datetime string.
 * Example: "2026-05-19 15:30:45"
 */
export function formatDateTime(iso: string | null | undefined): string {
  if (!iso) return t('unknown');

  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return t('unknown');

  const pad = (n: number) => n.toString().padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}
