-- +goose Up
-- 保有データ（要件 FR-17〜20）。同じ利用者・同じ銘柄の行は複数あってよい（口座違い。一覧では1行にまとめる。FR-4）。
--
-- すべての列を NOT NULL にする。CHECK と FK は NULL を検査しないので、NULL で FR-18〜20 を迂回させないため（プラン 4.2）。
-- acquisition_price（取得価格。円/株）を NUMERIC(…,1) にしないのは、PostgreSQL が小数第2位を黙って丸めて保存してしまうため。
-- 小数第2位以下を含む値は、丸めずに CHECK で拒む（FR-19、AC-15）。
-- 'NaN' と 'Infinity' は「0より大きい」と「小数第1位まで」の CHECK をどちらも通ってしまうので、有限の値に限る CHECK も付ける
-- （PostgreSQL は NaN を 'Infinity' より大きいとみなすので、< 'Infinity' で両方を拒める）。
CREATE TABLE holdings (
    id                BIGSERIAL   PRIMARY KEY,
    user_id           UUID        NOT NULL REFERENCES users (id),
    stock_code        TEXT        NOT NULL REFERENCES stocks (code),
    quantity          BIGINT      NOT NULL CHECK (quantity >= 1),
    acquisition_price NUMERIC     NOT NULL CHECK (acquisition_price > 0)
                                           CHECK (acquisition_price = round(acquisition_price, 1))
                                           CHECK (acquisition_price < 'Infinity'),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 一覧はいつも利用者で絞って読む（GET /api/holdings）。FK の列には索引が自動では作られないので作る。
CREATE INDEX holdings_user_id_idx ON holdings (user_id);

-- +goose Down
DROP TABLE holdings;
