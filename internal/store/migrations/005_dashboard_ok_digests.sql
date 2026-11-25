-- An OK for a dashboard command is for exact versions of the files it runs
-- (D-067). Every version the user allowed is kept, not only the latest: a
-- git checkout that brings back a version they allowed before needs no new
-- OK (2026-11-24: switching branches and back made a dashboard wait for
-- days). A "no" forgets them all. Safe to run twice.
ALTER TABLE dashboard_approvals ADD COLUMN IF NOT EXISTS ok_digests TEXT[] NOT NULL DEFAULT '{}';
UPDATE dashboard_approvals SET ok_digests = ARRAY[exec_digest] WHERE allowed AND ok_digests = '{}';
