import type { APIRequestContext, APIResponse } from '@playwright/test';

interface RegistrationOptions {
  data: { email: string; password: string; display_name?: string };
}

export async function registerVerified(
  request: APIRequestContext,
  apiBase: string,
  options: RegistrationOptions,
): Promise<APIResponse> {
  const pending = await request.post(`${apiBase}/api/v1/auth/register`, options);
  if (!pending.ok()) {
    throw new Error(`E2E registration request failed: ${pending.status()} ${await pending.text()}`);
  }
  return request.post(`${apiBase}/api/v1/auth/register/verify`, {
    data: {
      email: options.data.email,
      code: process.env.SOLO_E2E_AUTH_CODE ?? '123456',
    },
  });
}
