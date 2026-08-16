'use client';

import { useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { useAuth } from '@/lib/auth-context';
import { Spinner } from '@/components/ui/spinner';
import { t } from '@/lib/i18n';

export default function ObservabilityPage() {
  const router = useRouter();
  const { isAuthenticated, isLoading } = useAuth();

  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      router.push('/auth/login');
      return;
    }
    if (!isLoading && isAuthenticated) {
      router.replace('/observability/live');
    }
  }, [isAuthenticated, isLoading, router]);

  return (
    <div className="flex h-screen items-center justify-center bg-brutal-cream">
      <div className="flex flex-col items-center gap-3">
        <Spinner size="md" />
        <p className="font-body text-sm text-muted-foreground">{t('loading')}</p>
      </div>
    </div>
  );
}
