'use client';

import { Fragment, useEffect, useState, type ReactNode } from 'react';
import { getLocale, initLocaleFromStorage } from '@/lib/i18n';

export function LocaleHydrator({ children }: { children: ReactNode }) {
  const [locale, setLocale] = useState(getLocale());

  useEffect(() => {
    const sync = () => setLocale(getLocale());
    initLocaleFromStorage();
    sync();
    window.addEventListener('solo:locale-change', sync);
    return () => window.removeEventListener('solo:locale-change', sync);
  }, []);

  return <Fragment key={locale}>{children}</Fragment>;
}
