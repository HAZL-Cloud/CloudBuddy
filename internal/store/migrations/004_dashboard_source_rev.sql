-- Each run and each tracked number names the version of its source that
-- produced it (dash.SourceRev: the dashboard's folder, the command, its
-- folder and its format), so a source changed under the same key never
-- shows what the old version printed (Codex review, 2026-11-10). Safe to
-- run twice.
ALTER TABLE dashboard_runs ADD COLUMN IF NOT EXISTS source_rev TEXT NOT NULL DEFAULT '';
ALTER TABLE dashboard_points ADD COLUMN IF NOT EXISTS source_rev TEXT NOT NULL DEFAULT '';

-- Rows stored before this are taken to come from the version each source
-- has now: nothing recorded which version they came from. A source changed
-- under the same key before this keeps that older history: its numbers age
-- out in 30 days, and its last run that worked is shown, dated and marked
-- old, until the source next works. This must compute exactly what
-- dash.SourceRev does (a test checks the two agree).
CREATE TEMP TABLE dash_source_revs ON COMMIT DROP AS
    SELECT d.id AS dashboard_id, s->>'key' AS source_key,
        left(encode(sha256(convert_to(
            octet_length(d.workdir)::text || ':' || d.workdir || '|' ||
            octet_length(s->>'command')::text || ':' || (s->>'command') || '|' ||
            octet_length(coalesce(s->>'cwd', ''))::text || ':' || coalesce(s->>'cwd', '') || '|' ||
            coalesce(s->>'format', ''),
            'UTF8')), 'hex'), 16) AS source_rev
    FROM dashboards d, jsonb_array_elements(d.spec->'sources') s;

UPDATE dashboard_runs r SET source_rev = v.source_rev FROM dash_source_revs v
    WHERE r.source_rev = '' AND r.dashboard_id = v.dashboard_id AND r.source_key = v.source_key;

-- a series is "<source key>|<path>"; keys never hold "|"
UPDATE dashboard_points p SET source_rev = v.source_rev FROM dash_source_revs v
    WHERE p.source_rev = '' AND p.dashboard_id = v.dashboard_id AND split_part(p.series, '|', 1) = v.source_key;
