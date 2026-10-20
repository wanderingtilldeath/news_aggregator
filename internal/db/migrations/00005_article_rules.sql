-- +goose Up
-- article_prefixes: when set, only pages under these paths count as digest
-- articles. split_articles: store each <article id> block of a multi-topic
-- page as its own item.
ALTER TABLE crawls
    ADD COLUMN article_prefixes TEXT[]  NOT NULL DEFAULT '{}',
    ADD COLUMN split_articles   BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE crawls DROP COLUMN split_articles, DROP COLUMN article_prefixes;
