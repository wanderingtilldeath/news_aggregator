-- +goose Up
CREATE TABLE crawls (
    id            BIGSERIAL PRIMARY KEY,
    seed          TEXT        NOT NULL,
    max_depth     INTEGER     NOT NULL,
    allowed_hosts TEXT[]      NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The frontier is the work queue and the visited set: one row per canonical
-- URL per crawl. url_hash is sha256(canonical url) in hex.
CREATE TABLE frontier (
    crawl_id      BIGINT      NOT NULL REFERENCES crawls (id) ON DELETE CASCADE,
    url_hash      TEXT        NOT NULL,
    url           TEXT        NOT NULL,
    host          TEXT        NOT NULL,
    depth         INTEGER     NOT NULL,
    status        TEXT        NOT NULL DEFAULT 'pending',
    attempts      INTEGER     NOT NULL DEFAULT 0,
    next_fetch_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    lease_until   TIMESTAMPTZ,
    last_error    TEXT,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (crawl_id, url_hash),
    CONSTRAINT frontier_status_chk CHECK (
        status IN ('pending', 'leased', 'done', 'failed', 'blocked', 'skipped', 'dead')
    )
);

-- Only live rows are indexed, so the claim query stays fast as finished rows
-- pile up into the millions. Ordering by depth first keeps the crawl
-- breadth-first, which is what makes the depth limit mean "within N hops".
CREATE INDEX frontier_claim_idx ON frontier (depth, next_fetch_at) WHERE status IN ('pending', 'leased');
CREATE INDEX frontier_host_idx ON frontier (host);

CREATE TABLE pages (
    url_hash      TEXT PRIMARY KEY,
    crawl_id      BIGINT      NOT NULL,
    url           TEXT        NOT NULL,
    status_code   INTEGER     NOT NULL,
    content_type  TEXT        NOT NULL DEFAULT '',
    title         TEXT        NOT NULL DEFAULT '',
    description   TEXT        NOT NULL DEFAULT '',
    h1            TEXT        NOT NULL DEFAULT '',
    snippet       TEXT        NOT NULL DEFAULT '',
    content_hash  TEXT        NOT NULL,
    etag          TEXT        NOT NULL DEFAULT '',
    last_modified TEXT        NOT NULL DEFAULT '',
    size_bytes    INTEGER     NOT NULL,
    latency_ms    INTEGER     NOT NULL,
    fetched_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    search        tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('english', coalesce(title, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(h1, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(description, '')), 'B') ||
        setweight(to_tsvector('english', coalesce(snippet, '')), 'C')
    ) STORED
);

CREATE INDEX pages_search_idx ON pages USING GIN (search);

-- +goose Down
DROP TABLE pages;
DROP TABLE frontier;
DROP TABLE crawls;
