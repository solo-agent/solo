import type { TranslationKey } from '@/lib/i18n';

export const themeStorageKey = 'solo.skin';
export const defaultThemeId = 'archive';

export const themeOptions = [
  { id: 'archive', labelKey: 'themeArchive' },
  { id: 'classic', labelKey: 'themeClassic' },
] as const satisfies ReadonlyArray<{ id: string; labelKey: TranslationKey }>;

export type ThemeId = (typeof themeOptions)[number]['id'];

const themeIds = new Set<string>(themeOptions.map(({ id }) => id));

export function resolveThemeId(value: string | null | undefined): ThemeId {
  return value && themeIds.has(value) ? value as ThemeId : defaultThemeId;
}

export function getStoredTheme(): ThemeId {
  if (typeof window === 'undefined') return defaultThemeId;
  try {
    return resolveThemeId(window.localStorage.getItem(themeStorageKey));
  } catch {
    return defaultThemeId;
  }
}

export function setTheme(value: string): ThemeId {
  const theme = resolveThemeId(value);
  if (typeof document !== 'undefined') {
    document.documentElement.dataset.skin = theme;
  }
  if (typeof window !== 'undefined') {
    try {
      window.localStorage.setItem(themeStorageKey, theme);
    } catch {
      // The current page still changes when browser storage is unavailable.
    }
    window.dispatchEvent(new Event('solo:theme-change'));
  }
  return theme;
}
