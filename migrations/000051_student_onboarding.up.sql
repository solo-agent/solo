ALTER TABLE channels
    ADD COLUMN project_computer_id UUID REFERENCES computers(id) ON DELETE SET NULL,
    ADD COLUMN project_path TEXT;

ALTER TABLE channels
    ADD CONSTRAINT channels_project_binding_complete CHECK (
        project_path IS NULL OR length(btrim(project_path)) > 0
    );

INSERT INTO agent_templates (
    id, name, description, category, icon, members, relationships, translations, is_official
) VALUES
(
    'starter-web-page',
    '做一个简单网页',
    '适合第一次使用：由一个网页助手完成页面、样式和本地预览。',
    '入门',
    '🌱',
    '[{"ref":"builder","role":"网页助手","name":"Builder","description":"把一个小想法做成可以打开的网页","instructions":"你是网页助手。先用容易理解的话确认用户想做的页面，然后在当前项目文件夹中完成最小可用版本，运行真实检查，并告诉用户如何打开结果。不要引入不必要的框架或复杂团队。"}]'::jsonb,
    '[]'::jsonb,
    '{"en":{"name":"Build a simple web page","description":"A beginner-friendly single teammate builds the page, styling, and local preview.","category":"Starter","members":{"builder":{"role":"Web page helper","name":"Builder","description":"Turns a small idea into a page you can open"}}}}'::jsonb,
    true
),
(
    'starter-data-analysis',
    '分析一份数据',
    '适合课程和实验数据：由一个分析助手检查数据并给出清楚结论。',
    '入门',
    '📊',
    '[{"ref":"analyst","role":"数据分析助手","name":"Analyst","description":"检查表格或实验数据并解释发现","instructions":"你是数据分析助手。先确认数据文件和用户问题，在当前项目文件夹中保留可复现的分析过程，检查数据质量，用通俗语言说明结论和限制。"}]'::jsonb,
    '[]'::jsonb,
    '{"en":{"name":"Analyze a dataset","description":"A beginner-friendly single teammate checks course or experiment data and explains the findings.","category":"Starter","members":{"analyst":{"role":"Data helper","name":"Analyst","description":"Checks tables or experiment data and explains findings"}}}}'::jsonb,
    true
),
(
    'starter-study-organizer',
    '整理学习资料',
    '适合课程资料和复习：由一个学习助手整理文件、摘要和下一步。',
    '入门',
    '📚',
    '[{"ref":"organizer","role":"学习整理助手","name":"Organizer","description":"整理课程资料、笔记和复习计划","instructions":"你是学习整理助手。读取用户明确提供的资料，在当前项目文件夹中建立容易查找的结构，给出准确摘要、待确认问题和下一步复习建议，不虚构资料内容。"}]'::jsonb,
    '[]'::jsonb,
    '{"en":{"name":"Organize study materials","description":"A beginner-friendly single teammate organizes course files, summaries, and next steps.","category":"Starter","members":{"organizer":{"role":"Study helper","name":"Organizer","description":"Organizes course materials, notes, and review plans"}}}}'::jsonb,
    true
)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    category = EXCLUDED.category,
    icon = EXCLUDED.icon,
    members = EXCLUDED.members,
    relationships = EXCLUDED.relationships,
    translations = EXCLUDED.translations,
    is_official = true;
