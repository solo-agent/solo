UPDATE agent_templates
   SET is_official = true
 WHERE id IN ('starter-web-page', 'starter-data-analysis', 'starter-study-organizer');
