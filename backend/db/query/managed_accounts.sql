-- name: DeleteAllManagedAccountsOf :execrows
-- 利用者 user_id の口座をすべて消す（記録は ON DELETE CASCADE で一緒に消える。プラン 判断4）。おまかせ運用の取り込みが「入れ直し」の最初に行う（要件26）。
-- 他の利用者の口座と記録は消さない。利用者の存在の確認（UserExists）と行のロック（LockUser）は、売買履歴の取り込みと共通のものを使う。
-- 口座の読み取り（最新の記録）と、1つの口座の削除は PR② で足す。
DELETE FROM managed_accounts
WHERE user_id = sqlc.arg(user_id);
