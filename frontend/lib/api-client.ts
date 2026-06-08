// ============================================================================
// SOLO-08-F: API 客户端封装
// - fetch-based HTTP 客户端
// - 自动附加 JWT token (从 localStorage 读取)
// - 自动 refresh token (401 时用 refresh_token 换取新 access_token)
// - 401 + refresh 失败时自动跳转登录页
// - 统一错误处理 (网络错误、服务端错误、认证错误)
// - TypeScript 类型安全
// ============================================================================

import { t } from '@/lib/i18n';

// ---- Types ----

export interface ApiClientConfig {
  /** API 基地址，默认从 NEXT_PUBLIC_API_URL 环境变量读取 */
  baseUrl: string;
  /** 获取当前 access_token */
  getAccessToken: () => string | null;
  /** 获取当前 refresh_token */
  getRefreshToken: () => string | null;
  /** 刷新 token 成功后回调，用于持久化新 token */
  onTokenRefreshed: (accessToken: string) => void;
  /** token 过期且刷新失败时回调，用于跳转登录页 */
  onAuthFailure: () => void;
}

export class ApiError extends Error {
  public readonly status: number;
  public readonly code: string;

  constructor(message: string, status: number, code: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
  }

  /** 是否为客户端错误 (4xx) */
  get isClientError(): boolean {
    return this.status >= 400 && this.status < 500;
  }

  /** 是否为服务端错误 (5xx) */
  get isServerError(): boolean {
    return this.status >= 500;
  }

  /** 是否为认证错误 */
  get isAuthError(): boolean {
    return this.status === 401;
  }

  /** 是否为权限错误 */
  get isForbidden(): boolean {
    return this.status === 403;
  }

  /** 是否为资源冲突 */
  get isConflict(): boolean {
    return this.status === 409;
  }

  /** 是否为限流 */
  get isRateLimited(): boolean {
    return this.status === 429;
  }
}

export interface ApiResponseBody<T = unknown> {
  data?: T;
  message?: string;
  code?: string;
}

// ---- Defaults ----

const STORAGE_KEY_ACCESS_TOKEN = 'access_token';
const STORAGE_KEY_REFRESH_TOKEN = 'refresh_token';

/** 默认的 localStorage token 读取/写入函数 */
export const defaultTokenStorage = {
  getAccessToken: (): string | null => localStorage.getItem(STORAGE_KEY_ACCESS_TOKEN),
  getRefreshToken: (): string | null => localStorage.getItem(STORAGE_KEY_REFRESH_TOKEN),
  setAccessToken: (token: string): void => {
    localStorage.setItem(STORAGE_KEY_ACCESS_TOKEN, token);
  },
  setRefreshToken: (token: string): void => {
    localStorage.setItem(STORAGE_KEY_REFRESH_TOKEN, token);
  },
  removeTokens: (): void => {
    localStorage.removeItem(STORAGE_KEY_ACCESS_TOKEN);
    localStorage.removeItem(STORAGE_KEY_REFRESH_TOKEN);
  },
};

// ---- ApiClient ----

export class ApiClient {
  private readonly config: ApiClientConfig;
  /** 正在进行的 refresh 请求 */
  private refreshPromise: Promise<string | null> | null = null;

  constructor(config: ApiClientConfig) {
    this.config = config;
  }

  // ---- HTTP Method 便捷方法 ----

  async get<T>(path: string, params?: Record<string, string>): Promise<T> {
    const url = params ? this.buildUrlWithParams(path, params) : path;
    return this.request<T>(url, { method: 'GET' });
  }

  async post<T>(path: string, body?: unknown): Promise<T> {
    return this.request<T>(path, {
      method: 'POST',
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  }

  async patch<T>(path: string, body: unknown): Promise<T> {
    return this.request<T>(path, {
      method: 'PATCH',
      body: JSON.stringify(body),
    });
  }

  async delete<T>(path: string): Promise<T> {
    return this.request<T>(path, { method: 'DELETE' });
  }

  /**
   * POST with FormData body (multipart upload).
   * Unlike post(), does NOT JSON.stringify the body — passes FormData raw
   * so the browser sets Content-Type with correct boundary.
   */
  async postFormData<T>(path: string, formData: FormData): Promise<T> {
    return this.request<T>(path, {
      method: 'POST',
      body: formData,
    });
  }

  // ---- 核心请求方法 ----

  private async request<T>(
    path: string,
    options: RequestInit,
    /** 内部重试计数，外部调用不应传入 */
    retryCount = 0,
  ): Promise<T> {
    const url = `${this.config.baseUrl}${path}`;
    const accessToken = this.config.getAccessToken();

    const headers: Record<string, string> = {};
    // 合并自定义 headers
    if (options.headers) {
      const entries = options.headers instanceof Headers
        ? Array.from(options.headers.entries())
        : Array.isArray(options.headers)
          ? options.headers
          : Object.entries(options.headers as Record<string, string>);
      for (const [key, value] of entries) {
        headers[key] = value;
      }
    }

    // 自动附加 JWT token
    if (accessToken) {
      headers['Authorization'] = `Bearer ${accessToken}`;
    }

    // 自动设置 Content-Type（非 FormData 时）
    if (!headers['Content-Type'] && !(options.body instanceof FormData)) {
      headers['Content-Type'] = 'application/json';
    }

    let response: Response;

    try {
      response = await fetch(url, { ...options, headers });
    } catch (err) {
      // 网络错误（断网、DNS 解析失败等）
      throw new ApiError(
        t('apiNetworkError'),
        0,
        'NETWORK_ERROR',
      );
    }

    // ---- 401 处理：自动 refresh token 并重试 ----
    if (response.status === 401 && retryCount === 0) {
      const newToken = await this.refreshAccessToken();

      if (newToken) {
        // 重试原请求（仅重试一次，避免死循环）
        headers['Authorization'] = `Bearer ${newToken}`;
        const retryResponse = await fetch(url, { ...options, headers });
        return this.processResponse<T>(retryResponse);
      }

      // Refresh 失败，触发登出
      this.config.onAuthFailure();
      throw new ApiError(t('apiAuthExpired'), 401, 'UNAUTHORIZED');
    }

    return this.processResponse<T>(response);
  }

  // ---- 响应处理 ----

  private async processResponse<T>(response: Response): Promise<T> {
    if (!response.ok) {
      const body = await this.safeParseJson(response);
      const message = body?.message || this.defaultErrorMessage(response.status);
      const code = body?.code || this.defaultErrorCode(response.status);
      throw new ApiError(message, response.status, code);
    }

    // 204 No Content
    if (response.status === 204) {
      return undefined as unknown as T;
    }

    // 空响应体
    const text = await response.text();
    if (!text) {
      return undefined as unknown as T;
    }

    return JSON.parse(text) as T;
  }

  // ---- Token 刷新 (带并发去重) ----

  private async refreshAccessToken(): Promise<string | null> {
    // 如果已经有一个 refresh 请求在进行中，复用该 promise
    if (this.refreshPromise) {
      return this.refreshPromise;
    }

    this.refreshPromise = this.executeRefresh();
    try {
      return await this.refreshPromise;
    } finally {
      this.refreshPromise = null;
    }
  }

  private async executeRefresh(): Promise<string | null> {
    const refreshToken = this.config.getRefreshToken();
    if (!refreshToken) {
      return null;
    }

    const url = `${this.config.baseUrl}/api/v1/auth/refresh`;

    try {
      const response = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refresh_token: refreshToken }),
      });

