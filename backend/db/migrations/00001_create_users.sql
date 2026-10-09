-- +goose Up
-- 利用者。認証を作るまでは、開発用利用者（DEV_USER_ID）とテストの利用者だけが入る。
-- 利用者の行はここでは入れない（どの利用者を入れるかは seed が決める。プラン 4.4「テストデータ」）。
CREATE TABLE users (
    id         UUID        PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE users;
