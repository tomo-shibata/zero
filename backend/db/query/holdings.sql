-- name: ListHoldingRows :many
-- 利用者 user_id の保有データを、集約せずに1件ずつ、銘柄名と最新の終値・その取引日を付けて返す。
-- まとめ・並び替え・評価額の計算は Go の純粋関数（application/query の AggregateHoldings）で行うので、
-- ここでは GROUP BY も ORDER BY もしない（プラン 6章の判断3）。
-- 価格が一度も保存されていない銘柄も返す（LEFT JOIN。close_price と trading_date が NULL になる。FR-10）。
-- 最新の終値は、その銘柄の取引日が最も新しい行（FR-8）。主キー (stock_code, trading_date) の索引で1行だけ読む。
SELECT
    h.stock_code,
    s.name,
    h.quantity,
    h.acquisition_price,
    p.close_price,
    p.trading_date
FROM holdings AS h
JOIN stocks AS s ON s.code = h.stock_code
LEFT JOIN LATERAL (
    SELECT sp.close_price, sp.trading_date
    FROM stock_prices AS sp
    WHERE sp.stock_code = h.stock_code
    ORDER BY sp.trading_date DESC
    LIMIT 1
) AS p ON true
WHERE h.user_id = sqlc.arg(user_id);
