-- name: ListTradeRows :many
-- 利用者 user_id の売買履歴を、集計せずに1件ずつ、銘柄名・通貨と最新の終値・その取引日を付けて返す。
-- 保有数量・取得価格・評価額の計算と並び替えは Go の純粋関数（application/query の CalculateHoldings）で行うので、
-- ここでは GROUP BY も ORDER BY もしない（取引日と id の順にたどるのも Go で行う。プラン 4.3）。
-- 価格が一度も保存されていない銘柄の行も返す（LEFT JOIN。close_price と trading_date が NULL になる。保有株式一覧の FR-10）。
-- 最新の終値は、その銘柄の取引日が最も新しい行（保有株式一覧の FR-8）。主キー (stock_code, trading_date) の索引で1行だけ読む。
SELECT
    t.id,
    t.stock_code,
    s.name,
    s.currency,
    t.side,
    t.quantity,
    t.price,
    t.trade_date,
    p.close_price,
    p.trading_date
FROM trades AS t
JOIN stocks AS s ON s.code = t.stock_code
LEFT JOIN LATERAL (
    SELECT sp.close_price, sp.trading_date
    FROM stock_prices AS sp
    WHERE sp.stock_code = t.stock_code
    ORDER BY sp.trading_date DESC
    LIMIT 1
) AS p ON true
WHERE t.user_id = sqlc.arg(user_id);

-- name: ListHeldStocks :many
-- 誰かの保有数量（買いの株数の合計 − 売りの株数の合計）が1以上の銘柄を、銘柄コードと通貨で、重複なし・コードの昇順で返す。
-- 価格の取得処理が、終値を取得する銘柄を決め、取得した終値の精度を通貨で確かめるために使う（要件5・要件16）。売り切った銘柄は含めない。
-- 保有数量は利用者と銘柄ごとに求め（HAVING）、そのうえで銘柄コードの重複を GROUP BY でまとめる
-- （SELECT DISTINCT だと、ORDER BY に COLLATE を付けた式を書けないため）。通貨は銘柄マスタの値で、銘柄コードごとに1つに決まる。
-- 並びは COLLATE "C"（バイト順）にして、Go の文字列比較の昇順と揃える。
-- DB の既定の照合順序（en_US.utf8）のままだと、例えば 130a が 130A より前に来て食い違うため。
SELECT s.code AS stock_code, s.currency
FROM (
    SELECT user_id, stock_code
    FROM trades
    GROUP BY user_id, stock_code
    HAVING sum(CASE side WHEN 'buy' THEN quantity ELSE -quantity END) >= 1
) AS held
JOIN stocks AS s ON s.code = held.stock_code
GROUP BY s.code, s.currency
ORDER BY s.code COLLATE "C";

-- name: ListHeldStocksOf :many
-- 利用者 user_id の保有数量（買いの株数の合計 − 売りの株数の合計）が1以上の銘柄を、銘柄コードと通貨で、重複なし・コードの昇順で返す。
-- 価格の今すぐ更新が、その利用者の保有銘柄だけの終値を取得するために使う（要件5・要件16）。
-- ListHeldStocks（全利用者）と同じ理由で、並びは COLLATE "C"（バイト順）にする。
SELECT s.code AS stock_code, s.currency
FROM trades AS t
JOIN stocks AS s ON s.code = t.stock_code
WHERE t.user_id = sqlc.arg(user_id)
GROUP BY s.code, s.currency
HAVING sum(CASE t.side WHEN 'buy' THEN t.quantity ELSE -t.quantity END) >= 1
ORDER BY s.code COLLATE "C";

-- name: DeleteTradesOf :execrows
-- 利用者 user_id の、銘柄 stock_code の売買履歴をすべて消し、消えた件数を返す。銘柄ごとの削除が使う（要件18）。
-- 他の利用者の売買履歴は user_id で、同じ利用者の別の銘柄は stock_code で除く。銘柄マスタと価格は別のテーブルなので消えない。
DELETE FROM trades
WHERE user_id = sqlc.arg(user_id)
  AND stock_code = sqlc.arg(stock_code);

-- name: LockUser :exec
-- 利用者 user_id の行をロックする（トランザクションの終わりまで）。同じ利用者の売買履歴・並びを書き換えるトランザクションが、最初に行って順番に動くようにする。
-- 取り込み（同じ利用者への取り込みが同時に2つ動いて売買履歴が二重になるのを防ぐ。売買履歴の要件20）・並びの保存・銘柄の削除（カード表示のプラン 4.3・判断6。
-- 保存と削除が同時に動いても、消した銘柄の並びの行が残らないようにする）で共通に使う。
SELECT 1 FROM users WHERE id = sqlc.arg(user_id) FOR UPDATE;
