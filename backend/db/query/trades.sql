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

-- name: ListTradesOfAllUsers :many
-- 全利用者の売買履歴を、銘柄の通貨を付けて、登録した順（id の昇順）に返す。
-- 価格の取得処理が、誰かが保有している銘柄（保有数量が1以上の銘柄）を決めるために使う（売買履歴の要件5・要件16）。
-- 保有数量は分割を数えないと誤る（株式分割の要件4）ので、SQL では集計せず、Go の側が利用者ごとに分割も混ぜて
-- domain の HeldStockCodes で判定する（プラン 4.3。判断1）。価格・id は判定に要らないので読まない。
SELECT t.user_id, t.stock_code, s.currency, t.side, t.quantity, t.trade_date
FROM trades AS t
JOIN stocks AS s ON s.code = t.stock_code
ORDER BY t.id;

-- name: ListTradesOfUser :many
-- 利用者 user_id の売買履歴を、銘柄の通貨を付けて、登録した順（id の昇順）に返す。
-- 価格の今すぐ更新と、並びの保存が、その利用者の保有している銘柄を決めるために使う（売買履歴の要件16、カード表示の要件23）。
-- ListTradesOfAllUsers と同じ理由で、保有数量の判定は Go の側（domain の HeldStockCodes）で行う。
SELECT t.stock_code, s.currency, t.side, t.quantity, t.trade_date
FROM trades AS t
JOIN stocks AS s ON s.code = t.stock_code
WHERE t.user_id = sqlc.arg(user_id)
ORDER BY t.id;

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
