'use client';

import { useEffect } from 'react';
import { getLocale, t } from '@/lib/i18n';

export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    console.error('[GlobalError]', error);
  }, [error]);

  return (
    <html lang={getLocale()}>
      <body className="min-h-screen bg-brutal-cream font-sans">
        <main id="main-content" className="flex min-h-screen items-center justify-center p-8">
          <div className="w-full max-w-lg rounded-2xl border border-black bg-white p-8 shadow-brutal-lg">
            <div className="mb-5 h-2 w-16 rounded-full bg-brutal-danger" aria-hidden="true" />
            <h1 className="mb-3 font-heading text-3xl font-bold">{t('somethingWentWrong')}</h1>
            <p className="mb-2 font-body text-base leading-7 text-muted-foreground">{t('unexpectedError')}</p>
            {error.digest && (
              <p className="font-mono text-xs text-black/40 mb-6">
                Error ID: {error.digest}
              </p>
            )}
            <button
              onClick={reset}
              className="btn-brutal btn-brutal-primary mt-6 w-full"
            >
              {t('retry')}
            </button>
          </div>
        </main>
      </body>
    </html>
  );
}
