-- +goose Up
-- ドルの銘柄を扱えるようにする（要件1・5。プラン 4.2 の 00006）。
--
-- 1. 銘柄マスタに通貨（円 JPY かドル USD）を足す。今ある銘柄は円なので、DEFAULT 'JPY' で足してから既定値を外す。
--    既定値を外すのは、銘柄を足す側（seed・テスト・PR③ の取り込み）に通貨を必ず書かせるため（通貨の書き忘れを円として黙って受け入れない）。
--    NOT NULL にするのは、NULL は CHECK を通ってしまうため。
-- 2. 終値の精度を、円の「小数第1位まで」から、ドルに合わせた「小数第2位まで」にする。
--    円の終値の「小数第1位まで」は、銘柄の通貨が別のテーブルにあり CHECK からは見られないので、DB では確かめず、
--    価格の取得の処理で確かめる（要件5）。
--    NUMERIC(…,2) にしない理由は 00004 と同じ（小数第3位以下を黙って丸めて保存してしまうため。丸めずに CHECK で拒む）。
--    終値の精度の CHECK は、列に付けた3つの CHECK の2番目（00004）なので、名前は stock_prices_close_price_check1（PostgreSQL が付けた名前）。
-- 3. 売買履歴の取引日を 0001-01-01 から 9999-12-31 に限る。年が4桁でない日付（10000-01-01 や紀元前）は、
--    "YYYY-MM-DD" の文字列で比べると並び順を誤るため（売買履歴を取引日の昇順にたどるのは、文字列の比較で行う。PR① のステップ6 の指摘）。
ALTER TABLE stocks
    ADD COLUMN currency TEXT NOT NULL DEFAULT 'JPY' CHECK (currency IN ('JPY', 'USD'));
ALTER TABLE stocks
    ALTER COLUMN currency DROP DEFAULT;

ALTER TABLE stock_prices
    DROP CONSTRAINT stock_prices_close_price_check1;
ALTER TABLE stock_prices
    ADD CONSTRAINT stock_prices_close_price_check1 CHECK (close_price = round(close_price, 2));

ALTER TABLE trades
    ADD CONSTRAINT trades_trade_date_range_check CHECK (trade_date BETWEEN '0001-01-01' AND '9999-12-31');

-- +goose Down
-- 00005 の状態に戻す。stocks の通貨の列を消し、終値の CHECK を小数第1位に戻し、取引日の範囲の CHECK を消す。
-- ドルの銘柄や、小数第2位の終値（0.01 単位）が入っている状態では、通貨の列は消えて円と区別がつかなくなり、
-- 終値の CHECK は小数第1位に戻せずに失敗する（中身は戻らない。00005 の down と同じ考え方）。
ALTER TABLE trades
    DROP CONSTRAINT trades_trade_date_range_check;

ALTER TABLE stock_prices
    DROP CONSTRAINT stock_prices_close_price_check1;
ALTER TABLE stock_prices
    ADD CONSTRAINT stock_prices_close_price_check1 CHECK (close_price = round(close_price, 1));

ALTER TABLE stocks
    DROP COLUMN currency;
