-- +goose Up
-- 保有データ（holdings）をやめ、売買履歴（trades）を正にする（要件2〜4、プラン 4.2）。
-- 保有数量と取得価格は保存せず、一覧を取得するたびに売買履歴から計算する（要件4）。
-- 今ある holdings の中身は売買履歴へ移さない（本番はまだ動いていない。要件定義の「前提」）。
--
-- id は登録した順（要件8）。同じ取引日の売買履歴は id の昇順にたどる。
-- すべての列を NOT NULL にする。CHECK と FK は NULL を検査しないので、NULL で要件3 を迂回させないため。
-- price（1株あたりの価格）は、ドルに合わせて小数第2位までにする。円の「小数第1位まで」は、銘柄の通貨が別のテーブルにあり
-- CHECK からは見られないので、DB では確かめない（判断4）。通貨の列は PR②（00006）で stocks に足す。円の精度の確かめは
-- PR③ の CSV の取り込みで行う。それまでは SQL で入れる値を確かめない。
-- NUMERIC(…,2) にしないのは、PostgreSQL が小数第3位以下を黙って丸めて保存してしまうため。丸めずに CHECK で拒む。
-- 'NaN' と 'Infinity' は「0より大きい」と「小数第2位まで」の CHECK をどちらも通ってしまうので、有限の値に限る CHECK も付ける
-- （PostgreSQL は NaN を 'Infinity' より大きいとみなすので、< 'Infinity' で両方を拒める）。
-- trade_date も 'infinity' を入れられるので、有限の日付に限る。
-- 「持っている株数より多く売らない」（要件9）は、行をまたぐ決まりなので DB では確かめない（PR③ の取り込みで確かめる）。
CREATE TABLE trades (
    id         BIGSERIAL   PRIMARY KEY,
    user_id    UUID        NOT NULL REFERENCES users (id),
    stock_code TEXT        NOT NULL REFERENCES stocks (code),
    side       TEXT        NOT NULL CHECK (side IN ('buy', 'sell')),
    quantity   BIGINT      NOT NULL CHECK (quantity >= 1),
    price      NUMERIC     NOT NULL CHECK (price > 0)
                                    CHECK (price = round(price, 2))
                                    CHECK (price < 'Infinity'),
    trade_date DATE        NOT NULL CHECK (isfinite(trade_date)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 一覧と保有銘柄の読み取りは、いつも利用者で絞り、銘柄ごとに集める。FK の列には索引が自動では作られないので作る。
CREATE INDEX trades_user_id_stock_code_idx ON trades (user_id, stock_code);

DROP TABLE holdings;

-- +goose Down
-- holdings を 00003 の形に作り直し、trades を消す。trades の中身は holdings に戻らない。
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

CREATE INDEX holdings_user_id_idx ON holdings (user_id);

DROP TABLE trades;
