'use client';

import { useEffect, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { apiClient } from '@/lib/api-client';

interface AgentSummary {
  id: string;
  name: string;
}

interface AgentSelectorProps {
  agentId: string | null;
}

export function AgentSelector({ agentId }: AgentSelectorProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [agents, setAgents] = useState<AgentSummary[]>([]);

  useEffect(() => {
    apiClient.get<AgentSummary[]>('/api/v1/agents')
      .then(setAgents)
      .catch(() => setAgents([]));
  }, []);

  const handleChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    const newId = e.target.value;
    const params = new URLSearchParams(searchParams.toString());
    if (newId) {
      params.set('agent', newId);
    } else {
      params.delete('agent');
    }
    router.push(`/workspace?${params.toString()}`);
  };

  return (
    <select
      value={agentId || ''}
      onChange={handleChange}
      className="bg-black border-2 border-black text-white px-3 py-1.5 font-mono text-xs rounded-none focus:outline-none focus:border-brutal-pink"
    >
      <option value="">选择一个 Agent...</option>
      {agents.map((a) => (
        <option key={a.id} value={a.id}>{a.name}</option>
      ))}
    </select>
  );
}
