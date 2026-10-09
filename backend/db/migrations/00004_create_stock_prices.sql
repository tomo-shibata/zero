-- +goose Up
-- 銘柄ごと・取引日ごとの終値（要件 FR-8、FR-15）。一覧の「現在の価格」は、銘柄ごとに取引日が最新の行の終値。
-- 同じ銘柄・同じ取引日は1行だけ（主キー）。再取得したときは上書きする（FR-15。書き込みは PR③）。
--
-- すべての列を NOT NULL にし、close_price（終値。円/株）を NUMERIC(…,1) にしない理由は holdings の acquisition_price と同じ
-- （00003_create_holdings.sql）。trading_date も 'infinity' を入れられるので、有限の日付に限る。
CREATE TABLE stock_prices (
    stock_code   TEXT        NOT NULL REFERENCES stocks (code),
    trading_date DATE        NOT NULL CHECK (isfinite(trading_date)),
    close_price  NUMERIC     NOT NULL CHECK (close_price > 0)
                                      CHECK (close_price = round(close_price, 1))
                                      CHECK (close_price < 'Infinity'),
    fetched_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (stock_code, trading_date)
);

-- +goose Down
DROP TABLE stock_prices;
