import type { APIRequestContext, APIResponse } from '@playwright/test';

interface RegistrationOptions {
  data: { email: string; password: string; display_name?: string };
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
