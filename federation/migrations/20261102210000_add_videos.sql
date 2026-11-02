-- +goose Up
-- The videos of the notes: shown as their covers.
ALTER TABLE remote_posts
    ADD COLUMN videos jsonb NOT NULL DEFAULT '[]';

-- The first frames of the videos with no cover the author chose, cut
-- once; an empty frame is a video with none.
CREATE TABLE remote_video_covers (
    post_id    uuid        NOT NULL REFERENCES remote_posts (id) ON DELETE CASCADE,
    video      text        NOT NULL,
    frame      bytea       NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (post_id, video)
);

-- +goose Down
DROP TABLE remote_video_covers;

ALTER TABLE remote_posts
    DROP COLUMN videos;
