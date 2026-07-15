export type AgentRunStatus =
  | 'queued'
  | 'thinking'
  | 'running'
  | 'streaming'
  | 'waiting_input'
  | 'waiting_approval'
  | 'completed'
  | 'failed'
  | 'cancelled'
  | 'timeout';
