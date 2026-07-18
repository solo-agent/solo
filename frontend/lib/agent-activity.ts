import { t, type TranslationKey } from '@/lib/i18n';
import type { AgentRunStatus } from '@/lib/agent-run-types';

const RUN_STATUS_COLOR: Record<AgentRunStatus | 'idle', string> = {
  queued: '#9ca3af',
  thinking: '#3B7DD8',
  running: '#0891b2',
  streaming: '#16a34a',
  waiting_input: '#f59e0b',
  waiting_approval: '#ea580c',
  completed: '#16a34a',
  failed: '#dc2626',
  cancelled: '#6b7280',
  timeout: '#be123c',
  idle: '#000000',
};

const ACTIVE_DOT_STATUSES = new Set<AgentRunStatus>(['thinking', 'running', 'streaming']);
const ACTIVE_HALO_STATUSES = new Set<AgentRunStatus>([
  'queued',
  'thinking',
  'running',
  'streaming',
  'waiting_input',
  'waiting_approval',
]);

export function agentRunStatusColor(status?: AgentRunStatus) {
  return RUN_STATUS_COLOR[status ?? 'idle'];
}

export function agentRunShowsDots(status?: AgentRunStatus) {
  return Boolean(status && ACTIVE_DOT_STATUSES.has(status));
}

export function agentRunShowsHalo(status?: AgentRunStatus) {
  return Boolean(status && ACTIVE_HALO_STATUSES.has(status));
}

const ACTIVITY_TEXT_KEYS: Record<string, TranslationKey> = {
  'agent.activity.accepted': 'agentActivityAccepted',
  'agent.activity.no_visible_reply': 'agentActivityNoVisibleReply',
  'agent.activity.no_progress': 'agentActivityNoProgress',
  'agent.activity.completed': 'agentActivityCompleted',
  'agent.activity.cancelled': 'agentActivityCancelled',
  'agent.activity.timeout': 'agentActivityTimeout',
  'agent.activity.failed': 'agentActivityFailed',
  '已接收，正在处理': 'agentActivityAccepted',
  '仍在运行，暂无可见回复': 'agentActivityNoVisibleReply',
  '仍在运行，暂无新的进度': 'agentActivityNoProgress',
  '已完成': 'agentActivityCompleted',
  '已取消': 'agentActivityCancelled',
  '执行超时': 'agentActivityTimeout',
  '执行失败': 'agentActivityFailed',
};

const AGENT_ERROR_KEYS: Record<string, TranslationKey> = {
  'agent.error.no_available_daemon': 'agentErrorNoAvailableDaemon',
  'No available daemon to run this agent.': 'agentErrorNoAvailableDaemon',
};

const GENERIC_ACTIVITY_TEXT = new Set([
  '等待执行',
  '执行中',
  '运行中',
  '思考中',
  '思考中…',
  '生成中',
  '失败',
  'thinking...',
  'generating...',
  'using tool',
  'error',
]);

export function agentRunStatusText(status?: AgentRunStatus): string {
  switch (status) {
  case 'queued':
    return t('runQueued');
  case 'thinking':
    return t('runThinking');
  case 'running':
    return t('runRunning');
  case 'streaming':
    return t('runStreaming');
  case 'waiting_input':
    return t('runWaitingInput');
  case 'waiting_approval':
    return t('runWaitingApproval');
  case 'completed':
    return t('runCompleted');
  case 'failed':
    return t('runFailed');
  case 'cancelled':
    return t('runCancelled');
  case 'timeout':
    return t('runTimeout');
  default:
    return t('agentIdle');
  }
}

export function displayAgentActivity(status: AgentRunStatus | undefined, activityText?: string | null, toolInputSummary?: string | null, fallback?: string): string {
  const summary = toolInputSummary?.trim();
  if (summary) return summary;

  const text = activityText?.trim();
  if (!text) return fallback ?? agentRunStatusText(status);

  const key = ACTIVITY_TEXT_KEYS[text];
  if (key) return t(key);

  if (GENERIC_ACTIVITY_TEXT.has(text.toLowerCase()) || GENERIC_ACTIVITY_TEXT.has(text)) {
    return fallback ?? agentRunStatusText(status);
  }

  return text;
}

export function displayAgentErrorReason(error?: string | null, detail?: string | null): string {
  const text = error?.trim() || detail?.trim();
  if (!text) return t('unexpectedError');
  const key = AGENT_ERROR_KEYS[text];
  return key ? t(key) : text;
}
