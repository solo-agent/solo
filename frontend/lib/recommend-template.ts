import { t } from '@/lib/i18n';
import type { Template } from '@/lib/templates-api';

const STARTER_KEYWORDS: Record<string, string[]> = {
  'starter-web-page': ['网页', '网站', '页面', '前端', 'web', 'website', 'page', '喝水', '记录'],
  'starter-data-analysis': ['数据', '表格', '实验', '分析', '统计', 'data', 'spreadsheet', 'analysis', 'csv'],
  'starter-study-organizer': ['学习', '课程', '资料', '笔记', '复习', '文档', 'study', 'course', 'notes', 'document'],
};

export interface TemplateRecommendation {
  template: Template;
  reason: string;
}

export function recommendTemplate(goal: string, templates: Template[]): TemplateRecommendation | null {
  if (templates.length === 0) return null;
  const normalizedGoal = goal.trim().toLocaleLowerCase();
  const words = normalizedGoal
    .split(/[\s,，。.!！?？、:：;；/\\_-]+/u)
    .map((word) => word.trim())
    .filter((word) => word.length >= 2);

  const ranked = templates.map((template, index) => {
    const haystack = [template.id, template.name, template.description, template.category, ...(template.roles ?? [])]
      .join(' ')
      .toLocaleLowerCase();
    const starterKeywords = STARTER_KEYWORDS[template.id] ?? [];
    const keywordScore = starterKeywords.reduce(
      (score, keyword) => score + (normalizedGoal.includes(keyword) ? 8 : 0),
      0,
    );
    const textScore = words.reduce((score, word) => score + (haystack.includes(word) ? 3 : 0), 0);
    const starterBonus = template.id.startsWith('starter-') ? 12 : 0;
    const simplicityBonus = Math.max(0, 6 - template.member_count);
    return { template, score: keywordScore + textScore + starterBonus + simplicityBonus, index };
  });
  ranked.sort((a, b) => b.score - a.score || a.template.member_count - b.template.member_count || a.index - b.index);
  const template = ranked[0].template;
  const reason = t('templatesSmartReason', { name: template.name, n: template.member_count });
  return { template, reason };
}
