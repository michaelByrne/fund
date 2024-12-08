DROP TRIGGER IF EXISTS apply_default_trigger ON member;
DROP TABLE IF EXISTS member CASCADE;
DROP TYPE IF EXISTS role;
DROP FUNCTION IF EXISTS apply_default_if_no_role();