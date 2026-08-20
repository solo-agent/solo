UPDATE agent_templates
   SET is_official = false
 WHERE id IN ('starter-web-page', 'starter-data-analysis', 'starter-study-organizer');
