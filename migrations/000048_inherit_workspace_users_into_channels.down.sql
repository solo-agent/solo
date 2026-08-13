-- The inherited User rows are valid Channel memberships and cannot be
-- distinguished from historical explicit User memberships. Keep them on
-- rollback rather than destructively removing legitimate access.
SELECT 1;
