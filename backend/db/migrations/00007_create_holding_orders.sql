-- +goose Up
-- 保有株式一覧の並び（カード表示の要件23〜28。プラン 4.3、6章の判断4）。
-- 1行が「ある利用者の、ある銘柄の位置」。並びは position の昇順で、消した銘柄の行が抜けて飛び番になってもよい（順番だけを使う）。
-- 銘柄コードで覚える（銘柄マスタ・利用者に外部キーで結びつける）。同じ利用者の同じ銘柄と、同じ位置は1行だけ。
CREATE TABLE holding_orders (
    user_id    UUID    NOT NULL REFERENCES users (id),
    stock_code TEXT    NOT NULL REFERENCES stocks (code),
    position   INTEGER NOT NULL CHECK (position >= 0),
    PRIMARY KEY (user_id, stock_code),
    UNIQUE (user_id, position)
);

-- +goose Down
-- 並びの表を消す（保存した並びは消える。売買履歴・銘柄マスタには触れない）。
DROP TABLE holding_orders;
