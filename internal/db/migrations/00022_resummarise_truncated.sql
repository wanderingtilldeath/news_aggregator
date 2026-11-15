-- +goose Up
-- Gists written before the sentence fix were cut at a comma or at 20 words and end in an ellipsis
-- ("When asked about talks on sending Hasina to a third country..."). Mark the articles of the last
-- three days that have such a bullet, and still have their text, as not yet summarised: the next
-- crawl run writes them again from the whole sentence. Older articles are left alone, so the
-- one-off cost (model calls, when a Gemini key is set) is bounded.
UPDATE pages SET summarized_at = NULL
WHERE is_article AND summarized_at IS NOT NULL AND content <> ''
  AND COALESCE(published_at, first_seen_at) > now() - interval '3 days'
  AND EXISTS (SELECT 1 FROM unnest(summary) AS bullet WHERE bullet LIKE '%…' OR bullet LIKE '%...');

-- +goose Down
SELECT 1;
