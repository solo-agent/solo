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
      && Boolean(item.daemon_id)
    ));
    if (!computer) await new Promise((resolve) => setTimeout(resolve, 1000));
  } while (!computer && Date.now() < deadline);
  if (!computer) throw new Error('No online unpaired local Computer is available for E2E');

  if (!computer.my_role) {
    const claim = await request.post(`${apiBase}/api/v1/computers/${computer.id}/claim`, { headers });
    if (!claim.ok()) throw new Error(`Claim Computer: ${claim.status()} ${await claim.text()}`);
  }

  return {
    id: computer.id,
    name: computer.name,
    release: async (releaseRequest) => {
      const deletion = await releaseRequest.delete(`${apiBase}/api/v1/computers/${computer.id}`, { headers });
      if (!deletion.ok() && deletion.status() !== 404) {
        throw new Error(`Release Computer: ${deletion.status()} ${await deletion.text()}`);
      }
    },
  };
}
