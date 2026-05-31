// ============================================================================
// Settings / Profile page — neubrutalist styling
// - card-brutal form, input-brutal, btn-brutal-pink
// - Display email and display_name
// - Loading / error / success states
// ============================================================================

'use client';

import { useEffect, useState, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { User, Mail, ArrowLeft, AlertCircle, CheckCircle2 } from 'lucide-react';
import { useAuth } from '@/lib/auth-context';
import { useUser } from '@/lib/hooks/use-user';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';

export default function SettingsPage() {
  const router = useRouter();
  const { isAuthenticated, isLoading: authLoading } = useAuth();
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
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-brutal-pink border-t-transparent" />
          <p className="font-mono text-sm text-muted-foreground">加载中...</p>
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
          返回工作台
        </Button>
      </div>

      {/* Page header */}
      <div className="mb-8">
        <div className="flex items-center gap-3">
          <div className="flex h-12 w-12 items-center justify-center border-2 border-black bg-brutal-pink shadow-brutal-sm">
            <User className="h-6 w-6 text-white" />
          </div>
          <div>
            <h1 className="font-heading text-2xl font-bold text-foreground">
              个人设置
            </h1>
            <p className="mt-1 font-body text-sm text-muted-foreground">
              管理你的个人信息
            </p>
          </div>
        </div>
      </div>

      {/* Error banner */}
      {userError && (
        <div className="mb-6 flex items-center gap-3 border-2 border-brutal-red bg-brutal-red-light p-4 shadow-brutal-sm">
          <AlertCircle className="h-5 w-5 flex-shrink-0 text-brutal-red" />
          <span className="flex-1 font-body text-sm text-foreground">
            {userError}
          </span>
          <button
            type="button"
            onClick={refetch}
            className="btn-brutal btn-brutal-sm"
          >
            重试
          </button>
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

      {/* Profile form */}
      {!userLoading && user && (
        <div className="card-brutal">
          {/* Email (read-only) */}
          <div className="border-b-2 border-black px-6 py-5">
            <Label className="font-mono text-[11px] font-bold uppercase tracking-wider text-muted-foreground">
              邮箱
            </Label>
            <div className="mt-2 flex items-center gap-2">
              <Mail className="h-4 w-4 text-muted-foreground" />
              <span className="font-body text-sm text-foreground">{user.email}</span>
            </div>
            <p className="mt-1 font-mono text-[11px] text-muted-foreground">邮箱不可修改</p>
          </div>

          {/* Display name (editable) */}
          <div className="px-6 py-5">
            <Label
              htmlFor="display-name"
              className="font-mono text-[11px] font-bold uppercase tracking-wider text-muted-foreground"
            >
              显示名称
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
                  placeholder="输入你的显示名称"
                  disabled={isUpdating}
                  className="max-w-sm"
                />
              </div>
              <button
                type="button"
                onClick={handleSave}
                disabled={
                  isUpdating ||
                  !displayName.trim() ||
                  displayName.trim() === user.display_name
                }
                className="btn-brutal btn-brutal-pink"
              >
                {isUpdating ? '保存中...' : '保存'}
              </button>
            </div>

            {/* Feedback messages */}
            <div className="mt-3 space-y-1">
              {successMessage && (
                <div className="flex items-center gap-1.5 font-mono text-xs text-brutal-lime">
                  <CheckCircle2 className="h-4 w-4" />
                  <span>{successMessage}</span>
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
