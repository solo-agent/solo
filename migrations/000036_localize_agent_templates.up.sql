ALTER TABLE agent_templates
    ADD COLUMN translations JSONB NOT NULL DEFAULT '{}'::jsonb;

WITH template_i18n AS (
  SELECT *
    FROM jsonb_to_recordset($templates$
[
  {"id":"agency-dev-tech-design-review","name":"Technical Design Review","description":"An architect proposes a design, backend and security specialists review it in parallel, and a code reviewer consolidates the final decision."},
  {"id":"agency-dev-pr-review","name":"PR Code Review","description":"Parallel code-quality, security, and performance reviews produce one evidence-backed merge recommendation."},
  {"id":"agency-dev-tech-debt-audit","name":"Technical Debt Audit","description":"Architecture, code quality, and test coverage are assessed in parallel, then ranked into an actionable debt backlog."},
  {"id":"agency-dev-api-doc-gen","name":"API Documentation","description":"A technical writer drafts the API reference and an API tester verifies every contract and example."},
  {"id":"agency-dev-readme-i18n","name":"README Internationalization","description":"A content creator and technical writer adapt a README for an international developer audience."},
  {"id":"agency-dev-security-audit","name":"Security Audit","description":"Application security and threat-detection specialists assess vulnerabilities, coverage gaps, and remediation priorities."},
  {"id":"agency-dev-release-checklist","name":"Release Checklist","description":"Reliability, performance, security, and delivery owners produce a release-ready go or no-go checklist."},
  {"id":"agency-marketing-competitor-analysis","name":"Competitive Analysis Report","description":"Market, analytics, and search specialists investigate competitors before an executive summary turns the evidence into decisions."},
  {"id":"agency-marketing-xiaohongshu-content","name":"Xiaohongshu Campaign Post","description":"Strategy, copy, visuals, and platform operations combine into a publish-ready Xiaohongshu campaign post."},
  {"id":"agency-marketing-seo-content-matrix","name":"SEO Content Matrix","description":"Search, social, and editorial specialists build a coordinated content matrix for sustainable organic growth."},
  {"id":"agency-data-data-pipeline-review","name":"Data Pipeline Review","description":"A data engineer and database optimizer review the pipeline while an analytics specialist validates output quality."},
  {"id":"agency-data-dashboard-design","name":"Data Dashboard Design","description":"Analytics defines the metrics, UX structures the experience, and UI design turns it into a usable dashboard."},
  {"id":"agency-design-requirement-to-plan","name":"Requirements to Delivery Plan","description":"Product requirements become an architecture proposal and a sequenced, executable delivery plan."},
  {"id":"agency-design-ux-review","name":"UX Review","description":"User research and accessibility findings are consolidated into a prioritized experience improvement plan."},
  {"id":"agency-ops-incident-postmortem","name":"Incident Postmortem","description":"Incident evidence, reliability analysis, and delivery ownership produce a blameless postmortem with tracked actions."},
  {"id":"agency-ops-sre-health-check","name":"SRE Health Check","description":"Reliability, performance, and infrastructure specialists assess production health and remediation priorities."},
  {"id":"agency-ops-weekly-report","name":"Weekly or Monthly Report","description":"Raw updates and meeting notes become a concise, decision-ready operating report."},
  {"id":"agency-strategy-business-plan","name":"Business Plan","description":"Market research leads to financial forecasting and product planning, then a complete executive business plan."},
  {"id":"agency-legal-contract-review","name":"Contract Review","description":"Commercial contract risks and compliance obligations are reviewed together and turned into negotiation recommendations."},
  {"id":"agency-hr-interview-questions","name":"Interview Question Design","description":"Recruiting, psychology, and technical specialists design a fair, role-specific interview and evaluation plan."},
  {"id":"agency-product-review","name":"Product Requirements Review","description":"Product, architecture, and user-research perspectives produce a clear requirements decision and next steps."},
  {"id":"agency-content-pipeline","name":"Content Creation Pipeline","description":"A complete research, writing, editorial, and growth workflow takes content from topic to publication."},
  {"id":"agency-story-creation","name":"Short Story Creation","description":"Narrative structure, character psychology, story design, and prose craft combine into a finished short story."},
  {"id":"agency-ai-opinion-article","name":"AI Opinion Article","description":"Trend research and narrative framing support a distinctive, well-evidenced long-form AI article."},
  {"id":"agency-department-collab-code-review","name":"Cross-functional Code Review","description":"Architecture, security, and performance specialists review in parallel before one reviewer delivers the final verdict."},
  {"id":"agency-department-collab-hiring-pipeline","name":"Hiring Evaluation Pipeline","description":"Recruiting, technical leadership, and product leadership evaluate candidates against one shared evidence standard."},
  {"id":"agency-department-collab-content-publish","name":"Content Publishing Workflow","description":"Editorial creation, brand review, and legal compliance produce a publish-ready release package."},
  {"id":"agency-department-collab-incident-response","name":"Incident Response Workflow","description":"Reliability, backend, frontend, and operations specialists coordinate containment, diagnosis, recovery, and follow-up."},
  {"id":"agency-department-collab-marketing-campaign","name":"Marketing Campaign Planning","description":"Research, content, social strategy, and customer feedback combine into a measurable campaign plan."},
  {"id":"agency-department-collab-ceo-org-delegation","name":"CEO Organization Collaboration","description":"Executive planning aligns product, technology, marketing, hiring, and delivery into one operating plan."},
  {"id":"agency-solo-company-all-hands","name":"AI Solo Company: All Hands","description":"Eight AI department leads align strategy, research, product, technology, brand, growth, content, and finance."},
  {"id":"agency-ai-startup-launch","name":"AI Solo Company: SaaS Launch Decision","description":"A CEO prompt launches five parallel department reviews and returns a complete SaaS launch decision."}
]
$templates$::jsonb) AS item(id text, name text, description text)
),
role_i18n AS (
  SELECT *
    FROM jsonb_to_recordset($roles$
[
  {"ref":"software-architect","name":"Software Architect","description":"Designs scalable, maintainable systems through explicit architecture decisions and trade-offs."},
  {"ref":"backend-architect","name":"Backend Architect","description":"Designs robust APIs, data models, services, and cloud infrastructure for secure, high-performance systems."},
  {"ref":"security-engineer","name":"Security Engineer","description":"Specializes in threat modeling, vulnerability assessment, secure design, and incident response."},
  {"ref":"code-reviewer","name":"Code Reviewer","description":"Provides actionable review focused on correctness, maintainability, security, and performance."},
  {"ref":"testing-performance-benchmarker","name":"Performance Benchmarker","description":"Measures system performance, identifies bottlenecks, and validates improvements with reproducible evidence."},
  {"ref":"testing-test-results-analyzer","name":"Test Results Analyzer","description":"Turns test results and quality metrics into prioritized, actionable engineering insights."},
  {"ref":"sprint-prioritizer","name":"Sprint Prioritizer","description":"Ranks work using business value, risk, effort, and dependency evidence."},
  {"ref":"technical-writer","name":"Technical Writer","description":"Creates clear developer documentation, API references, README files, and tutorials."},
  {"ref":"testing-api-tester","name":"API Tester","description":"Validates API contracts, examples, errors, integrations, and performance."},
  {"ref":"content-creator","name":"Content Creator","description":"Develops editorial strategy, persuasive copy, and multi-platform content."},
  {"ref":"threat-detection-engineer","name":"Threat Detection Engineer","description":"Builds and reviews SIEM rules, threat coverage, alert quality, and detection-as-code workflows."},
  {"ref":"sre","name":"Site Reliability Engineer","description":"Owns SLOs, observability, incident readiness, error budgets, and toil reduction."},
  {"ref":"project-manager-senior","name":"Senior Project Manager","description":"Turns plans into realistic, owned, sequenced work with explicit risks and acceptance criteria."},
  {"ref":"trend-researcher","name":"Trend Researcher","description":"Finds emerging trends, competitive signals, and market opportunities that inform strategy."},
  {"ref":"analytics-reporter","name":"Analytics Reporter","description":"Transforms raw data into trustworthy metrics, dashboards, and decision-ready insights."},
  {"ref":"seo-specialist","name":"SEO Specialist","description":"Improves organic growth through technical SEO, search intent, content structure, and authority."},
  {"ref":"executive-summary-generator","name":"Executive Summary Generator","description":"Turns complex evidence into concise, structured recommendations for decision makers."},
  {"ref":"xiaohongshu-specialist","name":"Xiaohongshu Specialist","description":"Creates platform-native Xiaohongshu strategy, lifestyle content, and community engagement."},
  {"ref":"visual-storyteller","name":"Visual Storyteller","description":"Transforms information into coherent visual narratives and emotionally engaging creative direction."},
  {"ref":"xiaohongshu-operator","name":"Xiaohongshu Operator","description":"Operates publishing, account growth, community engagement, and platform distribution on Xiaohongshu."},
  {"ref":"social-media-strategist","name":"Social Media Strategist","description":"Plans cross-platform campaigns, community growth, real-time engagement, and thought leadership."},
  {"ref":"data-engineer","name":"Data Engineer","description":"Builds reliable pipelines, lakehouse systems, streaming workflows, and analytics-ready data infrastructure."},
  {"ref":"database-optimizer","name":"Database Optimizer","description":"Improves schema design, queries, indexes, and database performance."},
  {"ref":"ux-researcher","name":"UX Researcher","description":"Uses behavioral evidence and usability research to identify user needs and experience problems."},
  {"ref":"ui-designer","name":"UI Designer","description":"Creates accessible visual systems, component libraries, and polished interfaces."},
  {"ref":"manager","name":"Product Manager","description":"Aligns user needs, business outcomes, and technical constraints into a focused product plan."},
  {"ref":"testing-accessibility-auditor","name":"Accessibility Auditor","description":"Audits interfaces against WCAG and assistive-technology expectations to find concrete barriers."},
  {"ref":"ux-architect","name":"UX Architect","description":"Turns research findings into coherent information architecture and implementable experience systems."},
  {"ref":"incident-response-commander","name":"Incident Response Commander","description":"Coordinates structured production incident response, communication, recovery, and postmortems."},
  {"ref":"infrastructure-maintainer","name":"Infrastructure Maintainer","description":"Maintains reliable, secure, performant, and cost-effective infrastructure."},
  {"ref":"specialized-meeting-assistant","name":"Meeting Assistant","description":"Turns raw updates into structured notes with decisions, owners, and next actions."},
  {"ref":"financial-forecaster","name":"Financial Forecaster","description":"Builds forecasts, scenarios, unit economics, funding plans, and break-even analyses."},
  {"ref":"legal-contract-reviewer","name":"Contract Reviewer","description":"Reviews contracts for commercial, legal, and operational risk and proposes negotiation changes."},
  {"ref":"legal-compliance-checker","name":"Legal Compliance Checker","description":"Checks operations, data, and content against relevant laws, regulations, and industry standards."},
  {"ref":"recruiter","name":"Recruiter","description":"Designs fair hiring processes and evaluates candidates against explicit role evidence."},
  {"ref":"academic-psychologist","name":"Psychologist","description":"Applies evidence about behavior, motivation, personality, and cognition."},
  {"ref":"growth-hacker","name":"Growth Hacker","description":"Uses rapid experiments, funnel optimization, and scalable acquisition loops to drive growth."},
  {"ref":"academic-narratologist","name":"Narratologist","description":"Applies narrative theory, story structure, character arcs, and literary analysis."},
  {"ref":"narrative-designer","name":"Narrative Designer","description":"Designs story systems, dialogue, lore, conflict, and environmental storytelling."},
  {"ref":"brand-guardian","name":"Brand Guardian","description":"Protects brand positioning, voice, identity, and consistency across every touchpoint."},
  {"ref":"frontend-developer","name":"Frontend Developer","description":"Builds accessible, performant interfaces with modern frontend frameworks and browser standards."},
  {"ref":"devops-automator","name":"DevOps Automator","description":"Automates infrastructure, CI/CD, deployment, observability, and cloud operations."},
  {"ref":"feedback-synthesizer","name":"Feedback Synthesizer","description":"Combines qualitative feedback into evidence-backed product themes and priorities."},
  {"ref":"nexus-strategy","name":"NEXUS Strategy","description":"Aligns specialist work, resolves cross-functional trade-offs, and turns evidence into an executive decision."}
]
$roles$::jsonb) AS item(ref text, name text, description text)
),
member_i18n AS (
  SELECT template.id,
         jsonb_object_agg(
           member->>'ref',
           jsonb_build_object(
             'role', COALESCE(role.name, member->>'role'),
             'name', COALESCE(role.name, member->>'name'),
             'description', COALESCE(role.description, member->>'description')
           )
         ) AS members
    FROM agent_templates template
    JOIN template_i18n translation ON translation.id = template.id
   CROSS JOIN LATERAL jsonb_array_elements(template.members) member
    LEFT JOIN role_i18n role ON role.ref = member->>'ref'
   GROUP BY template.id
)
UPDATE agent_templates template
   SET translations = jsonb_build_object(
     'en',
     jsonb_build_object(
       'name', translation.name,
       'description', translation.description,
       'members', members.members
     )
   )
  FROM template_i18n translation
  JOIN member_i18n members ON members.id = translation.id
 WHERE template.id = translation.id;
