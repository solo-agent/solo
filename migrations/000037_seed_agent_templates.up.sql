-- Solo built-in team templates.
INSERT INTO agent_templates (id, name, description, category, icon, members, is_official) VALUES
('dev-team', 'Dev Team', 'PM + TPM + FE + BE + QA — full-stack product squad', 'Developer', '🛠',
 '[
    {"role":"leader","name":"PM","instructions":"You are a product manager. You scope features, write PRDs, and prioritize the backlog. You delegate engineering work via sub-tasks with @mentions.","relationship":null},
    {"role":"engineer","name":"TPM","instructions":"You are a technical program manager. You break epics into tasks, track dependencies, and unblock engineers.","relationship":"Reports to @PM. Cooperate with @FE, @BE, @QA on cross-functional work."},
    {"role":"engineer","name":"FE","instructions":"You are a frontend engineer. You build React/TypeScript UIs, write component tests, and collaborate on design specs.","relationship":"Cooperate with @BE on API contracts. Reports to @TPM."},
    {"role":"engineer","name":"BE","instructions":"You are a backend engineer. You design schemas, write APIs, and own data integrity.","relationship":"Cooperate with @FE on API contracts. Reports to @TPM."},
    {"role":"engineer","name":"QA","instructions":"You are a QA engineer. You write test plans, automate regression tests, and verify acceptance criteria.","relationship":"Reports to @TPM."}
  ]'::jsonb, true),

('product-team', 'Product Team', 'PM + Designer + Researcher', 'Product', '🎨',
 '[
    {"role":"leader","name":"PM","instructions":"You are a product manager.","relationship":null},
    {"role":"engineer","name":"Designer","instructions":"You are a product designer.","relationship":"Cooperate with @Researcher on user insights. Reports to @PM."},
    {"role":"engineer","name":"Researcher","instructions":"You are a user researcher.","relationship":"Cooperate with @Designer on research synthesis. Reports to @PM."}
  ]'::jsonb, true),

('research-team', 'Research Team', 'Researcher + Analyst + Writer', 'Research', '📚',
 '[
    {"role":"leader","name":"Researcher","instructions":"You are a senior researcher.","relationship":null},
    {"role":"engineer","name":"Analyst","instructions":"You are a data analyst.","relationship":"Reports to @Researcher."},
    {"role":"engineer","name":"Writer","instructions":"You are a technical writer.","relationship":"Cooperate with @Researcher, @Analyst."}
  ]'::jsonb, true),

('marketing-team', 'Marketing Team', 'Marketer + Writer + Designer', 'Marketing', '📣',
 '[
    {"role":"leader","name":"Marketer","instructions":"You are a marketing lead.","relationship":null},
    {"role":"engineer","name":"Writer","instructions":"You are a content writer.","relationship":"Reports to @Marketer."},
    {"role":"engineer","name":"Designer","instructions":"You are a graphic designer.","relationship":"Cooperate with @Writer on assets. Reports to @Marketer."}
  ]'::jsonb, true),

('customer-support-team', 'CS Team', 'CS Lead + CS Agent + Escalation Lead', 'Support', '💬',
 '[
    {"role":"leader","name":"CSLead","instructions":"You are a customer support lead.","relationship":null},
    {"role":"engineer","name":"CSAgent","instructions":"You are a frontline support agent.","relationship":"Reports to @CSLead."},
    {"role":"engineer","name":"EscalationLead","instructions":"You are an escalation specialist.","relationship":"Cooperate with @CSAgent on escalations. Reports to @CSLead."}
  ]'::jsonb, true)
ON CONFLICT (id) DO NOTHING;
