-- +goose Up
-- The people, organisations, places and laws a story is about (up to four), named by the first
-- enrichment stage. They feed the entity pages and the knowledge graph.
ALTER TABLE pages ADD COLUMN entities TEXT[] NOT NULL DEFAULT '{}';
CREATE INDEX pages_entities_idx ON pages USING GIN (entities);

-- +goose Down
DROP INDEX pages_entities_idx;
ALTER TABLE pages DROP COLUMN entities;
