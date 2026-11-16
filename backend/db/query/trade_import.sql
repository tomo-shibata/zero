-- name: UserExists :one
-- 利用者 user_id が users にいるか。売買履歴の CSV の取り込みが、取り込む先の利用者を確かめるために使う（要件21）。
SELECT EXISTS (SELECT 1 FROM users WHERE id = sqlc.arg(user_id)) AS user_exists;

-- name: ListStocksByCodes :many
-- 銘柄コードが codes にある銘柄を、銘柄マスタから読む。取り込みが、CSV の銘柄が銘柄マスタにあるか・通貨と銘柄名が違わないかを確かめるために使う（要件25）。
-- 並びは決めない（呼び出し側が銘柄コードをキーにしたマップにする）。
SELECT code, name, currency
FROM stocks
WHERE code = ANY(sqlc.arg(codes)::text[]);

-- name: DeleteAllTradesOf :execrows
-- 利用者 user_id の売買履歴をすべて消す。取り込みが「入れ直し」の最初に行う（要件20）。他の利用者の売買履歴は消さない。
DELETE FROM trades
WHERE user_id = sqlc.arg(user_id);

-- name: InsertStock :exec
-- 銘柄マスタに銘柄を足す。取り込みが、銘柄マスタにない銘柄を CSV の銘柄コード・銘柄名・通貨で登録するために使う（要件20）。
INSERT INTO stocks (code, name, currency)
VALUES (sqlc.arg(code), sqlc.arg(name), sqlc.arg(currency));

-- name: RenameStock :execrows
-- 銘柄マスタの銘柄 code の銘柄名だけを変える（通貨は変えない）。取り込みが、銘柄名が CSV と違う銘柄の銘柄名を直すために使う（要件20・要件25）。
UPDATE stocks
SET name = sqlc.arg(name)
WHERE code = sqlc.arg(code);

-- name: LockUserForImport :exec
-- 利用者 user_id の行をロックする（トランザクションの終わりまで）。取り込みがトランザクションの最初に行い、同じ利用者への取り込みが同時に2つ動いて、売買履歴が二重になるのを防ぐ（要件20）。
SELECT 1 FROM users WHERE id = sqlc.arg(user_id) FOR UPDATE;
