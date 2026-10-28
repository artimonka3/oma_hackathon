-- Черновик схемы этапа A (основа) технической спецификации.
-- Каркас работает на in-memory репозиториях; при подключении PostgreSQL
-- эта миграция применяется к DATABASE_URL, а репозитории доменов
-- (internal/project, internal/prorab, internal/soberi, internal/mockoma)
-- заменяются SQL-реализациями с теми же интерфейсами.
-- Денежные суммы — целые минимальные единицы (копейки, BYN).
-- Размеры — миллиметры. Время — UTC.

CREATE TABLE users (
    id          TEXT PRIMARY KEY,
    role        TEXT NOT NULL,
    trade_account TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE projects (
    id             TEXT PRIMARY KEY,
    owner_id       TEXT NOT NULL REFERENCES users(id),
    goal           TEXT NOT NULL,
    budget_minor   BIGINT NOT NULL DEFAULT 0,
    currency       TEXT NOT NULL DEFAULT 'BYN',
    execution_mode TEXT NOT NULL DEFAULT 'self',
    version        BIGINT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE project_participants (
    project_id TEXT NOT NULL REFERENCES projects(id),
    user_id    TEXT NOT NULL REFERENCES users(id),
    role       TEXT NOT NULL,
    PRIMARY KEY (project_id, user_id)
);

CREATE TABLE rooms (
    id                  TEXT PRIMARY KEY,
    project_id          TEXT NOT NULL REFERENCES projects(id),
    name                TEXT NOT NULL,
    width_mm            INT NOT NULL,
    length_mm           INT NOT NULL,
    height_mm           INT NOT NULL,
    dimensions_confirmed BOOLEAN NOT NULL DEFAULT false
);

CREATE TABLE media (
    id           TEXT PRIMARY KEY,
    project_id   TEXT NOT NULL REFERENCES projects(id),
    author_id    TEXT NOT NULL REFERENCES users(id),
    kind         TEXT NOT NULL,
    filename     TEXT NOT NULL,
    content_type TEXT,
    size         BIGINT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE inventory_lots (
    id         TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id),
    owner_id   TEXT NOT NULL REFERENCES users(id),
    sku        TEXT NOT NULL,
    title      TEXT,
    amount     TEXT NOT NULL,
    unit       TEXT,
    status     TEXT NOT NULL,           -- suggested | confirmed
    source     TEXT NOT NULL,           -- history | manual
    reserved   TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Журнал доменных событий: сохраняется в одной транзакции с командой
-- (раздел 9 ТЗ). Входящие события магазина дедуплицируются по event_id.
CREATE TABLE domain_events (
    id         BIGSERIAL PRIMARY KEY,
    event_id   TEXT UNIQUE,
    project_id TEXT,
    aggregate  TEXT NOT NULL,
    type       TEXT NOT NULL,
    version    BIGINT NOT NULL,
    author     TEXT,
    data       JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE ai_jobs (
    id         TEXT PRIMARY KEY,
    kind       TEXT NOT NULL,
    project_id TEXT NOT NULL,
    input      JSONB,
    state      TEXT NOT NULL,           -- queued | running | succeeded | failed | cancelled
    result     JSONB,
    error      TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
