// ============================================================================
// SOLO-08-F: Auth Context — 认证上下文 Provider + useAuth Hook
// - 管理用户认证状态 (user, loading, error)
// - 提供 login / register / logout 方法
// - 初始化时自动检查 localStorage 中的 token 并尝试验证
// - Token 过期自动尝试 refresh
// - 与 api-client 共享 token 存储
// ============================================================================

'use client';

import {
  createContext,
  useContext,
  useCallback,
  useEffect,
  useMemo,
  useReducer,
  type ReactNode,
} from 'react';
import { ApiError, apiClient, defaultTokenStorage } from './api-client';

// ---- 类型定义 ----

export interface User {
  id: string;
  email: string;
  display_name: string;
  role: string;
  created_at: string;
  updated_at?: string;
}

export interface LoginRequest {
  email: string;
  password: string;
}

export interface RegisterRequest {
  email: string;
  password: string;
  display_name?: string;
}

export interface AuthResponse {
  access_token: string;
  refresh_token: string;
  user: User;
}

// ---- 状态管理 ----

interface AuthState {
  user: User | null;
  isLoading: boolean;
  error: string | null;
}

type AuthAction =
  | { type: 'AUTH_START' }
  | { type: 'AUTH_SUCCESS'; user: User }
  | { type: 'AUTH_FAILURE'; error: string }
  | { type: 'CLEAR_ERROR' }
  | { type: 'LOGOUT' }
  | { type: 'SET_USER'; user: User };

const initialState: AuthState = {
  user: null,
  isLoading: true,
  error: null,
};

function authReducer(state: AuthState, action: AuthAction): AuthState {
  switch (action.type) {
    case 'AUTH_START':
      return { ...state, isLoading: true, error: null };
    case 'AUTH_SUCCESS':
      return { user: action.user, isLoading: false, error: null };
    case 'AUTH_FAILURE':
      return { user: null, isLoading: false, error: action.error };
    case 'CLEAR_ERROR':
      return { ...state, error: null };
    case 'LOGOUT':
      return { user: null, isLoading: false, error: null };
    case 'SET_USER':
      return { ...state, user: action.user };
    default:
      return state;
  }
}

// ---- Context ----

export interface AuthContextValue {
  user: User | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  error: string | null;
  login: (req: LoginRequest) => Promise<void>;
  register: (req: RegisterRequest) => Promise<void>;
  logout: () => Promise<void>;
  clearError: () => void;
}

const AuthContext = createContext<AuthContextValue | null>(null);

// ---- Provider ----

interface AuthProviderProps {
  children: ReactNode;
}

export function AuthProvider({ children }: AuthProviderProps) {
  const [state, dispatch] = useReducer(authReducer, initialState);

  // ---- 初始化：检查 localStorage 中的 token ----

  useEffect(() => {
    const token = defaultTokenStorage.getAccessToken();
    const refreshToken = defaultTokenStorage.getRefreshToken();

    if (!token && !refreshToken) {
      dispatch({ type: 'AUTH_FAILURE', error: '' });
      return;
    }

    if (!token && refreshToken) {
      refreshAndFetchUser(dispatch);
      return;
    }

    // 有 access_token，尝试获取用户信息
    fetchCurrentUser()
      .then((user) => {
        dispatch({ type: 'AUTH_SUCCESS', user });
      })
      .catch((err: unknown) => {
        if (err instanceof ApiError && err.status === 401) {
          // Token 过期，尝试刷新
          refreshAndFetchUser(dispatch);
        } else {
          defaultTokenStorage.removeTokens();
          dispatch({
            type: 'AUTH_FAILURE',
            error: err instanceof Error ? err.message : '认证初始化失败',
          });
        }
      });
  }, []);

  // ---- Login ----

  const login = useCallback(async (req: LoginRequest) => {
    dispatch({ type: 'AUTH_START' });
    try {
      const data = await apiClient.post<AuthResponse>('/api/v1/auth/login', req);
      defaultTokenStorage.setAccessToken(data.access_token);
      localStorage.setItem('refresh_token', data.refresh_token);
      dispatch({ type: 'AUTH_SUCCESS', user: data.user });
    } catch (err: unknown) {
      const message =
        err instanceof ApiError ? err.message : '登录失败，请稍后再试';
      dispatch({ type: 'AUTH_FAILURE', error: message });
      throw err;
    }
  }, []);

  // ---- Register ----

  const register = useCallback(async (req: RegisterRequest) => {
    dispatch({ type: 'AUTH_START' });
    try {
      const data = await apiClient.post<AuthResponse>('/api/v1/auth/register', req);
      defaultTokenStorage.setAccessToken(data.access_token);
      localStorage.setItem('refresh_token', data.refresh_token);
      dispatch({ type: 'AUTH_SUCCESS', user: data.user });
    } catch (err: unknown) {
      const message =
        err instanceof ApiError ? err.message : '注册失败，请稍后再试';
      dispatch({ type: 'AUTH_FAILURE', error: message });
      throw err;
    }
  }, []);

  // ---- Logout ----

  const logout = useCallback(async () => {
    dispatch({ type: 'AUTH_START' });
    try {
      await apiClient.post('/api/v1/auth/logout');
    } catch {
      // 即使登出 API 失败，也清除本地 token
    }
    defaultTokenStorage.removeTokens();
    dispatch({ type: 'LOGOUT' });
  }, []);

  // ---- Clear Error ----

  const clearError = useCallback(() => {
    dispatch({ type: 'CLEAR_ERROR' });
  }, []);

  // ---- Context Value ----

  const value = useMemo<AuthContextValue>(
    () => ({
      user: state.user,
      isAuthenticated: state.user !== null,
      isLoading: state.isLoading,
      error: state.error,
      login,
      register,
      logout,
      clearError,
    }),
    [state, login, register, logout, clearError],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

// ---- Hook ----

export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth 必须在 AuthProvider 内使用');
  }
  return context;
}

// ---- 内部辅助函数 ----

async function fetchCurrentUser(): Promise<User> {
  return apiClient.get<User>('/api/v1/users/me');
}

async function refreshAndFetchUser(
  dispatch: React.Dispatch<AuthAction>,
): Promise<void> {
  const refreshToken = defaultTokenStorage.getRefreshToken();
  if (!refreshToken) {
    defaultTokenStorage.removeTokens();
    dispatch({ type: 'AUTH_FAILURE', error: '' });
    return;
  }

  try {
    const data = await apiClient.post<{ access_token: string; refresh_token: string }>(
      '/api/v1/auth/refresh',
      { refresh_token: refreshToken },
    );
    defaultTokenStorage.setAccessToken(data.access_token);
    if (data.refresh_token) {
      localStorage.setItem('refresh_token', data.refresh_token);
    }

    const user = await fetchCurrentUser();
    dispatch({ type: 'AUTH_SUCCESS', user });
  } catch {
    defaultTokenStorage.removeTokens();
    dispatch({ type: 'AUTH_FAILURE', error: '' });
  }
}
