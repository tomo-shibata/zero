-- name: UpsertClosingPrice :exec
-- 銘柄 stock_code の取引日 trading_date の終値を保存する。同じ銘柄・同じ取引日の行がすでにあれば、
-- 終値を取得した値で上書きし、取得日時も今に更新する（再取得した値を正とし、成功として扱う。FR-15）。
-- 取得日時は、追加するときは列の DEFAULT now() に任せる。
INSERT INTO stock_prices (stock_code, trading_date, close_price)
VALUES (sqlc.arg(stock_code), sqlc.arg(trading_date), sqlc.arg(close_price))
ON CONFLICT (stock_code, trading_date)
DO UPDATE SET close_price = EXCLUDED.close_price, fetched_at = now();
