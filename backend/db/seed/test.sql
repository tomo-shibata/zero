-- テスト共通の seed（DML だけ）。
-- 内容はプラン（docs/plans/stock-holdings/plan.md）4.4「テストデータ」の表に従う。
-- テーブルは db/migrations/ のマイグレーションで作る。ここには DDL を書かない（テストルール 4）。
-- testdb.New がマイグレーションの後に流す。E2E の zero_e2e にも流す（プラン 4.5）。
-- 保有データ・価格は入れない（各テストで入れる）。

-- 利用者A・B（PR①）。created_at は DEFAULT now() に任せる。
INSERT INTO users (id) VALUES
    ('00000000-0000-0000-0000-00000000000a'), -- 利用者A
    ('00000000-0000-0000-0000-00000000000b'); -- 利用者B
