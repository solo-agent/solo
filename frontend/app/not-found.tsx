import Link from 'next/link';
import { t } from '@/lib/i18n';
import { EmptyState } from '@/components/ui/empty-state';

export default function NotFoundPage() {
  return (
    <main id="main-content" className="relative flex min-h-screen items-center justify-center bg-skin-canvas px-4 py-16">
      <div className="relative mx-auto w-full max-w-md">
        <EmptyState
          size="lg"
          illustration={{ src: '/illustrations/not-found.png', alt: t('pageNotFound') }}
          title={t('pageNotFound')}
          description={t('pageNotFoundDesc')}
        />
        <div className="mt-6 flex w-full flex-col gap-3 sm:flex-row">
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
