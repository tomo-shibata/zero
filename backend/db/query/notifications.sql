-- 価格の変動の通知の API（通知の一覧・見ていない数・見たにする。プラン 4.5）が使うクエリ。
-- 金額は 0.01 単位の整数（(列 * 100)::bigint）で受け取る。価格の列は小数第2位までの CHECK があるので、100 倍すると必ず整数になる。

-- name: ListNotifications :many
-- 利用者 user_id の通知を、取引日の降順 → 銘柄コードの昇順（COLLATE "C"）→ 全銘柄の条件が先 → 条件 ID の昇順に、100件まで返す（要件30）。
-- latest_id は、その利用者の通知の最大の ID（100件より古いものも含む）。一覧と同じ1つの SQL（同じスナップショット）で読むので、
-- 一覧にある通知の ID は必ず latest_id 以下になる。一覧が0件なら通知も0件なので、latest_id を読む行がない。
-- 索引は (user_id, trading_date DESC)。
SELECT
    n.id,
    n.stock_code,
    s.name AS stock_name,
    s.currency,
    to_char(n.trading_date, 'YYYY-MM-DD')::text AS trading_date,
    (NOT n.seen)::boolean AS is_new,
    n.kind,
    n.scope,
    n.condition_stock_code,
    n.direction,
    (n.threshold * 100)::bigint AS threshold_hundredths,
    (n.close_price * 100)::bigint AS close_hundredths,
    n.details,
    (SELECT max(m.id) FROM notifications AS m WHERE m.user_id = sqlc.arg(user_id))::bigint AS latest_id
FROM notifications AS n
JOIN stocks AS s ON s.code = n.stock_code
WHERE n.user_id = sqlc.arg(user_id)
ORDER BY n.trading_date DESC, n.stock_code COLLATE "C", (n.scope = 'all') DESC, n.condition_id
LIMIT 100;

-- name: CountUnseenNotifications :one
-- 利用者 user_id の、見ていない通知の数を返す。索引は (user_id) WHERE NOT seen。
SELECT count(*)::bigint
FROM notifications
WHERE user_id = sqlc.arg(user_id)
  AND NOT seen;

-- name: MarkNotificationsSeenUpTo :exec
-- 利用者 user_id の、ID が up_to_id 以下で見ていない通知を、見たにする（要件34）。ほかの利用者の通知は変えない。
UPDATE notifications
SET seen = true
WHERE user_id = sqlc.arg(user_id)
  AND id <= sqlc.arg(up_to_id)
  AND NOT seen;
