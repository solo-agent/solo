// ============================================================================
// Settings / Profile page — neubrutalist styling
// - card-brutal form, input-brutal, btn-brutal-primary
// - Display email and display_name
// - Loading / error / success states
// ============================================================================

'use client';

import { useEffect, useState, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { User, Mail, ArrowLeft, AlertCircle, CheckCircle2, LogOut } from 'lucide-react';
import { useAuth } from '@/lib/auth-context';
import { t } from '@/lib/i18n';
import { useUser } from '@/lib/hooks/use-user';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';
import { Spinner } from '@/components/ui/spinner';
import { BrutalAlert } from '@/components/ui/brutal-alert';

export default function SettingsPage() {
  const router = useRouter();
  const { isAuthenticated, isLoading: authLoading, logout } = useAuth();
  const {
    user,
    isLoading: userLoading,
    error: userError,
    updateDisplayName,
    isUpdating,
    successMessage,
    clearSuccess,
    refetch,
  } = useUser();

  const [displayName, setDisplayName] = useState('');
  const [loggingOut, setLoggingOut] = useState(false);

  const handleLogout = useCallback(async () => {
    setLoggingOut(true);
    try {
      await logout();
      router.push('/auth/login');
    } catch {
      setLoggingOut(false);
    }
  }, [logout, router]);

  useEffect(() => {
    if (user?.display_name) {
      setDisplayName(user.display_name);
    }
  }, [user?.display_name]);

  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      router.push('/auth/login');
    }
  }, [authLoading, isAuthenticated, router]);

  const handleSave = useCallback(async () => {
    const trimmed = displayName.trim();
    if (!trimmed) return;
    await updateDisplayName(trimmed);
  }, [displayName, updateDisplayName]);

  if (authLoading || !isAuthenticated) {
    return (
      <div className="flex h-screen items-center justify-center bg-brutal-cream">
        <div className="flex flex-col items-center gap-3">
          <Spinner size="md" />
          <p className="font-mono text-sm text-muted-foreground">{t('loading')}</p>
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-2xl px-6 py-8">
      {/* Back button */}
      <div className="mb-6">
        <Button
          variant="ghost"
          size="sm"
          onClick={() => router.push('/dashboard')}
          className="gap-1.5"
        >
          <ArrowLeft className="h-4 w-4" />
          {t('backToDashboard')}
        </Button>
      </div>

      {/* Page header — v3.2 (Phase 2): h1 gets sticker rotation +
          the title container uses card-brutal-heavy for hero weight. */}
      <div className="mb-8">
        <div className="flex items-center gap-3">
          <div
            className="flex h-10 w-10 items-center justify-center border-2 border-black bg-brutal-primary shadow-brutal-sm"
            style={{ transform: 'rotate(-3deg)' }}
          >
            <User className="h-6 w-6 text-white" />
          </div>
          <div>
            <h1
              className="font-heading text-2xl font-bold text-foreground"
              style={{ transform: 'rotate(-0.5deg)' }}
            >
              {t('settingsTitle')}
            </h1>
            <p className="mt-1 font-body text-sm text-muted-foreground">
              {t('settingsDesc')}
            </p>
          </div>
        </div>
      </div>

      {/* Error banner */}
      {userError && (
        <div className="mb-6 space-y-2">
          <BrutalAlert variant="error" className="p-4">
            {userError}
          </BrutalAlert>
          <Button variant="outline" size="sm" onClick={refetch}>
            {t('retry')}
          </Button>
        </div>
      )}

      {/* Loading state */}
      {userLoading && (
        <div className="space-y-6">
          <div className="card-brutal p-6">
            <div className="space-y-4">
              {[1, 2].map((i) => (
                <div key={i} className="space-y-2">
                  <Skeleton className="h-4 w-16 rounded-none" />
                  <Skeleton className={`h-10 rounded-none ${i === 1 ? 'w-full' : 'w-48'}`} />
                </div>
              ))}
              <Skeleton className="h-10 w-24 rounded-none" />
            </div>
          </div>
        </div>
      )}

      {/* Profile form — v3.2 (Phase 2): upgraded from card-brutal to
          card-brutal-heavy for hero weight (4px border + 12px shadow). */}
      {!userLoading && user && (
        <>
          <div className="card-brutal-heavy">
            {/* Email (read-only) */}
            <div className="border-b-2 border-black px-6 py-5">
              <Label className="font-mono text-[11px] font-bold uppercase tracking-wider text-muted-foreground">
                {t('email')}
              </Label>
              <div className="mt-2 flex items-center gap-2">
                <Mail className="h-4 w-4 text-muted-foreground" />
                <span className="font-body text-sm text-foreground">{user.email}</span>
              </div>
              <p className="mt-1 font-mono text-[11px] text-muted-foreground">{t('settingsEmailUnmodifiable')}</p>
            </div>

            {/* Display name (editable) */}
            <div className="px-6 py-5">
              <Label
                htmlFor="display-name"
                className="font-mono text-[11px] font-bold uppercase tracking-wider text-muted-foreground"
              >
                {t('settingsDisplayName')}
              </Label>
              <div className="mt-2 flex items-end gap-3">
                <div className="flex-1">
                  <Input
                    id="display-name"
                    value={displayName}
                    onChange={(e) => {
                      setDisplayName(e.target.value);
                      clearSuccess();
                    }}
                    placeholder={t('settingsDisplayNamePlaceholder')}
                    disabled={isUpdating}
                    className="max-w-sm"
                  />
                </div>
                <Button
                  type="button"
                  onClick={handleSave}
                  disabled={
                    isUpdating ||
                    !displayName.trim() ||
                    displayName.trim() === user.display_name
                  }
                  variant="default"
                >
                  {isUpdating ? t('settingsSaving') : t('settingsSave')}
                </Button>
              </div>

              {/* Feedback messages */}
              <div className="mt-3 space-y-1">
                {successMessage && (
                  <div className="flex items-center gap-1.5 font-mono text-xs text-brutal-success">
                    <CheckCircle2 className="h-4 w-4" />
                    <span>{successMessage}</span>
                  </div>
                )}
              </div>
            </div>
          </div>

          {/* Logout */}
          <div className="mt-6">
            <Button
              variant="outline"
              onClick={handleLogout}
              disabled={loggingOut}
              className="gap-2 border-red-500 text-red-600 hover:bg-red-50"
            >
              <LogOut className="h-4 w-4" />
              {loggingOut ? t('settingsLoggingOut') : t('settingsLogout')}
            </Button>
          </div>
        </>
      )}
    </div>
  );
}
