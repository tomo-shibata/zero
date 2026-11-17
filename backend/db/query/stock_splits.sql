-- name: ListStockSplitsOfUser :many
-- 利用者 user_id が売買履歴を持つ銘柄の分割を返す。1人の利用者の保有している銘柄の判定（価格の今すぐ更新）が、売買履歴に分割を混ぜるために使う（株式分割のプラン 4.3）。
-- 分割は全利用者で共有するので、銘柄を EXISTS で絞る（売買履歴が何件あっても、分割1件は1回だけ返る）。
-- 銘柄マスタは見ない。並びは決めない（Go の側が日付の順にたどる。プラン 4.2）。
SELECT ss.stock_code, ss.split_date, ss.ratio
FROM stock_splits AS ss
WHERE EXISTS (
    SELECT 1
    FROM trades AS t
    WHERE t.user_id = sqlc.arg(user_id)
      AND t.stock_code = ss.stock_code
);

-- name: ListStockSplitsByCodes :many
-- 銘柄コードが codes にある銘柄の分割を返す。売買履歴の CSV の取り込みが、多すぎる売りを分割も混ぜて確かめるために使う（株式分割の要件7）。
-- 銘柄マスタは見ない（銘柄マスタにない銘柄の分割も返す。要件11）。並びは決めない。
SELECT stock_code, split_date, ratio
FROM stock_splits
WHERE stock_code = ANY(sqlc.arg(codes)::text[]);

-- name: ListAllStockSplits :many
-- 登録済みの分割をすべて返す。保有している銘柄の判定（HeldStockCodes）が、全利用者の売買履歴に混ぜるために使う（プラン 4.3）。
-- 保有株式一覧の読み取り（HoldingsReader.ListStockSplits）も使う（売買履歴と別の読み取りなので、売買履歴で絞らない。プラン 4.8）。並びは決めない。
SELECT stock_code, split_date, ratio
FROM stock_splits;
