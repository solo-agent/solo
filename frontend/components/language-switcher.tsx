'use client';

import { Languages } from 'lucide-react';
import { getLocale, languageOptions, setLocale, t, type Locale } from '@/lib/i18n';

export function LanguageSwitcher() {
  const locale = getLocale();

  return (
    <label className="inline-flex items-center gap-2 border-2 border-black bg-white px-2 py-1.5 shadow-brutal-sm">
      <Languages className="h-4 w-4" aria-hidden />
      <span className="sr-only">{t('languageLabel')}</span>
      <select
        aria-label={t('languageLabel')}
        value={locale}
        onChange={(event) => setLocale(event.target.value as Locale)}
        className="bg-transparent font-body text-sm font-semibold outline-none"
      >
        {languageOptions.map((option) => (
          <option key={option.value} value={option.value}>{option.label}</option>
        ))}
      </select>
    </label>
  );
}
