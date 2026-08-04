import { apiClient } from '@/lib/api-client';

type PendingSend<T> = {
  attempts: number;
  inFlight: boolean;
  status: 'pending' | 'failed';
  timer: ReturnType<typeof setTimeout> | null;
  request: () => Promise<T>;
  onConfirmed: (result: T) => void;
  onFailed: () => void;
  onRetrying: () => void;
  resolve: (result: T | null) => void;
};

const pendingSends = new Map<string, PendingSend<unknown>>();

export function createClientMessageID(): string {
  return typeof crypto !== 'undefined' && crypto.randomUUID
    ? crypto.randomUUID()
    : `cm-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

export async function postMessageWithTimeout<T>(path: string, body: unknown): Promise<T> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 15_000);
  try {
    return await apiClient.post<T>(path, body, { signal: controller.signal });
  } finally {
    clearTimeout(timer);
  }
}

export function sendReliably<T>(
  clientMessageID: string,
  handlers: Omit<PendingSend<T>, 'attempts' | 'inFlight' | 'status' | 'timer' | 'resolve'>,
): Promise<T | null> {
  return new Promise((resolve) => {
    pendingSends.set(clientMessageID, {
      ...handlers,
      attempts: 0,
      inFlight: false,
      status: 'pending',
      timer: null,
      resolve,
    } as PendingSend<unknown>);
    void runSend(clientMessageID);
  });
}

export function acknowledgeReliableSend<T>(clientMessageID: string, result: T): boolean {
  const pending = pendingSends.get(clientMessageID) as PendingSend<T> | undefined;
  if (!pending) return false;

  pendingSends.delete(clientMessageID);
  if (pending.timer) clearTimeout(pending.timer);
  try {
    pending.onConfirmed(result);
  } finally {
    pending.resolve(result);
  }
  return true;
}

export function retryReliableSend(clientMessageID: string): void {
  const pending = pendingSends.get(clientMessageID);
  if (!pending || pending.inFlight) return;
  if (pending.timer) {
    clearTimeout(pending.timer);
    pending.timer = null;
  }
  void runSend(clientMessageID);
}

export function cancelReliableSend(clientMessageID: string): void {
  const pending = pendingSends.get(clientMessageID);
  if (!pending) return;
  pendingSends.delete(clientMessageID);
  if (pending.timer) clearTimeout(pending.timer);
  pending.resolve(null);
}

export function cancelAllReliableSends(): void {
  for (const clientMessageID of [...pendingSends.keys()]) {
    cancelReliableSend(clientMessageID);
  }
}

export function retryFailedReliableSends(): void {
  for (const [clientMessageID, pending] of pendingSends) {
    if (pending.status === 'failed') retryReliableSend(clientMessageID);
  }
}

async function runSend(clientMessageID: string): Promise<void> {
  const pending = pendingSends.get(clientMessageID);
  if (!pending || pending.inFlight) return;

  pending.inFlight = true;
  pending.status = 'pending';
  pending.attempts += 1;
  pending.onRetrying();
  try {
    const result = await pending.request();
    acknowledgeReliableSend(clientMessageID, result);
  } catch {
    const current = pendingSends.get(clientMessageID);
    if (current !== pending) return;
    pending.inFlight = false;
    pending.status = 'failed';
    pending.onFailed();
    if (pending.attempts < 3) {
      const delay = pending.attempts === 1 ? 3_000 : 2_000 * pending.attempts;
      pending.timer = setTimeout(() => retryReliableSend(clientMessageID), delay);
    }
  }
}
