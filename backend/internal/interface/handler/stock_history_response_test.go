package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"reflect"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// 仕様: 銘柄ごとの売買履歴の画面の要件（docs/requirements/stock-history.md）の基準18・基準19（要件15・要件16）と、
// プラン（docs/plans/stock-history/plan.md）4.4「API の形」・4.8 の PR①（newStockHistoryResponse）、5.1 の基準18・基準19 の行。
// 金額は 0.01 単位の整数から、浮動小数点を通さずに文字列にする（円は小数1桁、ドルは小数2桁。金額 amount も単価と同じ形）。
// 確かめるのは、newStockHistoryResponse の結果を json.Marshal した本文。

// marshalStockHistory は、h から newStockHistoryResponse で本文を作り、json.Marshal した JSON を返す。
func marshalStockHistory(t *testing.T, h dto.StockHistory) []byte {
	t.Helper()
	resp, err := newStockHistoryResponse(h)
	if err != nil {
		t.Fatalf("newStockHistoryResponse: got err %v, want nil", err)
	}
	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return body
}

// decodeHistoryJSON は、data を1つの JSON の値として読む。数は json.Number のまま残す（"3" と 3 を別のものとして比べるため）。
func decodeHistoryJSON(t *testing.T, data []byte) any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("JSON として読めない: %v\n本文: %s", err, data)
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		t.Fatalf("JSON の値の後に余分なものがある: %s", data)
	}
	return v
}

// assertHistoryJSON は、本文 got が JSON として want と同じ（キーの集まり・値・値の種類・配列の順がすべて同じ）であることを確かめる。
func assertHistoryJSON(t *testing.T, got []byte, want string) {
	t.Helper()
	if !reflect.DeepEqual(decodeHistoryJSON(t, got), decodeHistoryJSON(t, []byte(want))) {
		t.Errorf("本文の JSON が違う\n got: %s\nwant: %s", got, want)
	}
}

func histDivisor(n int64) *big.Int { return big.NewInt(n) }

// プラン 4.4 の JSON の例どおりの形（金額は文字列、splitDivisor は整数の文字列、株数と分割後の株数は数）。
// 売買の行は8つの欄だけで ratio がない。分割の行は kind・date・ratio だけで side・price などがない。
func Test基準18_プラン4_4の例どおりの本文を作る(t *testing.T) {
	t.Parallel()
	h := dto.StockHistory{
		Code: "7203", Name: "トヨタ自動車", Currency: dto.CurrencyJPY,
		Entries: []dto.StockHistoryEntry{
			{Kind: dto.StockHistoryEntryTrade, Date: "2026-03-31", Side: dto.TradeSideBuy, Quantity: 30,
				PriceHundredths: 100000, AmountHundredths: 3000000, SplitDivisor: histDivisor(1), AdjustedPriceHundredths: 100000},
			{Kind: dto.StockHistoryEntrySplit, Date: "2026-03-31", Ratio: 3},
			{Kind: dto.StockHistoryEntryTrade, Date: "2026-01-10", Side: dto.TradeSideBuy, Quantity: 100,
				PriceHundredths: 250000, AmountHundredths: 25000000, SplitDivisor: histDivisor(3), AdjustedPriceHundredths: 83330},
		},
		Closes: []dto.StockHistoryClose{
			{Date: "2026-03-30", PriceHundredths: 310000, SplitDivisor: histDivisor(3), AdjustedPriceHundredths: 103330},
			{Date: "2026-03-31", PriceHundredths: 105000, SplitDivisor: histDivisor(1), AdjustedPriceHundredths: 105000},
		},
	}

	body := marshalStockHistory(t, h)

	assertHistoryJSON(t, body, `{
		"stock": {"code": "7203", "name": "トヨタ自動車", "currency": "JPY"},
		"entries": [
			{"kind": "trade", "date": "2026-03-31", "side": "buy", "quantity": 30,
			 "price": "1000.0", "amount": "30000.0", "splitDivisor": "1", "adjustedPrice": "1000.0"},
			{"kind": "split", "date": "2026-03-31", "ratio": 3},
			{"kind": "trade", "date": "2026-01-10", "side": "buy", "quantity": 100,
			 "price": "2500.0", "amount": "250000.0", "splitDivisor": "3", "adjustedPrice": "833.3"}
		],
		"closes": [
			{"date": "2026-03-30", "price": "3100.0", "splitDivisor": "3", "adjustedPrice": "1033.3"},
			{"date": "2026-03-31", "price": "1050.0", "splitDivisor": "1", "adjustedPrice": "1050.0"}
		]
	}`)
}

