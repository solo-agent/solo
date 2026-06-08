// ============================================================================
// inferAgentGroup — keyword-based heuristic to bucket an agent into a role.
// Reused by teams-graph-view (structure layout) and teams-left-column (if
// we ever want to show role in the row). For now, only graph-view uses it.
// ============================================================================

export type TeamGroup =
  | 'Orchestrator'
  | 'Project Manager'
  | 'Backend'
  | 'Frontend'
  | 'QA'
  | 'Custom';

export const GROUP_ORDER: TeamGroup[] = [
  'Orchestrator',
  'Project Manager',
  'Backend',
  'Frontend',
  'QA',
  'Custom',
];

export const GROUP_DESCRIPTIONS: Record<TeamGroup, string> = {
  'Orchestrator': 'Responsible for high-level planning, coordination, and decision-making',
  'Project Manager': 'Responsible for product definition, requirements management, and project tracking',
  'Backend': 'Responsible for backend services, APIs, databases, and architecture',
  'Frontend': 'Responsible for frontend UI, components, and user experience',
  'QA': 'Responsible for quality assurance, testing, and defect management',
  'Custom': 'Unclassified or custom-role agents',
};

export function inferAgentGroup(systemPrompt: string): TeamGroup {
  const sp = systemPrompt.toLowerCase();
  if (sp.includes('leader') || sp.includes('orchestrator')) return 'Orchestrator';
  if (sp.includes('pm') || sp.includes('product') || sp.includes('project manager')) return 'Project Manager';
  if (sp.includes('rd') || sp.includes('backend') || sp.includes('architecture')) return 'Backend';
  if (sp.includes('fe') || sp.includes('frontend') || sp.includes('ui')) return 'Frontend';
  if (sp.includes('qa') || sp.includes('testing') || sp.includes('quality')) return 'QA';
  return 'Custom';
}
