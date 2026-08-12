import type { APIRequestContext } from '@playwright/test';

interface Computer {
  id: string;
  name: string;
  daemon_id?: string;
  my_role?: string;
  pairing_status: string;
  status: string;
}

export interface LocalComputerLease {
  id: string;
  name: string;
  release: (request: APIRequestContext) => Promise<void>;
}

export async function acquireLocalComputer(
  request: APIRequestContext,
  apiBase: string,
  token: string,
): Promise<LocalComputerLease> {
  const expectedDaemonID = process.env.SOLO_E2E_DAEMON_ID?.trim();
  if (!expectedDaemonID?.startsWith('daemon-e2e-')) {
    throw new Error('SOLO_E2E_DAEMON_ID must identify an isolated daemon-e2e-* Computer');
  }
  const headers = { authorization: `Bearer ${token}` };
  const deadline = Date.now() + 40000;
  let computer: Computer | undefined;
  do {
    const response = await request.get(`${apiBase}/api/v1/computers`, { headers });
    if (!response.ok()) throw new Error(`List computers: ${response.status()} ${await response.text()}`);
    const computers = await response.json() as Computer[];
    computer = computers.find((item) => (
      item.status === 'online'
      && item.pairing_status === 'unpaired'
      && item.daemon_id === expectedDaemonID
    ));
    if (!computer) await new Promise((resolve) => setTimeout(resolve, 1000));
  } while (!computer && Date.now() < deadline);
  if (!computer) throw new Error(`No online unpaired Computer exists for E2E Daemon ${expectedDaemonID}`);

  if (!computer.my_role) {
    const claim = await request.post(`${apiBase}/api/v1/computers/${computer.id}/claim`, { headers });
    if (!claim.ok()) throw new Error(`Claim Computer: ${claim.status()} ${await claim.text()}`);
  }

  return {
    id: computer.id,
    name: computer.name,
    release: async (releaseRequest) => {
      if (computer.daemon_id !== expectedDaemonID) {
        throw new Error(`Refusing to delete non-E2E Computer ${computer.id}`);
      }
      const deletion = await releaseRequest.delete(`${apiBase}/api/v1/computers/${computer.id}`, { headers });
      if (deletion.ok()) return;
      if (deletion.status() === 404) {
        const list = await releaseRequest.get(`${apiBase}/api/v1/computers`, { headers });
        if (list.ok()) {
          const remaining = await list.json() as Computer[];
          if (!remaining.some((item) => item.id === computer.id)) return;
        }
      }
      if (!deletion.ok()) {
        throw new Error(`Release Computer: ${deletion.status()} ${await deletion.text()}`);
      }
    },
  };
}