// 基準18（要件15・要件16）: 売買「買 1,234.5円 3株」「買 2,500円 100株」→ 価格と金額は「"1234.5" / "3703.5"」「"2500.0" / "250000.0"」。
// 「買 150.25ドル 10株」→ 「"150.25" / "1502.50"」（ドルは小数2桁。末尾の0も省かない）。
func Test基準18_価格と金額は円が小数1桁でドルが小数2桁の文字列(t *testing.T) {
	t.Parallel()

	t.Run("円", func(t *testing.T) {
		t.Parallel()
		h := dto.StockHistory{
			Code: "7203", Name: "トヨタ自動車", Currency: dto.CurrencyJPY,
			Entries: []dto.StockHistoryEntry{
				{Kind: dto.StockHistoryEntryTrade, Date: "2026-02-10", Side: dto.TradeSideBuy, Quantity: 100,
					PriceHundredths: 250000, AmountHundredths: 25000000, SplitDivisor: histDivisor(1), AdjustedPriceHundredths: 250000},
				{Kind: dto.StockHistoryEntryTrade, Date: "2026-01-10", Side: dto.TradeSideBuy, Quantity: 3,
					PriceHundredths: 123450, AmountHundredths: 370350, SplitDivisor: histDivisor(1), AdjustedPriceHundredths: 123450},
			},
			Closes: []dto.StockHistoryClose{},
		}

		body := marshalStockHistory(t, h)

		assertHistoryJSON(t, body, `{
			"stock": {"code": "7203", "name": "トヨタ自動車", "currency": "JPY"},
			"entries": [
				{"kind": "trade", "date": "2026-02-10", "side": "buy", "quantity": 100,
				 "price": "2500.0", "amount": "250000.0", "splitDivisor": "1", "adjustedPrice": "2500.0"},
				{"kind": "trade", "date": "2026-01-10", "side": "buy", "quantity": 3,
				 "price": "1234.5", "amount": "3703.5", "splitDivisor": "1", "adjustedPrice": "1234.5"}
			],
			"closes": []
		}`)
	})

	t.Run("ドル", func(t *testing.T) {
		t.Parallel()
		h := dto.StockHistory{
			Code: "AAPL", Name: "Apple", Currency: dto.CurrencyUSD,
			Entries: []dto.StockHistoryEntry{
				{Kind: dto.StockHistoryEntryTrade, Date: "2026-01-10", Side: dto.TradeSideBuy, Quantity: 10,
					PriceHundredths: 15025, AmountHundredths: 150250, SplitDivisor: histDivisor(1), AdjustedPriceHundredths: 15025},
			},
			Closes: []dto.StockHistoryClose{},
		}

		body := marshalStockHistory(t, h)

		assertHistoryJSON(t, body, `{
			"stock": {"code": "AAPL", "name": "Apple", "currency": "USD"},
			"entries": [
				{"kind": "trade", "date": "2026-01-10", "side": "buy", "quantity": 10,
				 "price": "150.25", "amount": "1502.50", "splitDivisor": "1", "adjustedPrice": "150.25"}
			],
			"closes": []
		}`)
	})
}

// 基準19（要件16）: 金額 99999989990000001（0.01 単位。ドル）は "999999899900000.01"、99999989900000010（円）は "999999899000000.1"
// （浮動小数点を通さないので、誤差が出ない）。
func Test基準19_最大の金額も誤差なく文字列にする(t *testing.T) {
	t.Parallel()

	t.Run("ドル", func(t *testing.T) {
		t.Parallel()
		h := dto.StockHistory{
			Code: "AAPL", Name: "Apple", Currency: dto.CurrencyUSD,
			Entries: []dto.StockHistoryEntry{
				{Kind: dto.StockHistoryEntryTrade, Date: "2026-01-10", Side: dto.TradeSideBuy, Quantity: 9999999,
					PriceHundredths: 9999999999, AmountHundredths: 99999989990000001, SplitDivisor: histDivisor(1), AdjustedPriceHundredths: 9999999999},
			},
			Closes: []dto.StockHistoryClose{},
		}

		body := marshalStockHistory(t, h)

		assertHistoryJSON(t, body, `{
			"stock": {"code": "AAPL", "name": "Apple", "currency": "USD"},
			"entries": [
				{"kind": "trade", "date": "2026-01-10", "side": "buy", "quantity": 9999999,
				 "price": "99999999.99", "amount": "999999899900000.01", "splitDivisor": "1", "adjustedPrice": "99999999.99"}
			],
			"closes": []
		}`)
	})

	t.Run("円", func(t *testing.T) {
		t.Parallel()
		h := dto.StockHistory{
			Code: "7203", Name: "トヨタ自動車", Currency: dto.CurrencyJPY,
			Entries: []dto.StockHistoryEntry{
				{Kind: dto.StockHistoryEntryTrade, Date: "2026-01-10", Side: dto.TradeSideBuy, Quantity: 9999999,
					PriceHundredths: 9999999990, AmountHundredths: 99999989900000010, SplitDivisor: histDivisor(1), AdjustedPriceHundredths: 9999999990},
			},
			Closes: []dto.StockHistoryClose{},
		}

		body := marshalStockHistory(t, h)

		assertHistoryJSON(t, body, `{
			"stock": {"code": "7203", "name": "トヨタ自動車", "currency": "JPY"},
			"entries": [
				{"kind": "trade", "date": "2026-01-10", "side": "buy", "quantity": 9999999,
				 "price": "99999999.9", "amount": "999999899000000.1", "splitDivisor": "1", "adjustedPrice": "99999999.9"}
			],
			"closes": []
		}`)
	})
}

