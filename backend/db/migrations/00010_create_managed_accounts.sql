-- +goose Up
-- おまかせ運用（WealthNavi など、投資一任の口座。おまかせ運用の要件1〜3。プラン 4.2）。
-- managed_accounts は利用者ごとの口座（名前で見分ける）、managed_account_valuations は口座の日付ごとの記録（評価額と損益。金額は円の整数）。
-- 元本と率は保存しない（要件4。画面に出すたびに最新の記録から計算する）。
-- すべての列を NOT NULL にする。CHECK は NULL を検査しないので、NULL で決まりを迂回させないため。
CREATE TABLE managed_accounts (
    user_id    UUID        NOT NULL REFERENCES users (id),
    -- 名前は書いたとおりの文字列で比べる（要件1）。DB は空文字だけを拒み、空白だけの名前は取り込みで拒む（stocks.name と同じ）。
    name       TEXT        NOT NULL CHECK (name <> ''),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- 同じ利用者の同じ名前の口座は1つだけ（要件1）。
    PRIMARY KEY (user_id, name)
);

CREATE TABLE managed_account_valuations (
    user_id        UUID        NOT NULL,
    name           TEXT        NOT NULL,
    -- valuation_date も 'infinity' を入れられるので有限の日付に限り、年が4桁でない日付は "YYYY-MM-DD" の文字列で比べると並び順を誤るので
    -- 範囲を限る（trades.trade_date と同じ理由。00006）。
    valuation_date DATE        NOT NULL CHECK (isfinite(valuation_date))
                                        CHECK (valuation_date BETWEEN '0001-01-01' AND '9999-12-31'),
    -- 評価額は1以上1兆未満、損益は絶対値が1兆未満（0とマイナスを含む）。どちらも円の整数（要件2・要件3）。
    valuation      BIGINT      NOT NULL CHECK (valuation BETWEEN 1 AND 999999999999),
    gain           BIGINT      NOT NULL CHECK (gain BETWEEN -999999999999 AND 999999999999),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- 同じ口座の同じ日付の記録は1件だけ（要件2）。口座ごとの最新の記録は、この主キーの索引をたどって読む。
    PRIMARY KEY (user_id, name, valuation_date),
    -- 口座を消すと記録も必ず消える（プラン 判断4）。
    FOREIGN KEY (user_id, name) REFERENCES managed_accounts (user_id, name) ON DELETE CASCADE,
    -- 元本（評価額 − 損益）は1円以上（要件3。0以下では率を計算できない）。
    CHECK (valuation - gain >= 1)
);

-- +goose Down
-- おまかせ運用の2つの表を消す（取り込んだ口座と記録は消える。売買履歴・銘柄マスタには触れない）。記録の表を先に消す（口座の表への外部キーがあるため）。
DROP TABLE managed_account_valuations;
DROP TABLE managed_accounts;
