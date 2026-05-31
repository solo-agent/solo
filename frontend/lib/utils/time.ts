// ============================================================================
// Relative time display helper — Chinese locale
// ============================================================================

/**
 * Returns a human-readable relative time string in Chinese.
 * Examples: "刚刚", "3 分钟前", "离线 2 小时", "3 天前", "2026-05-19"
 */
export function relativeTime(
  iso: string | null | undefined,
  /** When false, omits the "前" suffix for offline/elapsed contexts. Default true. */
  suffix = true,
): string {
  if (!iso) return '从未';

  const now = Date.now();
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return '未知';

  const diffMs = now - then;
  const diffSeconds = Math.floor(diffMs / 1000);
  const diffMinutes = Math.floor(diffSeconds / 60);
  const diffHours = Math.floor(diffMinutes / 60);
  const diffDays = Math.floor(diffHours / 24);

  if (diffSeconds < 60) return '刚刚';
  if (diffMinutes < 60) return `${diffMinutes} 分钟${suffix ? '前' : ''}`;
  if (diffHours < 24) return `${diffHours} 小时${suffix ? '前' : ''}`;
  if (diffDays < 7) return `${diffDays} 天${suffix ? '前' : ''}`;

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
  if (!iso) return '未知';

  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '未知';

  const pad = (n: number) => n.toString().padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}
