ALTER TABLE channels
    ADD COLUMN source_template_id VARCHAR(50) REFERENCES agent_templates(id) ON DELETE SET NULL;

ALTER TABLE agents
    ADD COLUMN home_channel_id UUID REFERENCES channels(id) ON DELETE RESTRICT,
    ADD COLUMN kind VARCHAR(20) NOT NULL DEFAULT 'agent'
        CHECK (kind IN ('agent', 'lucy'));

ALTER TABLE messages
    ADD COLUMN metadata JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE agent_templates
    ADD COLUMN relationships JSONB NOT NULL DEFAULT '[]'::jsonb;

-- Every owner's existing Welcome channel becomes the pinned Lucy Channel,
-- including workspaces where Lucy has not been configured yet.
UPDATE channels c
   SET type = 'lucy',
       updated_at = now()
 WHERE c.name LIKE 'welcome-%'
   AND c.is_archived = false;

WITH lucy_homes AS (
    SELECT DISTINCT ON (c.created_by)
           c.created_by AS owner_id,
           c.id AS channel_id
      FROM channels c
      LEFT JOIN agents candidate
        ON candidate.owner_id = c.created_by
       AND lower(candidate.name) = 'lucy'
      LEFT JOIN channel_members existing_membership
        ON existing_membership.channel_id = c.id
       AND existing_membership.member_type = 'agent'
       AND existing_membership.member_id = candidate.id
     WHERE c.type = 'lucy'
       AND c.is_archived = false
     ORDER BY c.created_by,
              (existing_membership.member_id IS NOT NULL) DESC,
              c.created_at ASC
),
lucy_agents AS (
    SELECT DISTINCT ON (a.owner_id)
           a.id,
           a.owner_id
      FROM agents a
      JOIN lucy_homes h ON h.owner_id = a.owner_id
     WHERE lower(a.name) = 'lucy'
     ORDER BY a.owner_id, a.is_active DESC, a.created_at ASC
)
UPDATE agents a
   SET kind = 'lucy',
       home_channel_id = h.channel_id,
       system_prompt = 'You are Lucy, the steward in the pinned Lucy Channel for this Solo server. You respond only in your Lucy Channel or a DM. You may inspect and manage other Channels only when the owner explicitly asks; never treat their messages as incoming context. For team creation, always run solo template list --json first. Create only on an explicit owner request, then call solo team form with intent_summary, channel, and template_id. Never reuse Agents or silently modify template members or relationships. If an older workspace note conflicts with this prompt, this prompt is authoritative.',
       updated_at = now()
  FROM lucy_agents selected
  JOIN lucy_homes h ON h.owner_id = selected.owner_id
 WHERE a.id = selected.id;

-- Lucy keeps only her pinned Lucy membership plus any existing hidden DMs.
DELETE FROM channel_members cm
 USING agents a, channels c
 WHERE cm.member_type = 'agent'
   AND cm.member_id = a.id
   AND cm.channel_id = c.id
   AND a.kind = 'lucy'
   AND c.type NOT IN ('lucy', 'dm');

INSERT INTO channel_members (channel_id, member_type, member_id, role)
SELECT a.home_channel_id, 'agent', a.id, 'member'
  FROM agents a
 WHERE a.kind = 'lucy'
   AND a.home_channel_id IS NOT NULL
ON CONFLICT DO NOTHING;

-- Legacy global ordinary Agents intentionally keep their existing state and
-- runtime behavior. New product flows create only Channel-scoped Agents.

DROP INDEX idx_agents_owner_name_active;

CREATE UNIQUE INDEX idx_agents_home_channel_name_active
    ON agents(home_channel_id, lower(name))
    WHERE is_active = true AND kind = 'agent';

CREATE UNIQUE INDEX idx_agents_owner_lucy_active
    ON agents(owner_id)
    WHERE is_active = true AND kind = 'lucy';

CREATE INDEX idx_agents_home_channel_active
    ON agents(home_channel_id, is_active);

CREATE INDEX idx_channels_source_template
    ON channels(source_template_id)
    WHERE source_template_id IS NOT NULL;

CREATE OR REPLACE FUNCTION enforce_agent_channel_membership()
RETURNS trigger AS $$
DECLARE
    agent_kind VARCHAR(20);
    agent_home UUID;
    channel_kind VARCHAR(20);
BEGIN
    IF NEW.member_type <> 'agent' THEN
        RETURN NEW;
    END IF;

    SELECT kind, home_channel_id
      INTO agent_kind, agent_home
      FROM agents
     WHERE id = NEW.member_id;

    SELECT type
      INTO channel_kind
      FROM channels
     WHERE id = NEW.channel_id;

    IF channel_kind = 'dm' THEN
        RETURN NEW;
    END IF;

    IF agent_home IS DISTINCT FROM NEW.channel_id THEN
        RAISE EXCEPTION 'agent % belongs to channel %, not %',
            NEW.member_id, agent_home, NEW.channel_id;
    END IF;

    IF agent_kind = 'lucy' AND channel_kind <> 'lucy' THEN
        RAISE EXCEPTION 'Lucy may only join her Lucy channel or a DM';
    END IF;

    IF agent_kind = 'agent' AND channel_kind <> 'channel' THEN
        RAISE EXCEPTION 'ordinary agents may only join their home channel or a DM';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_enforce_agent_channel_membership
