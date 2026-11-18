-- name: DeleteAllManagedAccountsOf :execrows
-- 利用者 user_id の口座をすべて消す（記録は ON DELETE CASCADE で一緒に消える。プラン 判断4）。おまかせ運用の取り込みが「入れ直し」の最初に行う（要件26）。
-- 他の利用者の口座と記録は消さない。利用者の存在の確認（UserExists）と行のロック（LockUser）は、売買履歴の取り込みと共通のものを使う。
-- 最新の記録の読み取りと、1つの口座の削除は、下に置く（PR②）。
DELETE FROM managed_accounts
WHERE user_id = sqlc.arg(user_id);

-- name: ListLatestManagedValuations :many
-- 利用者 user_id の口座ごとに、日付がいちばん新しい記録を1件ずつ返す（要件4・要件5）。口座の並びは決めない（名前の順は Go で並べる。プラン 判断6）。
-- 主キー（user_id, name, valuation_date）の索引をたどる。記録のない口座は、口座の表にあっても返さない（取り込みは必ず記録を入れる）。
SELECT DISTINCT ON (name)
    name,
    valuation_date,
    valuation,
    gain
FROM managed_account_valuations
WHERE user_id = sqlc.arg(user_id)
ORDER BY name, valuation_date DESC;

-- name: DeleteManagedAccountOf :execrows
-- 利用者 user_id の名前 name の口座を1つ消す（記録は ON DELETE CASCADE で一緒に消える。プラン 判断4）。名前は書いたとおりに比べる（要件1）。
-- 他の利用者の口座と記録は消さない。行のロック（LockUser）は呼び出し側が先に行う。
DELETE FROM managed_accounts
WHERE user_id = sqlc.arg(user_id)
  AND name = sqlc.arg(name);
