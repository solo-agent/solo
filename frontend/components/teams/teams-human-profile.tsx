// ============================================================================
// TeamsHumanProfile — User card shown when a human row is selected in the
// Teams left column. Read-only view of the current user (we only have one
// human in the system today; this is structured for future multi-user).
// ============================================================================

'use client';

import { useEffect, useState, useCallback } from 'react';
import { User, Mail, Calendar, AlertCircle, RefreshCw } from 'lucide-react';
import { apiClient, ApiError } from '@/lib/api-client';
import { Skeleton } from '@/components/ui/skeleton';

interface UserInfo {
  id: string;
  display_name: string;
  email?: string;
  created_at?: string;
}

interface TeamsHumanProfileProps {
  userId: string;
}

export function TeamsHumanProfile({ userId }: TeamsHumanProfileProps) {
  const [user, setUser] = useState<UserInfo | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      // /api/v1/users/me gives us the current user. If the userId doesn't
      // match, the row is treated as inaccessible (multi-user not yet supported).
      const me = await apiClient.get<UserInfo>('/api/v1/users/me');
      if (me.id !== userId) {
        setError('该用户不存在或当前不可访问');
        setUser(null);
      } else {
        setUser(me);
      }
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '加载用户信息失败');
    } finally {
      setIsLoading(false);
    }
  }, [userId]);

  useEffect(() => { void load(); }, [load]);

  if (isLoading) {
    return (
      <div className="space-y-3 p-6">
        <Skeleton className="h-16 w-full rounded-none" />
        <Skeleton className="h-6 w-1/2 rounded-none" />
        <Skeleton className="h-6 w-2/3 rounded-none" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center py-12">
        <div className="mb-3 flex h-12 w-12 items-center justify-center border-2 border-black bg-brutal-red-light shadow-brutal-sm">
          <AlertCircle className="h-6 w-6 text-brutal-red" />
        </div>
        <p className="font-body text-sm text-brutal-red">{error}</p>
        <button onClick={load} className="btn-brutal btn-brutal-sm mt-4">
          <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
          重试
        </button>
      </div>
    );
  }

  if (!user) return null;

  return (
    <div className="p-6">
      <div className="border-2 border-black bg-white p-6 shadow-brutal">
        <div className="flex items-center gap-4">
          <div className="flex h-16 w-16 items-center justify-center border-2 border-black bg-brutal-yellow text-2xl font-bold">
            <User className="h-8 w-8" />
          </div>
          <div>
            <h2 className="font-heading text-xl font-bold">{user.display_name}</h2>
            <p className="font-mono text-xs text-muted-foreground">@me</p>
          </div>
        </div>

        <div className="mt-6 space-y-3">
          {user.email && (
            <div className="flex items-center gap-3 border-2 border-black bg-brutal-cream p-3">
              <Mail className="h-4 w-4 text-muted-foreground" />
              <span className="font-body text-sm">{user.email}</span>
            </div>
          )}
          {user.created_at && (
            <div className="flex items-center gap-3 border-2 border-black bg-brutal-cream p-3">
              <Calendar className="h-4 w-4 text-muted-foreground" />
              <span className="font-body text-sm">
                注册于 {new Date(user.created_at).toLocaleDateString('zh-CN')}
              </span>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
