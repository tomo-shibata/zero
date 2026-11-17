-- name: ListHoldingOrder :many
-- 利用者 user_id の保存した並び（銘柄コード）を、位置の昇順で返す。保存していなければ0行（カード表示の要件23・要件26。プラン 4.3「読む」）。
SELECT stock_code
FROM holding_orders
WHERE user_id = sqlc.arg(user_id)
ORDER BY position;

-- name: DeleteHoldingOrdersOf :exec
-- 利用者 user_id の並びの行をすべて消す。並びの保存が「置き直し」の最初に行う（プラン 4.3「保存する」）。他の利用者の行は消さない。
DELETE FROM holding_orders
WHERE user_id = sqlc.arg(user_id);

-- name: InsertHoldingOrders :exec
-- 利用者 user_id の並びとして、codes を配列の順に position 0, 1, 2, … で入れる。並びの保存が、全部を消した後に行う（プラン 4.3「保存する」）。
INSERT INTO holding_orders (user_id, stock_code, position)
SELECT sqlc.arg(user_id)::uuid, c.stock_code, (c.ordinality - 1)::int
FROM unnest(sqlc.arg(codes)::text[]) WITH ORDINALITY AS c (stock_code, ordinality);

-- name: DeleteHoldingOrderOf :exec
-- 利用者 user_id の、銘柄 stock_code の並びの行を消す。銘柄の削除が、売買履歴と同じトランザクションで行う（要件28。プラン 4.3「消すとき」）。
-- 他の利用者の行と、同じ利用者の別の銘柄の行は消さない。
DELETE FROM holding_orders
WHERE user_id = sqlc.arg(user_id)
  AND stock_code = sqlc.arg(stock_code);

-- name: DeleteHoldingOrdersExcept :exec
-- 利用者 user_id の並びの行のうち、銘柄コードが codes にない銘柄の行を消す。codes が空なら、その利用者の行をすべて消す。
-- 保有している銘柄（codes）は、呼び出し側が分割も混ぜて決める（保有の判定を SQL に持たない。株式分割のプラン 4.3）。
-- 売買履歴の CSV の取り込み（売買履歴を入れた後。カード表示の要件28）と、分割の取り込み（全利用者）が使う。他の利用者の行は消さない。
DELETE FROM holding_orders
WHERE user_id = sqlc.arg(user_id)
  AND NOT (stock_code = ANY(coalesce(sqlc.arg(codes)::text[], '{}')));