// 補助（4.4。基準14 の API の土台）: 終値が0件なら "closes": []（null にしない）。Closes が空のスライスでも nil でも [] にする。
func Test基準14_終値が0件ならclosesはnullでなく空の配列(t *testing.T) {
	t.Parallel()
	entries := []dto.StockHistoryEntry{
		{Kind: dto.StockHistoryEntryTrade, Date: "2026-01-10", Side: dto.TradeSideSell, Quantity: 50,
			PriceHundredths: 700000, AmountHundredths: 35000000, SplitDivisor: histDivisor(1), AdjustedPriceHundredths: 700000},
	}
	want := `{
		"stock": {"code": "9984", "name": "ソフトバンクグループ", "currency": "JPY"},
		"entries": [
			{"kind": "trade", "date": "2026-01-10", "side": "sell", "quantity": 50,
			 "price": "7000.0", "amount": "350000.0", "splitDivisor": "1", "adjustedPrice": "7000.0"}
		],
		"closes": []
	}`

	for name, closes := range map[string][]dto.StockHistoryClose{
		"空のスライス": {},
		"nil":    nil,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := dto.StockHistory{Code: "9984", Name: "ソフトバンクグループ", Currency: dto.CurrencyJPY, Entries: entries, Closes: closes}

			body := marshalStockHistory(t, h)

			assertHistoryJSON(t, body, want)
		})
	}
}

// 補助（4.8。今の newHoldingsResponse と同じ）: 金額の書式にできないとき（円の価格が 0.1円の単位でない、知らない通貨）は、黙って切り捨てずに error。
func Test補助_金額の書式にできないときはエラーを返す(t *testing.T) {
	t.Parallel()
	trade := func(cur dto.Currency, price, amount, adjusted int64) dto.StockHistory {
		return dto.StockHistory{
			Code: "7203", Name: "トヨタ自動車", Currency: cur,
			Entries: []dto.StockHistoryEntry{
				{Kind: dto.StockHistoryEntryTrade, Date: "2026-01-10", Side: dto.TradeSideBuy, Quantity: 1,
					PriceHundredths: price, AmountHundredths: amount, SplitDivisor: histDivisor(1), AdjustedPriceHundredths: adjusted},
			},
			Closes: []dto.StockHistoryClose{},
		}
	}
	cases := []struct {
		name string
		h    dto.StockHistory
	}{
		{"円の価格が0.05円", trade(dto.CurrencyJPY, 5, 10, 10)},
		{"円の金額が0.1円の単位でない", trade(dto.CurrencyJPY, 250000, 5, 250000)},
		{"円の割った価格が0.1円の単位でない", trade(dto.CurrencyJPY, 250000, 250000, 5)},
		{"知らない通貨", trade(dto.Currency("EUR"), 250000, 250000, 250000)},
		{"円の終値が0.1円の単位でない", func() dto.StockHistory {
			h := trade(dto.CurrencyJPY, 250000, 250000, 250000)
			h.Closes = []dto.StockHistoryClose{
				{Date: "2026-01-10", PriceHundredths: 300025, SplitDivisor: histDivisor(1), AdjustedPriceHundredths: 300020},
			}
			return h
		}()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := newStockHistoryResponse(tc.h); err == nil {
				t.Errorf("newStockHistoryResponse: got nil error, want error")
			}
		})
	}
}
