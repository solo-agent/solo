-- Compatibility is retained when rolling back to 77. The feature migrations
-- 70 and 68 remove their own columns/guards without deleting historical rows.
SELECT 1;
