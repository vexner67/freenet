-- +goose Up
-- How the ads do: each reader's views of an ad in the feed and clicks on
-- its link, one row per ad and reader, so that the stats know both how
-- often and by how many people. They go with the ad and with the reader.
CREATE TABLE ad_stats (
    ad_id uuid NOT NULL REFERENCES ads (id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    views bigint NOT NULL DEFAULT 0 CHECK (views >= 0),
    clicks bigint NOT NULL DEFAULT 0 CHECK (clicks >= 0),
    PRIMARY KEY (ad_id, user_id)
);

CREATE INDEX ad_stats_user_id_idx ON ad_stats (user_id);

-- +goose Down
DROP TABLE ad_stats;