BEFORE INSERT OR UPDATE OF channel_id, member_type, member_id
ON channel_members
FOR EACH ROW
EXECUTE FUNCTION enforce_agent_channel_membership();

CREATE OR REPLACE FUNCTION enforce_relationship_channel_scope()
RETURNS trigger AS $$
DECLARE
    from_home UUID;
    to_home UUID;
BEGIN
    SELECT home_channel_id INTO from_home FROM agents WHERE id = NEW.from_agent_id;
    SELECT home_channel_id INTO to_home FROM agents WHERE id = NEW.to_agent_id;

    IF from_home IS NULL OR to_home IS NULL OR from_home <> to_home THEN
        RAISE EXCEPTION 'agent relationships must stay inside one home channel';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_enforce_relationship_channel_scope
BEFORE INSERT OR UPDATE OF from_agent_id, to_agent_id
ON agent_relationships
FOR EACH ROW
EXECUTE FUNCTION enforce_relationship_channel_scope();

UPDATE agent_templates
   SET members = '[
         {"ref":"lead","role":"leader","name":"Lead","description":"Coordinator — breaks work down and reviews results","instructions":"You are a team coordinator. Break large work into subtasks, assign them to suitable agents, and review submitted work. Prefer delegation over doing all work yourself."},
         {"ref":"fe","role":"engineer","name":"FE","description":"Frontend engineer — UI and interaction work","instructions":"You are a frontend engineer. Claim frontend tasks, implement UI changes, test the result, and submit work for review."},
         {"ref":"be","role":"engineer","name":"BE","description":"Backend engineer — APIs, data, and server logic","instructions":"You are a backend engineer. Claim backend tasks, implement APIs/data changes, test the result, and submit work for review."},
         {"ref":"qa","role":"engineer","name":"QA","description":"QA engineer — test plans and verification","instructions":"You are a QA engineer. Verify submitted work, write focused tests, and report bugs with reproduction steps."}
       ]'::jsonb,
       relationships = '[
         {"from_ref":"lead","to_ref":"fe","type":"assigns_to","weight":1,"instruction":"Delegate frontend work with: UI goal, affected pages/components, API contract, and acceptance criteria."},
         {"from_ref":"lead","to_ref":"be","type":"assigns_to","weight":1,"instruction":"Delegate backend work with: endpoint/schema requirements, data constraints, integration points, and acceptance criteria."},
         {"from_ref":"lead","to_ref":"qa","type":"assigns_to","weight":1,"instruction":"Delegate QA work with: feature scope, acceptance criteria, risk areas, and test environment."}
       ]'::jsonb
 WHERE id = 'dev-team';

UPDATE agent_templates
   SET members = '[
         {"ref":"editor","role":"leader","name":"Editor","description":"Editor — plans topics and reviews drafts","instructions":"You are an editor. Plan content work, delegate research and writing, and review drafts for clarity and accuracy."},
         {"ref":"researcher","role":"researcher","name":"Researcher","description":"Researcher — gathers sources and verifies claims","instructions":"You are a researcher. Gather evidence, verify claims, cite sources, and separate facts from assumptions."},
         {"ref":"writer","role":"writer","name":"Writer","description":"Writer — creates drafts and revisions","instructions":"You are a writer. Produce concise drafts from research notes, follow the requested tone, and submit drafts for review."}
       ]'::jsonb,
       relationships = '[
         {"from_ref":"editor","to_ref":"researcher","type":"assigns_to","weight":1,"instruction":"Delegate research with: topic, audience, required depth, source constraints, and questions to answer."},
         {"from_ref":"editor","to_ref":"writer","type":"assigns_to","weight":1,"instruction":"Delegate writing with: topic brief, research notes, target audience, tone, and length."}
       ]'::jsonb
 WHERE id = 'content-team';

UPDATE agent_templates
   SET members = '[
         {"ref":"lead","role":"leader","name":"ResearchLead","description":"Research lead — scopes investigations and synthesizes results","instructions":"You are a research lead. Define the research question, assign investigation tasks, and synthesize findings into recommendations."},
         {"ref":"analyst","role":"analyst","name":"Analyst","description":"Analyst — evaluates data and produces findings","instructions":"You are an analyst. Examine data, identify patterns, state confidence levels, and call out data-quality issues."},
         {"ref":"writer","role":"writer","name":"Writer","description":"Writer — turns findings into readable reports","instructions":"You are a report writer. Turn findings into clear structured reports with concise summaries and next actions."}
       ]'::jsonb,
       relationships = '[
         {"from_ref":"lead","to_ref":"analyst","type":"assigns_to","weight":1,"instruction":"Delegate analysis with: dataset/context, questions to answer, method constraints, and expected output format."},
         {"from_ref":"lead","to_ref":"writer","type":"assigns_to","weight":1,"instruction":"Delegate report writing with: findings, audience, format, key points, and deadline."}
       ]'::jsonb
 WHERE id = 'research-team';
