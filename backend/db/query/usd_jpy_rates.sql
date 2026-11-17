-- name: GetLatestUsdJpyRate :one
-- 日付がいちばん新しいドル円のレートを1件返す（ドルを円にまとめる合計の要件14）。1件もなければ行がない（pgx.ErrNoRows）。
-- 主キー rate_date の索引を逆にたどって1行だけ読むので、レートが何件あっても速い（非機能1）。
SELECT rate_date, rate
FROM usd_jpy_rates
ORDER BY rate_date DESC
LIMIT 1;
