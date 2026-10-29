CREATE TABLE IF NOT EXISTS oma_schema_migrations(version INTEGER PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS oma_runtime(singleton BOOLEAN PRIMARY KEY DEFAULT true CHECK(singleton),revision BIGINT NOT NULL DEFAULT 0,updated_at TIMESTAMPTZ NOT NULL DEFAULT now());
INSERT INTO oma_runtime(singleton) VALUES(true) ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS oma_components(name TEXT PRIMARY KEY,payload JSONB NOT NULL CHECK(jsonb_typeof(payload)='object'),updated_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE OR REPLACE VIEW oma_projects AS SELECT e.key AS id,e.value->>'owner_id' AS owner_id,e.value->>'goal' AS goal,(e.value->>'version')::bigint AS version,e.value AS body FROM oma_components c CROSS JOIN LATERAL jsonb_each(c.payload->'Projects') e WHERE c.name='projects';
CREATE OR REPLACE VIEW oma_orders AS SELECT e.key AS id,e.value->>'project_id' AS project_id,e.value->>'status' AS status,(e.value->>'total_minor')::bigint AS total_minor,e.value AS body FROM oma_components c CROSS JOIN LATERAL jsonb_each(c.payload->'Orders') e WHERE c.name='commerce';
CREATE OR REPLACE VIEW oma_reviews AS SELECT e.key AS id,e.value->>'project_id' AS project_id,e.value->>'foreman_id' AS foreman_id,e.value->>'status' AS status,e.value AS body FROM oma_components c CROSS JOIN LATERAL jsonb_each(c.payload->'Reviews') e WHERE c.name='prorab';
CREATE OR REPLACE VIEW oma_reports AS SELECT e.key AS id,e.value->>'stage_id' AS stage_id,(e.value->>'version')::bigint AS version,e.value AS body FROM oma_components c CROSS JOIN LATERAL jsonb_each(c.payload->'Reports') e WHERE c.name='prorab';
INSERT INTO oma_schema_migrations(version) VALUES(1) ON CONFLICT DO NOTHING;
