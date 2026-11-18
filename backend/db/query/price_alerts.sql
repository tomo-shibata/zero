-- 価格の変動の通知の判定（プラン 4.1〜4.4）が使うクエリ。判定の入力を銘柄ごとに読み、判定の結果と通知を1つのトランザクションで書く。
-- 金額は 0.01 単位の整数（(列 * 100)::bigint）で受け取る。価格の列は小数第2位までの CHECK があるので、100 倍すると必ず整数になる。

-- name: GetAlertStockCurrency :one
-- 銘柄 stock_code の通貨を返す（銘柄マスタにない銘柄は0件）。
SELECT currency
FROM stocks
WHERE code = sqlc.arg(stock_code);

-- name: GetLatestTradingDate :one
-- 銘柄 stock_code の、最新の取引日（終値がある取引日のうち最も新しい日）を返す。終値が1件もなければ0件。主キー (stock_code, trading_date) の索引で1行だけ読む。
SELECT trading_date
FROM stock_prices
WHERE stock_code = sqlc.arg(stock_code)
ORDER BY trading_date DESC
LIMIT 1;

-- name: ListAlertConditionsOfStock :many
-- 銘柄 stock_code に効く条件（全利用者の、全銘柄の条件と、この銘柄の条件）を、足した順（id の昇順）に返す。
-- 値は 0.01 単位の整数。足した日は created_at の日本時間の日付（"YYYY-MM-DD"。要件15）。
SELECT
    id,
    user_id,
    kind,
    scope,
    stock_code,
    direction,
    (threshold * 100)::bigint AS threshold_hundredths,
    to_char((created_at AT TIME ZONE 'Asia/Tokyo')::date, 'YYYY-MM-DD')::text AS added_on
FROM alert_conditions
WHERE scope = 'all' OR stock_code = sqlc.arg(stock_code)
ORDER BY id;

-- name: ListTradesOfStockForUsers :many
-- 利用者 user_ids の、銘柄 stock_code の売買履歴を、銘柄マスタの銘柄名・通貨を付けて返す。並びは決めない（たどる順に並べるのは Go の側）。
-- 索引は trades_user_id_stock_code_idx。価格は 0.01 単位の整数。
SELECT
    t.id,
    t.user_id,
    t.stock_code,
    s.name,
    s.currency,
    t.side,
    t.quantity,
    (t.price * 100)::bigint AS price_hundredths,
    t.trade_date
FROM trades AS t
JOIN stocks AS s ON s.code = t.stock_code
WHERE t.stock_code = sqlc.arg(stock_code)
  AND t.user_id = ANY(sqlc.arg(user_ids)::uuid[]);

-- name: GetPreviousClose :one
-- 銘柄 stock_code の、取引日が before_date より前で最も新しい終値を返す（なければ0件）。主キーの索引で1行だけ読む。
SELECT trading_date, (close_price * 100)::bigint AS close_hundredths
FROM stock_prices
WHERE stock_code = sqlc.arg(stock_code)
  AND trading_date < sqlc.arg(before_date)
ORDER BY trading_date DESC
LIMIT 1;

-- name: ListClosesBetween :many
-- 銘柄 stock_code の、取引日が from_date 以後 to_date 以前の終値を、取引日の昇順に返す（最高値の条件があるときだけ使う。要件12）。
-- 昇順を ORDER BY で必ず保証する。索引は主キー (stock_code, trading_date)。
SELECT trading_date, (close_price * 100)::bigint AS close_hundredths
FROM stock_prices
WHERE stock_code = sqlc.arg(stock_code)
  AND trading_date >= sqlc.arg(from_date)
  AND trading_date <= sqlc.arg(to_date)
ORDER BY trading_date;

-- name: ListPreviousAlertJudgments :many
-- 条件 condition_ids の、銘柄 stock_code の、取引日が before_date より前で最も新しい判定の結果を、条件ごとに1件返す（要件16）。主キーの索引をたどる。
SELECT DISTINCT ON (condition_id) condition_id, trading_date, satisfied
FROM alert_judgments
WHERE stock_code = sqlc.arg(stock_code)
  AND trading_date < sqlc.arg(before_date)
  AND condition_id = ANY(sqlc.arg(condition_ids)::bigint[])
