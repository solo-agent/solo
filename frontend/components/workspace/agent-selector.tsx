'use client';

import { useRouter, useSearchParams } from 'next/navigation';

interface AgentSelectorProps {
  agentId: string | null;
}

export function AgentSelector({ agentId }: AgentSelectorProps) {
  const router = useRouter();
  const searchParams = useSearchParams();

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
    </select>
  );
}
