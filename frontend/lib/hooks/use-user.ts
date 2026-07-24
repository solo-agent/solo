// ============================================================================
// SOLO-59-F: useUser — fetch and update current user profile
// - GET /api/v1/users/me — fetch current user
// - PATCH /api/v1/users/me — update display_name
// - Loading / error / success states
// ============================================================================

'use client';

import { t } from '@/lib/i18n';
import { useState, useCallback } from 'react';
import { apiClient, ApiError } from '@/lib/api-client';
import { useAuth, type User } from '@/lib/auth-context';

interface UseUserResult {
  /** Current user data */
  user: User | null;
  /** Whether the profile is being fetched */
  isLoading: boolean;
  /** Error message if profile load failed */
  error: string | null;
  /** Update display_name — returns true on success */
  updateDisplayName: (displayName: string) => Promise<boolean>;
  /** Whether an update is in progress */
  isUpdating: boolean;
  /** Last update success message (clears after 3s) */
  successMessage: string | null;
  /** Clear success message */
  clearSuccess: () => void;
  /** Refetch user profile */
  refetch: () => Promise<void>;
}

export function useUser(): UseUserResult {
  const { user: authUser, updateProfile } = useAuth();
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [isUpdating, setIsUpdating] = useState(false);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);
  const [localUser, setLocalUser] = useState<User | null>(null);

  // Prefer local over auth user so UI reflects PATCH response immediately
  const user = localUser ?? authUser;

  const refetch = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const data = await apiClient.get<User>('/api/v1/users/me');
      setLocalUser(data);
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : `${t('userLoadError')}`;
      setError(msg);
    } finally {
      setIsLoading(false);
    }
  }, []);

  const updateDisplayName = useCallback(
    async (displayName: string): Promise<boolean> => {
      setIsUpdating(true);
      setError(null);
      setSuccessMessage(null);
      try {
        const updated = await updateProfile({
          display_name: displayName,
        });
        setLocalUser(updated);
        setSuccessMessage(`${t('authNameUpdated')}`);
        // Auto-clear success after 3s
        setTimeout(() => setSuccessMessage(null), 3000);
        return true;
      } catch (err) {
        const msg = err instanceof ApiError ? err.message : `${t('authUpdateError')}`;
        setError(msg);
        return false;
      } finally {
        setIsUpdating(false);
      }
    },
    [updateProfile],
  );

  const clearSuccess = useCallback(() => setSuccessMessage(null), []);

  return {
    user,
    isLoading,
    error,
    updateDisplayName,
    isUpdating,
    successMessage,
    clearSuccess,
    refetch,
  } as const;
}