      if (!response.ok) {
        return null;
      }

      const body = await response.json() as ApiResponseBody<{ access_token: string; refresh_token: string }>;

      // 兼容两种响应格式：{ access_token, refresh_token } 或 { data: { access_token, refresh_token } }
      const payload = body.data || (body as unknown as { access_token: string; refresh_token: string });
      const newAccessToken = payload.access_token;
      const newRefreshToken = payload.refresh_token;

      if (newAccessToken) {
        if (newRefreshToken) {
          localStorage.setItem(STORAGE_KEY_REFRESH_TOKEN, newRefreshToken);
        }
        this.config.onTokenRefreshed(newAccessToken);
        return newAccessToken;
      }

      return null;
    } catch {
      // 网络错误时不抛异常，让调用方处理
      return null;
    }
  }

  // ---- 辅助方法 ----

  private buildUrlWithParams(path: string, params: Record<string, string>): string {
    const searchParams = new URLSearchParams(params);
    return `${path}?${searchParams.toString()}`;
  }

  private async safeParseJson(response: Response): Promise<ApiResponseBody | null> {
    try {
      return await response.json() as ApiResponseBody;
    } catch {
      return null;
    }
  }

  private defaultErrorMessage(status: number): string {
    switch (status) {
      case 400: return t('apiBadRequest');
      case 401: return t('apiUnauthorized');
      case 403: return t('apiForbidden');
      case 404: return t('apiNotFound');
      case 409: return t('apiConflict');
      case 429: return t('apiTooManyRequests');
      case 500: return t('apiInternalError');
      case 502: return t('apiBadGateway');
      case 503: return t('apiServiceUnavailable');
      default: return t('apiDefaultError', { n: status });
    }
  }

  private defaultErrorCode(status: number): string {
    switch (status) {
      case 400: return 'INVALID_INPUT';
      case 401: return 'UNAUTHORIZED';
      case 403: return 'FORBIDDEN';
      case 404: return 'NOT_FOUND';
      case 409: return 'CONFLICT';
      case 429: return 'RATE_LIMITED';
      case 500: return 'INTERNAL_ERROR';
      default: return 'UNKNOWN_ERROR';
    }
  }
}

// ---- 客户端单例创建 ----

/**
 * 创建 API 客户端实例。
 * 建议在应用启动时调用一次，导出单例供全局使用。
 */
export function createApiClient(config?: Partial<ApiClientConfig>): ApiClient {
  return new ApiClient({
    baseUrl: config?.baseUrl ?? process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080',
    getAccessToken: config?.getAccessToken ?? defaultTokenStorage.getAccessToken,
    getRefreshToken: config?.getRefreshToken ?? defaultTokenStorage.getRefreshToken,
    onTokenRefreshed: config?.onTokenRefreshed ?? defaultTokenStorage.setAccessToken,
    onAuthFailure:
      config?.onAuthFailure ??
      (() => {
        defaultTokenStorage.removeTokens();
        window.location.href = '/auth/login';
      }),
  });
}

// ---- 统一 token 写入入口 ----

/**
 * 同时写入 access_token 和 refresh_token。
 * 所有 token 写入都应通过此函数，避免各处直接操作 localStorage。
 */
export function setAuthTokens(access: string, refresh: string): void {
  defaultTokenStorage.setAccessToken(access);
  defaultTokenStorage.setRefreshToken(refresh);
}

/**
 * 清除本地保存的 access_token 和 refresh_token。
 */
export function clearAuthTokens(): void {
  defaultTokenStorage.removeTokens();
}

// ---- 全局单例 ----
// 应用启动时初始化，可在任何模块中引用
export const apiClient = createApiClient();
