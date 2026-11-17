-- +goose Up
-- 株式分割（株式分割の要件1〜3。プラン 4.1）。1行が「ある銘柄の、ある日の1件の分割」。
-- 分割は売買履歴とは別に、銘柄ごとに全利用者で共有する（要件1）ので、利用者の列は持たない。
-- 銘柄マスタへの外部キーは付けない（要件11）。銘柄マスタにない銘柄の分割も、先に登録できるようにするため。
-- すべての列を NOT NULL にする。CHECK は NULL を検査しないので、NULL で決まりを迂回させないため。
-- split_date も 'infinity' を入れられるので有限の日付に限り、年が4桁でない日付は "YYYY-MM-DD" の文字列で比べると並び順を誤るので
-- 範囲を限る（trades.trade_date と同じ理由。00006）。
-- ratio は分割後の株数（1株が ratio 株になる）で、2〜1,000の整数（要件2）。併合は扱わない。
CREATE TABLE stock_splits (
    stock_code TEXT        NOT NULL CHECK (stock_code <> ''),
    split_date DATE        NOT NULL CHECK (isfinite(split_date))
                                    CHECK (split_date BETWEEN '0001-01-01' AND '9999-12-31'),
    ratio      INTEGER     NOT NULL CHECK (ratio BETWEEN 2 AND 1000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- 同じ銘柄・同じ分割の日は1件だけ（要件2）。
    PRIMARY KEY (stock_code, split_date)
);

-- +goose Down
-- 分割の表を消す（登録した分割は消える。売買履歴・銘柄マスタには触れない）。
DROP TABLE stock_splits;
