import type { APIRequestContext, APIResponse } from '@playwright/test';

interface RegistrationOptions {
  data: { email: string; password: string; display_name?: string };
}

// Long real-runtime suites can outlive the access JWT. Refresh through the
// actual authentication service and retry only the rejected request once.
export async function requestAuthenticated(
  request: APIRequestContext,
  apiBase: string,
  auth: { access_token: string; refresh_token: string },
  method: 'get' | 'post' | 'patch' | 'delete',
  path: string,
  options: { headers: Record<string, string>; data?: unknown },
): Promise<APIResponse> {
  const send = () => {
    options.headers.authorization = `Bearer ${auth.access_token}`;
    return request[method](`${apiBase}${path}`, options);
  };
  const response = await send();
  if (response.status() !== 401 || path.startsWith('/api/v1/auth/')) return response;
  const refreshed = await request.post(`${apiBase}/api/v1/auth/refresh`, { data: { refresh_token: auth.refresh_token } });
  if (!refreshed.ok()) throw new Error(`E2E session refresh failed: ${refreshed.status()}`);
  const tokens = await refreshed.json() as { access_token: string; refresh_token: string };
  auth.access_token = tokens.access_token;
  auth.refresh_token = tokens.refresh_token;
  return send();
}

export async function registerVerified(
  request: APIRequestContext,
  apiBase: string,
  options: RegistrationOptions,
): Promise<APIResponse> {
  const post = async (path: string, data: unknown) => {
    let response = await request.post(`${apiBase}${path}`, { data });
    for (let attempt = 0; response.status() === 429 && attempt < 2; attempt += 1) {
      const retryAfter = Number(response.headers()['retry-after'] ?? '6');
      await new Promise((resolve) => setTimeout(resolve, Math.max(1, retryAfter) * 1000));
      response = await request.post(`${apiBase}${path}`, { data });
    }
    return response;
  };

  const pending = await post('/api/v1/auth/register', options.data);
  if (!pending.ok()) {
    throw new Error(`E2E registration request failed: ${pending.status()} ${await pending.text()}`);
  }
  const registration = await pending.json() as { verification_code?: string };
  return post('/api/v1/auth/register/verify', {
    email: options.data.email,
    code: registration.verification_code ?? process.env.SOLO_E2E_AUTH_CODE ?? '123456',
  });
}
