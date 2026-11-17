-- +goose Up
-- ドル円のレート（ドルを円にまとめる合計の要件13。プラン 4.2）。1行が「ある日の、1ドルあたりの円」。
-- 全利用者で共有するので、利用者の列は持たない。人間が SQL で入れる（要件15）。いちばん新しい日付のレートを使う（要件14）。
-- すべての列を NOT NULL にする。CHECK は NULL を検査しないので、NULL で決まりを迂回させないため。
-- rate_date も 'infinity' を入れられるので有限の日付に限り、年が4桁でない日付は "YYYY-MM-DD" の文字列で比べると並び順を誤るので
-- 範囲を限る（trades.trade_date と同じ理由。00006）。
-- rate は NUMERIC(…,2) にしない（小数第3位以下を黙って丸めて保存するため）。0 より大きく、小数第2位まで（round で比べる）、有限の値に限る。
-- NaN と Infinity は「0より大きい」と「小数第2位まで」を通ってしまうので、rate < 'Infinity' で拒む（stock_prices・trades と同じ）。
CREATE TABLE usd_jpy_rates (
    -- 同じ日付は1件だけ（要件13）。主キーの索引を逆にたどって、いちばん新しいレートを読む。
    rate_date  DATE        PRIMARY KEY CHECK (isfinite(rate_date))
                                       CHECK (rate_date BETWEEN '0001-01-01' AND '9999-12-31'),
    rate       NUMERIC     NOT NULL CHECK (rate > 0)
                                    CHECK (rate = round(rate, 2))
                                    CHECK (rate < 'Infinity'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
-- レートの表を消す（入れたレートは消える。売買履歴・銘柄マスタには触れない）。
DROP TABLE usd_jpy_rates;
