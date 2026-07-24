DELETE FROM agent_templates
 WHERE id IN (
    'agency-dev-tech-design-review',
    'agency-dev-pr-review',
    'agency-dev-tech-debt-audit',
    'agency-dev-api-doc-gen',
    'agency-dev-readme-i18n',
    'agency-dev-security-audit',
    'agency-dev-release-checklist',
    'agency-marketing-competitor-analysis',
    'agency-marketing-xiaohongshu-content',
    'agency-marketing-seo-content-matrix',
    'agency-data-data-pipeline-review',
    'agency-data-dashboard-design',
    'agency-design-requirement-to-plan',
    'agency-design-ux-review',
    'agency-ops-incident-postmortem',
    'agency-ops-sre-health-check',
    'agency-ops-weekly-report',
    'agency-strategy-business-plan',
    'agency-legal-contract-review',
    'agency-hr-interview-questions',
    'agency-product-review',
    'agency-content-pipeline',
    'agency-story-creation',
    'agency-ai-opinion-article',
    'agency-department-collab-code-review',
    'agency-department-collab-hiring-pipeline',
    'agency-department-collab-content-publish',
    'agency-department-collab-incident-response',
    'agency-department-collab-marketing-campaign',
    'agency-department-collab-ceo-org-delegation',
    'agency-solo-company-all-hands',
    'agency-ai-startup-launch'
 );

UPDATE agent_templates
   SET is_official = true
 WHERE id IN ('dev-team', 'content-team', 'research-team');
