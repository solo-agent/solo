// ============================================================================
// Settings / Profile page — neubrutalist styling
// - card-brutal form, input-brutal, btn-brutal-primary
// - Display email and display_name
// - Loading / error / success states
// ============================================================================

'use client';

import { useEffect, useState, useCallback, useRef } from 'react';
import { useRouter } from 'next/navigation';
import { User, Mail, ArrowLeft, LogOut, Globe2, Palette, Check, Upload, RotateCcw } from 'lucide-react';
import { useAuth } from '@/lib/auth-context';
import { getLocale, languageOptions, setLocale, t, type Locale } from '@/lib/i18n';
import { defaultThemeId, getStoredTheme, setTheme, themeOptions, type ThemeId } from '@/lib/theme';
import { apiClient, ApiError } from '@/lib/api-client';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { Select } from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { Spinner } from '@/components/ui/spinner';
import {
  USER_AVATAR_PRESET_COUNT,
  USER_AVATAR_PRESET_PREFIX,
  UserAvatar,
  userAvatarPresetIndex,
  userAvatarPresetValue,
} from '@/components/ui/user-avatar';

export default function SettingsPage() {
  const router = useRouter();
  const { user, isAuthenticated, isLoading: authLoading, logout, updateProfile } = useAuth();
  const avatarInputRef = useRef<HTMLInputElement>(null);

  const [loggingOut, setLoggingOut] = useState(false);
  const [language, setLanguage] = useState<Locale>('en');
  const [theme, setThemeState] = useState<ThemeId>(defaultThemeId);
  const [avatarSaving, setAvatarSaving] = useState<string | null>(null);
  const [avatarError, setAvatarError] = useState<string | null>(null);

  useEffect(() => {
    setLanguage(getLocale());
    setThemeState(getStoredTheme());
  }, []);

  const handleLogout = useCallback(async () => {
    setLoggingOut(true);
    try {
      await logout();
      router.push('/auth/login');
    } catch {
      setLoggingOut(false);
    }
  }, [logout, router]);

  const handleLanguageChange = useCallback((next: string) => {
    if (setLocale(next)) {
      setLanguage(getLocale());
    }
  }, []);

  const chooseAvatarPreset = useCallback(async (index: number) => {
    setAvatarSaving(`preset-${index}`);
    setAvatarError(null);
    try {
      await updateProfile({ avatar_url: userAvatarPresetValue(index) });
    } catch (error) {
      setAvatarError(error instanceof ApiError ? error.message : t('settingsAvatarSaveError'));
    } finally {
      setAvatarSaving(null);
    }
  }, [updateProfile]);

  const uploadAvatar = useCallback(async (file: File) => {
    if (!['image/jpeg', 'image/png', 'image/webp'].includes(file.type)) {
      setAvatarError(t('settingsAvatarTypeError'));
      return;
    }
    if (file.size > 5 * 1024 * 1024) {
      setAvatarError(t('settingsAvatarSizeError'));
      return;
    }

    setAvatarSaving('upload');
    setAvatarError(null);
    try {
      const formData = new FormData();
      formData.append('file', file);
      const uploaded = await apiClient.postFormData<{
        url: string;
        thumbnail_url?: string;
      }>('/api/v1/attachments/upload', formData);
      await updateProfile({ avatar_url: uploaded.thumbnail_url || uploaded.url });
    } catch (error) {
      setAvatarError(error instanceof ApiError ? error.message : t('settingsAvatarUploadError'));
    } finally {
      setAvatarSaving(null);
      if (avatarInputRef.current) avatarInputRef.current.value = '';
    }
  }, [updateProfile]);

  const resetAvatar = useCallback(async () => {
    setAvatarSaving('reset');
    setAvatarError(null);
    try {
      await updateProfile({ avatar_url: null });
    } catch (error) {
      setAvatarError(error instanceof ApiError ? error.message : t('settingsAvatarSaveError'));
    } finally {
      setAvatarSaving(null);
    }
  }, [updateProfile]);

  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      router.push('/auth/login');
    }
  }, [authLoading, isAuthenticated, router]);

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
    <div className="mx-auto max-w-4xl px-6 py-8">
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

      {/* Loading state */}
      {authLoading && (
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
      {!authLoading && user && (
        <>
          <div className="card-brutal-heavy">
            {/* Avatar contact sheet */}
            <div className="border-b-2 border-black bg-brutal-cream px-6 py-5">
              <Label className="font-mono text-[11px] font-bold uppercase tracking-wider text-muted-foreground">
                {t('settingsAvatar')}
              </Label>
              <div className="mt-3 grid gap-5 md:grid-cols-[auto_1fr] md:items-start">
                <div className="flex items-center gap-3 md:flex-col md:items-start">
                  <UserAvatar
                    userId={user.id}
                    name={user.display_name}
                    avatarUrl={user.avatar_url}
                    size="lg"
                  />
                  <div>
                    <div className="font-heading text-sm font-black">{user.display_name}</div>
                    <div className="font-mono text-[10px] font-bold uppercase tracking-wider text-black/45">
                      {user.avatar_url && !user.avatar_url.startsWith(USER_AVATAR_PRESET_PREFIX)
                        ? t('settingsAvatarCustom')
                        : t('settingsAvatarPixel')}
                    </div>
                  </div>
                </div>

                <div>
                  <p className="font-body text-sm text-muted-foreground">{t('settingsAvatarHint')}</p>
                  <div className="mt-3 grid grid-cols-6 gap-2" aria-label={t('settingsAvatarPresets')}>
                    {Array.from({ length: USER_AVATAR_PRESET_COUNT }, (_, index) => {
                      const selected = (
                        (!user.avatar_url || user.avatar_url.startsWith(USER_AVATAR_PRESET_PREFIX))
                        && userAvatarPresetIndex(user.id, user.avatar_url) === index
                      );
                      return (
                        <button
                          key={index}
                          type="button"
                          aria-label={t('settingsAvatarPreset', { n: index + 1 })}
                          aria-pressed={selected}
                          disabled={avatarSaving !== null}
                          onClick={() => chooseAvatarPreset(index)}
                          className={`relative flex aspect-square items-center justify-center border-2 border-black bg-white transition-[transform,box-shadow] ${
                            selected
                              ? '-translate-y-0.5 shadow-brutal'
                              : 'shadow-brutal-sm hover:-translate-y-0.5 hover:shadow-brutal'
                          } disabled:cursor-wait disabled:opacity-60`}
                        >
                          <UserAvatar
                            userId={user.id}
                            name={user.display_name}
                            avatarUrl={userAvatarPresetValue(index)}
                            size="md"
                            className="border-0 shadow-none"
                          />
                          {selected && (
                            <span className="absolute -right-1 -top-1 flex h-4 w-4 items-center justify-center border border-black bg-brutal-success">
                              <Check className="h-3 w-3" aria-hidden="true" />
                            </span>
                          )}
                        </button>
                      );
                    })}
                  </div>

                  <div className="mt-4 flex flex-wrap gap-2">
                    <input
                      ref={avatarInputRef}
                      type="file"
                      className="sr-only"
                      accept="image/jpeg,image/png,image/webp"
                      onChange={(event) => {
                        const file = event.target.files?.[0];
                        if (file) void uploadAvatar(file);
                      }}
                    />
                    <Button
                      type="button"
                      size="sm"
                      onClick={() => avatarInputRef.current?.click()}
                      disabled={avatarSaving !== null}
                      className="gap-2"
                    >
                      {avatarSaving === 'upload' ? <Spinner size="sm" /> : <Upload className="h-4 w-4" />}
                      {t('settingsAvatarUpload')}
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={resetAvatar}
                      disabled={avatarSaving !== null || !user.avatar_url}
                      className="gap-2"
                    >
                      <RotateCcw className="h-4 w-4" />
                      {t('settingsAvatarReset')}
                    </Button>
                  </div>
                  {avatarError && (
                    <p role="alert" className="mt-3 font-mono text-xs font-bold text-brutal-danger">
                      {avatarError}
                    </p>
                  )}
                </div>
              </div>
            </div>

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

            {/* Display name (read-only) */}
            <div className="px-6 py-5">
              <Label className="font-mono text-[11px] font-bold uppercase tracking-wider text-muted-foreground">
                {t('settingsDisplayName')}
              </Label>
              <div className="mt-2 flex items-center gap-2">
                <User className="h-4 w-4 text-muted-foreground" />
                <span className="font-body text-sm text-foreground">{user.display_name}</span>
              </div>
              <p className="mt-1 font-mono text-[11px] text-muted-foreground">{t('settingsDisplayNameHint')}</p>
            </div>

            {/* Language */}
            <div className="border-t-2 border-black px-6 py-5">
              <Label className="font-mono text-[11px] font-bold uppercase tracking-wider text-muted-foreground">
                {t('settingsLanguage')}
              </Label>
              <div className="mt-2 flex items-center gap-2">
                <Globe2 className="h-4 w-4 text-muted-foreground" />
                <Select
                  options={languageOptions}
                  value={language}
                  onChange={handleLanguageChange}
                  size="md"
                  className="w-40"
                  aria-label={t('settingsLanguage')}
                />
              </div>
              <p className="mt-1 font-mono text-[11px] text-muted-foreground">{t('settingsLanguageHint')}</p>
            </div>

            {/* Skin */}
            <div className="border-t-2 border-black px-6 py-5">
              <Label className="flex items-center gap-2 font-mono text-[11px] font-bold uppercase tracking-wider text-muted-foreground">
                <Palette className="h-4 w-4" aria-hidden="true" />
                {t('settingsTheme')}
              </Label>
              <p className="mt-1 font-mono text-[11px] text-muted-foreground">{t('settingsThemeHint')}</p>
              <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
                {themeOptions.map((option) => {
                  const selected = theme === option.id;
                  return (
                    <button
                      key={option.id}
                      type="button"
                      data-skin-preview={option.id}
                      aria-pressed={selected}
                      onClick={() => setThemeState(setTheme(option.id))}
                      className={`overflow-hidden border-2 border-black bg-white text-left font-heading text-xs font-bold text-black transition-[transform,box-shadow] ${
                        selected
                          ? '-translate-y-0.5 shadow-brutal'
                          : 'shadow-brutal-sm hover:-translate-y-0.5 hover:shadow-brutal'
                      }`}
                    >
                      <span className="grid grid-cols-4 border-b-2 border-black" aria-hidden="true">
                        <span className="h-8 bg-brutal-primary" />
                        <span className="h-8 bg-brutal-accent" />
                        <span className="h-8 bg-brutal-info" />
                        <span className="h-8 bg-brutal-success" />
                      </span>
                      <span className="flex items-center justify-between gap-2 px-3 py-2">
                        <span>{t(option.labelKey)}</span>
                        {selected && <Check className="h-4 w-4 flex-shrink-0" aria-hidden="true" />}
                      </span>
                    </button>
                  );
                })}
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
