-- +goose Up
-- The likes and the poll votes of a page its owner deleted and may still
-- restore: taken out of their tables, and so out of the counts and the
-- lists of who liked, and put back when the page is restored. The target
-- is a post, a comment, a photo of an album, a message of a topic or a
-- comment of a photo; place is the option of a poll vote, 0 otherwise.
-- No foreign key: what was liked may go meanwhile, and the like with it.
CREATE TABLE held_reactions (
    user_id    uuid        NOT NULL,
    kind       text        NOT NULL CHECK (kind IN (
        'post_like', 'comment_like', 'album_photo_like', 'topic_message_like', 'photo_comment_like', 'poll_vote'
    )),
    target_id  uuid        NOT NULL,
    place      smallint    NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (user_id, kind, target_id, place)
);

-- +goose Down
-- The pages deleted now lose their likes for good.
DROP TABLE held_reactions;