ORDER BY condition_id, trading_date DESC;

-- name: UpsertAlertJudgments :exec
-- 条件 condition_ids の、銘柄 stock_code・取引日 trading_date の判定の結果を、まとめて satisfied（当てはまったか）で書く
-- （同じ取引日を保存し直したら上書きする。要件18）。当てはまった条件と当てはまらなかった条件で、2回に分けて呼ぶ。
-- 判定の途中で消えた条件は、alert_conditions から選ぶので書かれない（外部キーの違反で、ほかの条件の結果まで書けなくなるのを避けるため）。
-- 残っている条件は FOR KEY SHARE でロックし、条件を消す側がこのトランザクションのコミットまで待つようにする（選んだ後に消されて外部キーの違反になるのを防ぐ）。
INSERT INTO alert_judgments (condition_id, stock_code, trading_date, satisfied)
SELECT c.id, sqlc.arg(stock_code)::text, sqlc.arg(trading_date)::date, sqlc.arg(satisfied)::boolean
FROM alert_conditions AS c
WHERE c.id = ANY(sqlc.arg(condition_ids)::bigint[])
ORDER BY c.id
FOR KEY SHARE OF c
ON CONFLICT (condition_id, stock_code, trading_date)
DO UPDATE SET satisfied = EXCLUDED.satisfied;

-- name: PruneAlertJudgments :exec
-- 条件 condition_ids の、銘柄 stock_code の判定の結果を、取引日の新しい順に2件だけ残し、それより古い行を消す（プラン 4.2。
-- 前の判定として使うのは1件だが、同じ取引日を保存し直したとき前の日の結果が要るので2件）。
DELETE FROM alert_judgments AS j
USING (
    SELECT a.condition_id, a.trading_date,
           row_number() OVER (PARTITION BY a.condition_id ORDER BY a.trading_date DESC) AS position
    FROM alert_judgments AS a
    WHERE a.stock_code = sqlc.arg(stock_code)
      AND a.condition_id = ANY(sqlc.arg(condition_ids)::bigint[])
) AS ranked
WHERE j.stock_code = sqlc.arg(stock_code)
  AND j.condition_id = ranked.condition_id
  AND j.trading_date = ranked.trading_date
  AND ranked.position > 2;

-- name: InsertAlertNotifications :exec
-- 通知をまとめて追加する（1回の問い合わせで）。notifications は通知の JSON の配列で、各要素は
-- {user_id, condition_id, kind, scope, condition_stock_code, direction, threshold_hundredths, stock_code, trading_date, close_hundredths, details}。
-- 同じ（条件・銘柄・取引日）の通知があれば何もしない（作った通知は書き換えない。要件18）。threshold・close_price は 100 で割って小数第2位までの NUMERIC にする。
-- 残っている条件で、持ち主が通知の user_id と同じものの分だけ入れる（判定の途中で消された条件や、持ち主の違う通知は入れない）。
-- 条件は FOR KEY SHARE でロックし（入れる順とロックの順は条件 ID の昇順）、条件を消す側がこのトランザクションのコミットまで待つようにする。
INSERT INTO notifications (
    user_id, condition_id, kind, scope, condition_stock_code, direction, threshold,
    stock_code, trading_date, close_price, details
)
SELECT
    n.user_id, n.condition_id, n.kind, n.scope, n.condition_stock_code, n.direction,
    round(n.threshold_hundredths::numeric / 100, 2),
    n.stock_code, n.trading_date, round(n.close_hundredths::numeric / 100, 2), n.details
FROM jsonb_to_recordset(sqlc.arg(notifications)::jsonb) AS n (
    user_id uuid, condition_id bigint, kind text, scope text, condition_stock_code text, direction text,
    threshold_hundredths bigint, stock_code text, trading_date date, close_hundredths bigint, details jsonb
)
JOIN alert_conditions AS c ON c.id = n.condition_id AND c.user_id = n.user_id
ORDER BY n.condition_id
FOR KEY SHARE OF c
ON CONFLICT (condition_id, stock_code, trading_date) DO NOTHING;
