-- name: ListTradesOfStock :many
-- 利用者 user_id の、銘柄 stock_code の売買履歴を、銘柄マスタの銘柄名・通貨を付けて返す（銘柄ごとの売買履歴の画面。プラン 4.2）。
-- 並びは決めない（取引日と id の順に並べるのは Go の BuildStockHistory で行う）。
-- 銘柄マスタにない銘柄コードは JOIN で0件になる（売買履歴を持たない銘柄と同じ扱い。要件20）。
-- 索引は trades_user_id_stock_code_idx。
SELECT
    t.id,
    t.stock_code,
    s.name,
    s.currency,
    t.side,
    t.quantity,
    t.price,
    t.trade_date
FROM trades AS t
JOIN stocks AS s ON s.code = t.stock_code
WHERE t.user_id = sqlc.arg(user_id)
  AND t.stock_code = sqlc.arg(stock_code);

-- name: ListStockSplitsOfCode :many
-- 銘柄 stock_code の分割を返す（分割は全利用者で共有。銘柄マスタは見ない）。並びは決めない。
-- 索引は主キー (stock_code, split_date)。
SELECT stock_code, split_date, ratio
FROM stock_splits
WHERE stock_code = sqlc.arg(stock_code);

-- name: ListClosingPricesSince :many
-- 銘柄 stock_code の、取引日が from_date と同じかそれより後の終値を、取引日の昇順に返す（銘柄ごとの売買履歴のチャート。要件7）。
-- 昇順を ORDER BY で必ず保証する（主キーの索引の順に頼らない）。索引は主キー (stock_code, trading_date)。
SELECT trading_date, close_price
FROM stock_prices
WHERE stock_code = sqlc.arg(stock_code)
  AND trading_date >= sqlc.arg(from_date)
ORDER BY trading_date;
