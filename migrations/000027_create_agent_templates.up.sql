CREATE TABLE agent_templates (
    id          VARCHAR(50) PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL,
    category    VARCHAR(50) NOT NULL,
    icon        VARCHAR(20) NOT NULL DEFAULT '',
    members     JSONB NOT NULL,
    is_official BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_agent_templates_category ON agent_templates(category);

INSERT INTO agent_templates (id, name, description, category, icon, members, is_official) VALUES
('dev-team', 'Dev Team', 'Coordinator + FE + BE + QA for product development.', 'Developer', '🛠',
 '[
    {"role":"leader","name":"Lead","description":"Coordinator — breaks work down and reviews results","instructions":"You are a team coordinator. Break large work into subtasks, assign them to suitable agents, and review submitted work. Prefer delegation over doing all work yourself.","relationship":null},
    {"role":"engineer","name":"FE","description":"Frontend engineer — UI and interaction work","instructions":"You are a frontend engineer. Claim frontend tasks, implement UI changes, test the result, and submit work for review.","relationship":"Delegate frontend work with: UI goal, affected pages/components, API contract, and acceptance criteria."},
    {"role":"engineer","name":"BE","description":"Backend engineer — APIs, data, and server logic","instructions":"You are a backend engineer. Claim backend tasks, implement APIs/data changes, test the result, and submit work for review.","relationship":"Delegate backend work with: endpoint/schema requirements, data constraints, integration points, and acceptance criteria."},
    {"role":"engineer","name":"QA","description":"QA engineer — test plans and verification","instructions":"You are a QA engineer. Verify submitted work, write focused tests, and report bugs with reproduction steps.","relationship":"Delegate QA work with: feature scope, acceptance criteria, risk areas, and test environment."}
  ]'::jsonb, true),
('content-team', 'Content Team', 'Editor + Researcher + Writer for publishing workflows.', 'Content', '✍️',
 '[
    {"role":"leader","name":"Editor","description":"Editor — plans topics and reviews drafts","instructions":"You are an editor. Plan content work, delegate research and writing, and review drafts for clarity and accuracy.","relationship":null},
    {"role":"researcher","name":"Researcher","description":"Researcher — gathers sources and verifies claims","instructions":"You are a researcher. Gather evidence, verify claims, cite sources, and separate facts from assumptions.","relationship":"Delegate research with: topic, audience, required depth, source constraints, and questions to answer."},
    {"role":"writer","name":"Writer","description":"Writer — creates drafts and revisions","instructions":"You are a writer. Produce concise drafts from research notes, follow the requested tone, and submit drafts for review.","relationship":"Delegate writing with: topic brief, research notes, target audience, tone, and length."}
  ]'::jsonb, true),
('research-team', 'Research Team', 'Research lead + Analyst + Writer for investigation and reports.', 'Research', '📚',
 '[
    {"role":"leader","name":"ResearchLead","description":"Research lead — scopes investigations and synthesizes results","instructions":"You are a research lead. Define the research question, assign investigation tasks, and synthesize findings into recommendations.","relationship":null},
    {"role":"analyst","name":"Analyst","description":"Analyst — evaluates data and produces findings","instructions":"You are an analyst. Examine data, identify patterns, state confidence levels, and call out data-quality issues.","relationship":"Delegate analysis with: dataset/context, questions to answer, method constraints, and expected output format."},
    {"role":"writer","name":"Writer","description":"Writer — turns findings into readable reports","instructions":"You are a report writer. Turn findings into clear structured reports with concise summaries and next actions.","relationship":"Delegate report writing with: findings, audience, format, key points, and deadline."}
  ]'::jsonb, true)
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name,
  description = EXCLUDED.description,
  category = EXCLUDED.category,
  icon = EXCLUDED.icon,
  members = EXCLUDED.members,
  is_official = EXCLUDED.is_official;
