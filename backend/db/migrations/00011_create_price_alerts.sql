-- +goose Up
-- 価格の変動の通知（プラン 4.2）。alert_conditions は利用者ごとの条件、alert_judgments は条件と銘柄の組ごとの判定の結果（前の判定を覚えるため）、
-- notifications は作った通知（条件の中身を写して持つ）。
-- すべての列を NOT NULL にする。CHECK は NULL を検査しないので、NULL で決まりを迂回させないため。
-- 条件の値（threshold）の種類と通貨ごとの精度と範囲（要件4）は、通貨が別のテーブルにあるので DB では確かめず、domain（NewAlertCondition）で確かめる。
CREATE TABLE alert_conditions (
    -- 足した順。同じ対象の中の並びに使う（要件40）。
    id         BIGSERIAL   PRIMARY KEY,
    user_id    UUID        NOT NULL REFERENCES users (id),
    -- 取得価格から・最高値から・前の取引日から・含み損益の金額・決めた価格。
    kind       TEXT        NOT NULL CHECK (kind IN ('cost', 'peak', 'prev', 'pnl', 'price')),
    scope      TEXT        NOT NULL CHECK (scope IN ('all', 'stock')),
    -- 全銘柄は ''、1銘柄は銘柄コード。'' を入れるので外部キーは付けない（足すときに、アプリで保有している銘柄かを確かめる）。
    stock_code TEXT        NOT NULL,
    direction  TEXT        NOT NULL,
    -- 値。% の種類は 20 のような %、含み損益の金額と決めた価格はその通貨の金額。0 より大きく、小数第2位まで（小数第3位以下を黙って丸めずに拒む）。
    -- NaN は '< Infinity' で拒む（NaN は Infinity より大きいと数えられる）。上限は10億（要件4 の最大は円の含み損益の10億。判定の (threshold * 100)::bigint があふれないようにする）。
    threshold  NUMERIC     NOT NULL CHECK (threshold > 0)
                                    CHECK (threshold = round(threshold, 2))
                                    CHECK (threshold < 'Infinity')
                                    CHECK (threshold <= 1000000000),
    -- 日本時間の日付を「足した日」に使う（要件15）。
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((scope = 'all') = (stock_code = '')),
    -- 含み損益の金額と決めた価格は、1銘柄だけ（要件2）。
    CHECK (kind NOT IN ('pnl', 'price') OR scope = 'stock'),
    -- 種類ごとの向きの組み合わせ（要件2）。
    CHECK (
        (kind = 'cost' AND direction IN ('up', 'down'))
        OR (kind = 'peak' AND direction = 'down')
        OR (kind = 'prev' AND direction IN ('up', 'down', 'either'))
        OR (kind = 'pnl' AND direction IN ('loss', 'gain'))
        OR (kind = 'price' AND direction IN ('above', 'below'))
    ),
    -- 同じ条件は1つだけ（要件5）。NUMERIC の比較なので 20 と 20.0 は同じ。
    -- 先頭が user_id なので、利用者ごとの一覧はこの制約の索引で引ける（user_id だけの索引は足さない）。
    UNIQUE (user_id, kind, scope, stock_code, direction, threshold)
);

-- 判定で「全銘柄、またはこの銘柄」の条件を探す。
CREATE INDEX alert_conditions_stock_code_idx ON alert_conditions (stock_code);

CREATE TABLE alert_judgments (
    -- 条件を消すと、判定の結果も消える。
    condition_id BIGINT  NOT NULL REFERENCES alert_conditions (id) ON DELETE CASCADE,
    stock_code   TEXT    NOT NULL REFERENCES stocks (code),
    -- trade_date と同じ理由で、有限の日付に限り、年が4桁の範囲にする（00006）。
    trading_date DATE    NOT NULL CHECK (isfinite(trading_date))
                                  CHECK (trading_date BETWEEN '0001-01-01' AND '9999-12-31'),
    -- 当てはまったか。
    satisfied    BOOLEAN NOT NULL,
    -- 同じ取引日を保存し直したら上書きする（要件18）。組ごとに新しい2件だけを残す（書き込みの後に古い行を消す）。
    PRIMARY KEY (condition_id, stock_code, trading_date)
);

CREATE TABLE notifications (
    -- 見たにする範囲（一覧を取得したときの最大の ID 以下）に使う。
    id                   BIGSERIAL   PRIMARY KEY,
    user_id              UUID        NOT NULL REFERENCES users (id),
    -- 外部キーにしない。条件を消しても通知を残す（要件23）。
    condition_id         BIGINT      NOT NULL,
    -- 作ったときの条件の中身（条件の文を出すため。要件31）。alert_conditions と同じ型と CHECK。
    kind                 TEXT        NOT NULL CHECK (kind IN ('cost', 'peak', 'prev', 'pnl', 'price')),
    scope                TEXT        NOT NULL CHECK (scope IN ('all', 'stock')),
    condition_stock_code TEXT        NOT NULL,
    direction            TEXT        NOT NULL,
    threshold            NUMERIC     NOT NULL CHECK (threshold > 0)
                                              CHECK (threshold = round(threshold, 2))
                                              CHECK (threshold < 'Infinity')
                                              CHECK (threshold <= 1000000000),
    stock_code           TEXT        NOT NULL REFERENCES stocks (code),
    -- 判定した終値の取引日。
    trading_date         DATE        NOT NULL CHECK (isfinite(trading_date))
                                              CHECK (trading_date BETWEEN '0001-01-01' AND '9999-12-31'),
    -- 判定した終値。stock_prices.close_price と同じ CHECK。
    close_price          NUMERIC     NOT NULL CHECK (close_price > 0)
                                              CHECK (close_price = round(close_price, 2))
                                              CHECK (close_price < 'Infinity'),
    -- 種類で変わる値（率・基準の価格とその日・含み損益・株数・分割で割った値）。中身の形はアプリの型（dto.AlertNotificationDetails）で決める。
    details              JSONB       NOT NULL,
    seen                 BOOLEAN     NOT NULL DEFAULT false,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((scope = 'all') = (condition_stock_code = '')),
    CHECK (kind NOT IN ('pnl', 'price') OR scope = 'stock'),
    CHECK (
        (kind = 'cost' AND direction IN ('up', 'down'))
        OR (kind = 'peak' AND direction = 'down')
        OR (kind = 'prev' AND direction IN ('up', 'down', 'either'))
        OR (kind = 'pnl' AND direction IN ('loss', 'gain'))
        OR (kind = 'price' AND direction IN ('above', 'below'))
    ),
    -- 同じ条件・銘柄・取引日の通知は1件まで（要件18。同じ取引日を保存し直しても2件目を作らない）。
    UNIQUE (condition_id, stock_code, trading_date)
);

-- 一覧（利用者ごとに、取引日の新しい順）。
CREATE INDEX notifications_user_id_trading_date_idx ON notifications (user_id, trading_date DESC);
-- 見ていない数。
CREATE INDEX notifications_user_id_unseen_idx ON notifications (user_id) WHERE NOT seen;

-- +goose Down
-- 価格の変動の通知の3つの表を消す（作った条件・判定の結果・通知は消える。売買履歴・銘柄マスタ・価格には触れない）。
-- 判定の結果の表は条件の表への外部キーがあるので先に消す。
DROP TABLE notifications;
DROP TABLE alert_judgments;
DROP TABLE alert_conditions;
