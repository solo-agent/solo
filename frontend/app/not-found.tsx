import Link from 'next/link';
import { FileQuestion } from 'lucide-react';
import { t } from '@/lib/i18n';

export default function NotFoundPage() {
  return (
    <main id="main-content" className="relative flex min-h-screen items-center justify-center bg-brutal-cream px-4 py-16">
      <div className="pointer-events-none absolute inset-0 bg-grid opacity-20" aria-hidden />
      <div className="relative mx-auto flex max-w-md flex-col items-center rounded-2xl border border-black bg-white p-10 text-center shadow-brutal-lg">
        <div className="mb-6 flex h-20 w-20 items-center justify-center rounded-2xl border border-black bg-brutal-primary shadow-brutal-sm">
          <FileQuestion className="h-9 w-9" aria-hidden="true" />
        </div>
        <p className="font-mono text-xs font-bold uppercase tracking-[0.22em] text-muted-foreground">404</p>
        <h1 className="mt-2 font-heading text-3xl font-bold">{t('pageNotFound')}</h1>
        <p className="mt-3 font-body text-sm leading-6 text-muted-foreground">{t('pageNotFoundDesc')}</p>
        <div className="mt-8 flex w-full flex-col gap-3 sm:flex-row">
          <Link
            href="/dashboard"
            className="btn-brutal btn-brutal-primary flex-1 px-5 py-2.5 text-sm"
          >
            {t('backToDashboard')}
          </Link>
          <Link
            href="/auth/login"
            className="btn-brutal flex-1 bg-white px-5 py-2.5 text-sm"
          >
            {t('backToLogin')}
          </Link>
        </div>
      </div>
    </main>
  );
}
