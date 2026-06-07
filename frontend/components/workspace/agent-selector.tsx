'use client';

import { useEffect, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { apiClient } from '@/lib/api-client';
import { Select, type SelectOption } from '@/components/ui/select';

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

  const options: SelectOption[] = agents.map((a) => ({
    value: a.id,
    label: a.name,
  }));

  return (
    <Select
      value={agentId || ''}
      onChange={handleChange}
      options={options}
      placeholder="选择一个 Agent..."
      size="sm"
    />
  );
}
