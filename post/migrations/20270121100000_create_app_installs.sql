-- +goose Up
-- "Приложения": who added which HTML widget to keep it at hand ("Добавить
-- к себе"). A deleted widget takes them with it; user_id refers to the
-- user service's database, so there is no foreign key.
CREATE TABLE app_installs (
    app_id     uuid        NOT NULL REFERENCES documents (id) ON DELETE CASCADE,
    user_id    uuid        NOT NULL,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (app_id, user_id)
);

-- "Мои приложения" and the deletion of a page.
CREATE INDEX app_installs_user_idx ON app_installs (user_id, app_id);
-- "Мои приложения" of the owner of widgets.
CREATE INDEX documents_html_idx ON documents (owner_id, id) WHERE ext = 'html';

-- +goose Down
DROP INDEX documents_html_idx;
DROP TABLE app_installs;
