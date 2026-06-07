// ============================================================================
// inferAgentGroup — keyword-based heuristic to bucket an agent into a role.
// Reused by teams-graph-view (structure layout) and teams-left-column (if
// we ever want to show role in the row). For now, only graph-view uses it.
// ============================================================================

export type TeamGroup =
  | '统筹'
  | '产品/项目管理'
  | '后端开发'
  | '前端开发'
  | '测试/QA'
  | '自定义角色';

export const GROUP_ORDER: TeamGroup[] = [
  '统筹',
  '产品/项目管理',
  '后端开发',
  '前端开发',
  '测试/QA',
  '自定义角色',
];

export const GROUP_DESCRIPTIONS: Record<TeamGroup, string> = {
  '统筹': '负责全局规划、协调和决策',
  '产品/项目管理': '负责产品定义、需求管理和项目推进',
  '后端开发': '负责后端服务、API、数据库和架构',
  '前端开发': '负责前端 UI、组件和用户体验',
  '测试/QA': '负责质量保障、测试和缺陷管理',
  '自定义角色': '未归类或自定义角色的 Agent',
};

export function inferAgentGroup(systemPrompt: string): TeamGroup {
  const sp = systemPrompt.toLowerCase();
  if (sp.includes('leader') || sp.includes('统筹')) return '统筹';
  if (sp.includes('pm') || sp.includes('产品') || sp.includes('项目管理')) return '产品/项目管理';
  if (sp.includes('rd') || sp.includes('后端') || sp.includes('架构')) return '后端开发';
  if (sp.includes('fe') || sp.includes('前端') || sp.includes('ui')) return '前端开发';
  if (sp.includes('qa') || sp.includes('测试') || sp.includes('质量')) return '测试/QA';
  return '自定义角色';
}
