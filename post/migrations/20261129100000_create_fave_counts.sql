-- +goose Up
-- "N закладок" counted the likes of a person with every page of their
-- faves. fave_counts keeps the numbers: how many posts, comments and
-- photos of albums a person likes. Likes come and go in many ways (a like
-- taken back, a deleted post taking its likes along, a deleted page whose
-- likes are set aside), so the tables of likes count themselves: one
-- trigger per statement, whatever the statement was.
LOCK TABLE post_likes, comment_likes, album_photo_likes IN SHARE ROW EXCLUSIVE MODE;

CREATE TABLE fave_counts (
    user_id  uuid    PRIMARY KEY,
    posts    integer NOT NULL DEFAULT 0 CHECK (posts >= 0),
    comments integer NOT NULL DEFAULT 0 CHECK (comments >= 0),
    photos   integer NOT NULL DEFAULT 0 CHECK (photos >= 0)
);

INSERT INTO fave_counts (
    user_id,
    posts,
    comments,
    photos
)
SELECT
    user_id,
    sum(posts),
    sum(comments),
    sum(photos)
FROM (
    SELECT user_id, count(*) AS posts, 0 AS comments, 0 AS photos FROM post_likes GROUP BY user_id
    UNION ALL
    SELECT user_id, 0, count(*), 0 FROM comment_likes GROUP BY user_id
    UNION ALL
    SELECT user_id, 0, 0, count(*) FROM album_photo_likes GROUP BY user_id
) AS likes
GROUP BY user_id;

-- The counters of several people change in the order of their IDs, so two
-- statements over the same people do not wait for each other in a circle.
-- +goose StatementBegin
CREATE FUNCTION count_faves() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    EXECUTE format(
        'INSERT INTO fave_counts AS c (user_id, %1$I)
         SELECT user_id, count(*) FROM changed GROUP BY user_id ORDER BY user_id
         ON CONFLICT (user_id) DO UPDATE SET %1$I = c.%1$I + EXCLUDED.%1$I',
        TG_ARGV[0]);

    RETURN NULL;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION uncount_faves() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM 1
    FROM fave_counts
    WHERE user_id IN (SELECT user_id FROM changed)
    ORDER BY user_id
    FOR UPDATE;

    EXECUTE format(
        'UPDATE fave_counts AS c
         SET %1$I = c.%1$I - gone.n
         FROM (SELECT user_id, count(*) AS n FROM changed GROUP BY user_id) AS gone
         WHERE c.user_id = gone.user_id',
        TG_ARGV[0]);

    RETURN NULL;
END
$$;
-- +goose StatementEnd

CREATE TRIGGER post_likes_counted AFTER INSERT ON post_likes
    REFERENCING NEW TABLE AS changed FOR EACH STATEMENT EXECUTE FUNCTION count_faves('posts');
CREATE TRIGGER post_likes_uncounted AFTER DELETE ON post_likes
    REFERENCING OLD TABLE AS changed FOR EACH STATEMENT EXECUTE FUNCTION uncount_faves('posts');
CREATE TRIGGER comment_likes_counted AFTER INSERT ON comment_likes
    REFERENCING NEW TABLE AS changed FOR EACH STATEMENT EXECUTE FUNCTION count_faves('comments');
CREATE TRIGGER comment_likes_uncounted AFTER DELETE ON comment_likes
    REFERENCING OLD TABLE AS changed FOR EACH STATEMENT EXECUTE FUNCTION uncount_faves('comments');
CREATE TRIGGER album_photo_likes_counted AFTER INSERT ON album_photo_likes
    REFERENCING NEW TABLE AS changed FOR EACH STATEMENT EXECUTE FUNCTION count_faves('photos');
CREATE TRIGGER album_photo_likes_uncounted AFTER DELETE ON album_photo_likes
    REFERENCING OLD TABLE AS changed FOR EACH STATEMENT EXECUTE FUNCTION uncount_faves('photos');

-- +goose Down
DROP TRIGGER album_photo_likes_uncounted ON album_photo_likes;
DROP TRIGGER album_photo_likes_counted ON album_photo_likes;
DROP TRIGGER comment_likes_uncounted ON comment_likes;
DROP TRIGGER comment_likes_counted ON comment_likes;
DROP TRIGGER post_likes_uncounted ON post_likes;
DROP TRIGGER post_likes_counted ON post_likes;
DROP FUNCTION uncount_faves();
DROP FUNCTION count_faves();
DROP TABLE fave_counts;
