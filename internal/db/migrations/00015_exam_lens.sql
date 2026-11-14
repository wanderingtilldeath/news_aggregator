-- +goose Up
-- The "exam lens" the enrichment pass writes for UPSC stories (india, world and upsc categories):
-- how relevant a story is to Prelims, Mains, Essay and Ethics (0 none to 3 high), kept in four
-- small columns so the front page can show them cheaply, and the rest (dimensions, arguments for
-- and against, Prelims facts, a Mains answer skeleton) as JSON for the story page.
ALTER TABLE pages
    ADD COLUMN rel_prelims SMALLINT NOT NULL DEFAULT 0,
    ADD COLUMN rel_mains   SMALLINT NOT NULL DEFAULT 0,
    ADD COLUMN rel_essay   SMALLINT NOT NULL DEFAULT 0,
    ADD COLUMN rel_ethics  SMALLINT NOT NULL DEFAULT 0,
    ADD COLUMN exam        JSONB;

-- +goose Down
ALTER TABLE pages
    DROP COLUMN exam, DROP COLUMN rel_ethics, DROP COLUMN rel_essay, DROP COLUMN rel_mains, DROP COLUMN rel_prelims;
