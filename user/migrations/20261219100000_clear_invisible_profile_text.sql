-- +goose Up
-- Texts of the page may not have characters that show nothing anymore
-- (user.ErrInvisibleText): they are taken out of what was written before,
-- and a field left with nothing to show is cleared — an additional field
-- is dropped, and a blank name of a filled-in page becomes its nickname.

-- +goose StatementBegin
-- The characters of user.invisible: format and control characters and the
-- fillers; then what shows nothing is trimmed off the ends, as user.blank.
CREATE FUNCTION pg_temp.visible(t text) RETURNS text
LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE WHEN v ~ '^[︀-️]*$' THEN '' ELSE v END
    FROM (
        SELECT regexp_replace(
            regexp_replace(t, '[\u0001-\u0008\u000b\u000c\u000e-\u001f\u007f-\u009f­͏؀-؅۝܏࢐࢑࣢ᅟᅠ឴឵᠎​‪-‮⁠-⁤⁦-⁯⠀ㅤ﻿￹-￻ﾠ\U000110bd\U000110cd\U00013430-\U0001343f\U0001bca0-\U0001bca3\U0001d173-\U0001d17a\U000e0001]', '', 'g'),
            '^[\u0009-\u000d \u0085   - ‌-‏    　]+|[\u0009-\u000d \u0085   - ‌-‏    　]+$', '', 'g') AS v
    ) AS cleaned
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION pg_temp.visible_fields(fields jsonb) RETURNS jsonb
LANGUAGE sql IMMUTABLE AS $$
    SELECT COALESCE(jsonb_agg(jsonb_build_object('name', v.name, 'text', v.text, 'place', e.field->>'place') ORDER BY e.at), '[]')
    FROM jsonb_array_elements(fields) WITH ORDINALITY AS e(field, at),
        LATERAL (SELECT pg_temp.visible(e.field->>'name') AS name, pg_temp.visible(e.field->>'text') AS text) AS v
    WHERE v.name <> '' AND v.text <> ''
$$;
-- +goose StatementEnd

UPDATE users SET
    name = CASE WHEN pg_temp.visible(name) = '' AND nickname <> '' THEN nickname ELSE pg_temp.visible(name) END,
    surname = pg_temp.visible(surname),
    status = pg_temp.visible(status),
    hometown = pg_temp.visible(hometown),
    contact_email = pg_temp.visible(contact_email),
    website = pg_temp.visible(website),
    city = pg_temp.visible(city),
    address = pg_temp.visible(address),
    interests = pg_temp.visible(interests),
    fav_music = pg_temp.visible(fav_music),
    fav_films = pg_temp.visible(fav_films),
    fav_shows = pg_temp.visible(fav_shows),
    fav_books = pg_temp.visible(fav_books),
    fav_quotes = pg_temp.visible(fav_quotes),
    fav_games = pg_temp.visible(fav_games),
    about = pg_temp.visible(about),
    extra_fields = pg_temp.visible_fields(extra_fields)
WHERE (name, surname, status, hometown, contact_email, website, city, address, interests, fav_music, fav_films, fav_shows, fav_books, fav_quotes, fav_games, about, extra_fields)
    IS DISTINCT FROM (pg_temp.visible(name), pg_temp.visible(surname), pg_temp.visible(status), pg_temp.visible(hometown), pg_temp.visible(contact_email), pg_temp.visible(website), pg_temp.visible(city), pg_temp.visible(address), pg_temp.visible(interests), pg_temp.visible(fav_music), pg_temp.visible(fav_films), pg_temp.visible(fav_shows), pg_temp.visible(fav_books), pg_temp.visible(fav_quotes), pg_temp.visible(fav_games), pg_temp.visible(about), pg_temp.visible_fields(extra_fields));

DROP FUNCTION pg_temp.visible_fields(jsonb);
DROP FUNCTION pg_temp.visible(text);

-- +goose Down
-- Nothing to take back: the characters taken out are gone.
