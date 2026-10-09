-- +goose Up
-- 銘柄マスタ（要件 FR-20）。一覧に出す銘柄名はここから取る。
-- 保有データ（holdings）と価格（stock_prices）の銘柄コードは、ここに登録済みのものに限る（それぞれの FK）。
-- 銘柄コードの形式（文字数・使える文字）は要件にないので、空文字だけを拒む（プラン 4.2）。
CREATE TABLE stocks (
    code TEXT PRIMARY KEY CHECK (code <> ''),
    name TEXT NOT NULL CHECK (name <> '')
);

-- +goose Down
DROP TABLE stocks;
