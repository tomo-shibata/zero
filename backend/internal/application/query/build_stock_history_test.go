package query_test

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/application/query"
	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// 仕様: 銘柄ごとの売買履歴の画面の要件（docs/requirements/stock-history.md）の基準11・12・13・17・18・19・20（要件9〜11・要件15〜18）と、
// プラン（docs/plans/stock-history/plan.md）4.3（組み立て）・4.8 の PR①（query.BuildStockHistory・dto.StockHistory）、
// 5.1 の PR① の行と 5.2「build_stock_history_test.go」の行。
// 金額は 0.01 単位の整数（2,500円は 250000、833.3円は 83330、15.00ドルは 1500）。期待値は要件・プランに書かれた値をそのまま使い、テストの中で計算しない。
// 割る数（SplitDivisor）は *big.Int なので、文字列にして比べる。

// histRow は、銘柄 code・name・通貨 cur の売買履歴1件（id は登録した順）を作る。
func histRow(code, name string, cur dto.Currency, id int64, side dto.TradeSide, quantity, priceHundredths int64, date string) dto.TradeRow {
	return dto.TradeRow{
		ID: id, Code: code, Name: name, Currency: cur, Side: side,
		Quantity: quantity, PriceHundredths: priceHundredths, TradeDate: date,
	}
}

// histToyota は、7203 トヨタ自動車（円）の売買履歴1件。
func histToyota(id int64, side dto.TradeSide, quantity, priceHundredths int64, date string) dto.TradeRow {
	return histRow("7203", "トヨタ自動車", dto.CurrencyJPY, id, side, quantity, priceHundredths, date)
}

// histApple は、AAPL Apple（ドル）の売買履歴1件。
func histApple(id int64, side dto.TradeSide, quantity, priceHundredths int64, date string) dto.TradeRow {
	return histRow("AAPL", "Apple", dto.CurrencyUSD, id, side, quantity, priceHundredths, date)
}

func histSplit(code, date string, ratio int64) dto.StockSplitRow {
	return dto.StockSplitRow{Code: code, SplitDate: date, Ratio: ratio}
}

func histClose(date string, priceHundredths int64) dto.ClosingPriceRow {
	return dto.ClosingPriceRow{TradingDate: date, ClosePriceHundredths: priceHundredths}
}

// buildHistory は BuildStockHistory を呼び、エラーならテストを止める。
func buildHistory(t *testing.T, trades []dto.TradeRow, splits []dto.StockSplitRow, closes []dto.ClosingPriceRow) dto.StockHistory {
	t.Helper()
	got, err := query.BuildStockHistory(trades, splits, closes)
	if err != nil {
		t.Fatalf("BuildStockHistory: got err %v, want nil", err)
	}
	return got
}

// entrySummary は、表の1行を「<日付> trade <buy か sell> <株数>」か「<日付> split 1→<分割後の株数>」の文字列にする（並びを比べるため）。
func entrySummary(e dto.StockHistoryEntry) string {
	switch e.Kind {
	case dto.StockHistoryEntryTrade:
		return fmt.Sprintf("%s trade %s %d", e.Date, e.Side, e.Quantity)
	case dto.StockHistoryEntrySplit:
		return fmt.Sprintf("%s split 1→%d", e.Date, e.Ratio)
	default:
		return fmt.Sprintf("%s ?%s", e.Date, e.Kind)
	}
}

func entrySummaries(entries []dto.StockHistoryEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, entrySummary(e))
	}
	return out
}

func assertEntryOrder(t *testing.T, got dto.StockHistory, want []string) {
	t.Helper()
	if gotOrder := entrySummaries(got.Entries); !reflect.DeepEqual(gotOrder, want) {
		t.Errorf("Entries の並び:\n got %q\nwant %q", gotOrder, want)
	}
}

// assertTradeEntry は、entries[i] が売買の行で、割る数（文字列）と割った価格（0.01 単位）が want のとおりであることを確かめる。
func assertTradeEntry(t *testing.T, entries []dto.StockHistoryEntry, i int, wantDivisor string, wantAdjusted int64) {
	t.Helper()
	if i >= len(entries) {
		t.Fatalf("Entries[%d] がない（%d 行）", i, len(entries))
	}
	e := entries[i]
	if e.Kind != dto.StockHistoryEntryTrade {
		t.Fatalf("Entries[%d] は売買の行のはずが %q", i, e.Kind)
	}
	if got := divisorString(e.SplitDivisor); got != wantDivisor {
		t.Errorf("Entries[%d]（%s）の SplitDivisor: got %s, want %s", i, entrySummary(e), got, wantDivisor)
	}
	if e.AdjustedPriceHundredths != wantAdjusted {
		t.Errorf("Entries[%d]（%s）の AdjustedPriceHundredths: got %d, want %d", i, entrySummary(e), e.AdjustedPriceHundredths, wantAdjusted)
	}
}

// assertClose は、closes[i] の日付・割る数（文字列）・割った価格（0.01 単位）が want のとおりであることを確かめる。
func assertClose(t *testing.T, closes []dto.StockHistoryClose, i int, wantDate, wantDivisor string, wantAdjusted int64) {
	t.Helper()
	if i >= len(closes) {
		t.Fatalf("Closes[%d] がない（%d 件）", i, len(closes))
	}
	c := closes[i]
	if c.Date != wantDate {
		t.Errorf("Closes[%d] の Date: got %s, want %s", i, c.Date, wantDate)
	}
	if got := divisorString(c.SplitDivisor); got != wantDivisor {
		t.Errorf("Closes[%d]（%s）の SplitDivisor: got %s, want %s", i, c.Date, got, wantDivisor)
	}
	if c.AdjustedPriceHundredths != wantAdjusted {
		t.Errorf("Closes[%d]（%s）の AdjustedPriceHundredths: got %d, want %d", i, c.Date, c.AdjustedPriceHundredths, wantAdjusted)
	}
}

func divisorString(d *big.Int) string {
	if d == nil {
		return "<nil>"
	}
	return d.String()
}

// 銘柄の情報（コード・名前・通貨）は、売買履歴の行から取る（4.4 の stock）。
func Test補助_銘柄コードと銘柄名と通貨は売買履歴の行のとおり(t *testing.T) {
	t.Parallel()

	got := buildHistory(t,
		[]dto.TradeRow{histApple(1, dto.TradeSideBuy, 10, 15025, "2026-01-10")}, nil, nil)

	if got.Code != "AAPL" || got.Name != "Apple" || got.Currency != dto.CurrencyUSD {
		t.Errorf("Code・Name・Currency: got %q・%q・%q, want AAPL・Apple・USD", got.Code, got.Name, got.Currency)
	}
}

// 基準11（要件11・要件18）: 同じ日の2件は、後に登録したものが先（id の降順）。
// Aの 7203 の売買履歴が、この順に登録した「買 2,500円 100株 2026-01-10」「買 2,520円 10株 2026-01-10」
// → 2026-01-10 の 10株（id2）が 100株（id1）より先。
func Test基準11_同じ日の売買は後に登録したものが先(t *testing.T) {
	t.Parallel()
	trades := []dto.TradeRow{
		histToyota(1, dto.TradeSideBuy, 100, 250000, "2026-01-10"),
		histToyota(2, dto.TradeSideBuy, 10, 252000, "2026-01-10"),
	}

	got := buildHistory(t, trades, nil, nil)

	assertEntryOrder(t, got, []string{"2026-01-10 trade buy 10", "2026-01-10 trade buy 100"})
}

// 基準17（要件18）: Aの 7203 の売買履歴を、登録した順に「買 2,500円 100株 2026-01-10」（id1）「買 2,520円 10株 2026-01-10」（id2）
// 「売 3,050円 50株 2026-03-02」（id3）とする。trades は [売 id3, 100株 id1, 10株 id2] と順を崩して渡す
// （受け取った順を逆にするだけの誤りを見分けるため）。
// → Entries は上から 2026-03-02 の売、2026-01-10 の10株の買、2026-01-10 の100株の買。
func Test基準17_表の行は新しい順で同じ日は後に登録したものが先(t *testing.T) {
	t.Parallel()
	trades := []dto.TradeRow{
		histToyota(3, dto.TradeSideSell, 50, 305000, "2026-03-02"),
		histToyota(1, dto.TradeSideBuy, 100, 250000, "2026-01-10"),
		histToyota(2, dto.TradeSideBuy, 10, 252000, "2026-01-10"),
	}

	got := buildHistory(t, trades, nil, nil)

	assertEntryOrder(t, got, []string{
		"2026-03-02 trade sell 50",
		"2026-01-10 trade buy 10",
		"2026-01-10 trade buy 100",
	})
}

// 補助（5.2。要件18）: 登録した順が日付の順と違う。id1「売 3,050円 50株 2026-03-02」、id2「買 2,500円 100株 2026-01-10」、
// id3「買 2,520円 10株 2026-01-10」→ 並びは 2026-03-02 の売、id3（10株）、id2（100株）。id の降順だけで並べる誤りを見分ける。
func Test補助_要件18_登録した順が日付の順と違っても日付の新しい順で同じ日は後の登録が先(t *testing.T) {
	t.Parallel()
	trades := []dto.TradeRow{
		histToyota(1, dto.TradeSideSell, 50, 305000, "2026-03-02"),
		histToyota(2, dto.TradeSideBuy, 100, 250000, "2026-01-10"),
		histToyota(3, dto.TradeSideBuy, 10, 252000, "2026-01-10"),
	}

	got := buildHistory(t, trades, nil, nil)

	assertEntryOrder(t, got, []string{
		"2026-03-02 trade sell 50",
		"2026-01-10 trade buy 10",
		"2026-01-10 trade buy 100",
	})
}

// 基準12（要件9〜11）: 7203 の分割が「1→3 2026-03-31」、Aの売買履歴が「買 2,500円 100株 2026-01-10」「買 1,000円 30株 2026-03-31」、
// 終値が 2026-03-30 3,100円・2026-03-31 1,050円。
// → 2026-03-30 の終値は割る数3・割った価格 103330（1,033.3円）、2026-03-31 の終値は割る数1・105000。
// 2026-01-10 の売買は割る数3・83330（833.3円）、2026-03-31 の売買は割る数1（分割の日と同じ日は割らない）・100000。
// 株数と価格は割らずにそのまま（要件15）。終値は取引日の昇順のまま。
func Test基準12_分割の前の終値と売買は割る数で割り分割の日と同じ日は割らない(t *testing.T) {
	t.Parallel()
	trades := []dto.TradeRow{
		histToyota(1, dto.TradeSideBuy, 100, 250000, "2026-01-10"),
		histToyota(2, dto.TradeSideBuy, 30, 100000, "2026-03-31"),
	}
	splits := []dto.StockSplitRow{histSplit("7203", "2026-03-31", 3)}
	closes := []dto.ClosingPriceRow{histClose("2026-03-30", 310000), histClose("2026-03-31", 105000)}

	got := buildHistory(t, trades, splits, closes)

	assertEntryOrder(t, got, []string{"2026-03-31 trade buy 30", "2026-03-31 split 1→3", "2026-01-10 trade buy 100"})
	assertTradeEntry(t, got.Entries, 0, "1", 100000)
	assertTradeEntry(t, got.Entries, 2, "3", 83330)
	if len(got.Closes) != 2 {
		t.Fatalf("Closes の件数: got %d, want 2", len(got.Closes))
	}
	assertClose(t, got.Closes, 0, "2026-03-30", "3", 103330)
	assertClose(t, got.Closes, 1, "2026-03-31", "1", 105000)
	// 価格は割らずにそのまま（要件15）。終値の価格もそのまま。
	if p := got.Entries[2].PriceHundredths; p != 250000 {
		t.Errorf("2026-01-10 の売買の PriceHundredths: got %d, want 250000（割らない）", p)
	}
	if p := got.Closes[0].PriceHundredths; p != 310000 {
		t.Errorf("2026-03-30 の終値の PriceHundredths: got %d, want 310000（割らない）", p)
	}
	// 分割の行は日付と分割後の株数（要件17）。
	if s := got.Entries[1]; s.Kind != dto.StockHistoryEntrySplit || s.Date != "2026-03-31" || s.Ratio != 3 {
		t.Errorf("分割の行: got %+v, want 分割・2026-03-31・ratio 3", s)
	}
}

// 基準13（要件9）: AAPL（ドル）の分割が「1→2 2026-02-01」「1→5 2026-04-01」、Aの売買履歴が「買 150.00ドル 10株 2026-01-10」「買 40.00ドル 10株 2026-03-01」
// → 2026-01-10 の割る数は10（2×5）で 1500（15.00ドル）、2026-03-01 の割る数は5で 800（8.00ドル）。
func Test基準13_分割が2つあるときは後の分割の分割後の株数をすべて掛けた数で割る(t *testing.T) {
	t.Parallel()
	trades := []dto.TradeRow{
		histApple(1, dto.TradeSideBuy, 10, 15000, "2026-01-10"),
		histApple(2, dto.TradeSideBuy, 10, 4000, "2026-03-01"),
	}
	splits := []dto.StockSplitRow{histSplit("AAPL", "2026-02-01", 2), histSplit("AAPL", "2026-04-01", 5)}

	got := buildHistory(t, trades, splits, nil)

	assertEntryOrder(t, got, []string{
		"2026-04-01 split 1→5",
		"2026-03-01 trade buy 10",
		"2026-02-01 split 1→2",
		"2026-01-10 trade buy 10",
	})
	assertTradeEntry(t, got.Entries, 1, "5", 800)
	assertTradeEntry(t, got.Entries, 3, "10", 1500)
}

// 補助（4.3 の3。BuildStockHistory は splits の並びに頼らない）: 基準13 と同じ分割を新しい順（{2026-04-01, 1→5}, {2026-02-01, 1→2}）で渡しても、
// 売買の割る数は 2026-01-10 が10・2026-03-01 が5。終値も、2つの分割の間の日で割る数が変わる
// （2026-01-10 は10、2026-03-01 は5、2つの分割より後の 2026-05-01 は1）。終値の側で分割を2つ掛ける確認も兼ねる。
func Test補助_分割が日付の順に並んでいない入力でも割る数は同じ(t *testing.T) {
	t.Parallel()
	trades := []dto.TradeRow{
		histApple(1, dto.TradeSideBuy, 10, 15000, "2026-01-10"),
		histApple(2, dto.TradeSideBuy, 10, 4000, "2026-03-01"),
	}
	splits := []dto.StockSplitRow{histSplit("AAPL", "2026-04-01", 5), histSplit("AAPL", "2026-02-01", 2)}
	closes := []dto.ClosingPriceRow{
		histClose("2026-01-10", 15000),
		histClose("2026-03-01", 4000),
		histClose("2026-05-01", 20000),
	}

	got := buildHistory(t, trades, splits, closes)

	assertEntryOrder(t, got, []string{
		"2026-04-01 split 1→5",
		"2026-03-01 trade buy 10",
		"2026-02-01 split 1→2",
		"2026-01-10 trade buy 10",
	})
	assertTradeEntry(t, got.Entries, 1, "5", 800)
	assertTradeEntry(t, got.Entries, 3, "10", 1500)
	if len(got.Closes) != 3 {
		t.Fatalf("Closes の件数: got %d, want 3", len(got.Closes))
	}
	assertClose(t, got.Closes, 0, "2026-01-10", "10", 1500)
	assertClose(t, got.Closes, 1, "2026-03-01", "5", 800)
	assertClose(t, got.Closes, 2, "2026-05-01", "1", 20000)
}

// 基準18（要件15・要件16）: 売買「買 1,234.5円 3株」「買 2,500円 100株」「買 150.25ドル 10株」
// → 金額は 370350・25000000・150250（0.01 単位。株数 × 価格）。
func Test基準18_金額は株数と価格の積を丸めずに出す(t *testing.T) {
	t.Parallel()

	t.Run("円の2件", func(t *testing.T) {
		t.Parallel()
		trades := []dto.TradeRow{
			histToyota(1, dto.TradeSideBuy, 3, 123450, "2026-01-10"),
			histToyota(2, dto.TradeSideBuy, 100, 250000, "2026-02-10"),
		}

		got := buildHistory(t, trades, nil, nil)

		// 新しい順: 2026-02-10 の 100株、2026-01-10 の 3株。
		assertEntryOrder(t, got, []string{"2026-02-10 trade buy 100", "2026-01-10 trade buy 3"})
		if a := amountOf(t, got, 0); a != 25000000 {
			t.Errorf("2,500円×100株の AmountHundredths: got %d, want 25000000", a)
		}
		if a := amountOf(t, got, 1); a != 370350 {
			t.Errorf("1,234.5円×3株の AmountHundredths: got %d, want 370350", a)
		}
	})

	t.Run("ドル", func(t *testing.T) {
		t.Parallel()
		trades := []dto.TradeRow{histApple(1, dto.TradeSideBuy, 10, 15025, "2026-01-10")}

		got := buildHistory(t, trades, nil, nil)

		if a := amountOf(t, got, 0); a != 150250 {
			t.Errorf("150.25ドル×10株の AmountHundredths: got %d, want 150250", a)
		}
	})
}

// 基準19（要件16）: 売買「買 99,999,999.99ドル 9,999,999株」「買 99,999,999.9円 9,999,999株」
// → 金額は 99999989990000001（0.01 単位。ドル）と 99999989900000010（0.01 単位。円。999,999,899,000,000.1円）。どちらも誤差なし。
func Test基準19_最大の株数と価格でも金額に誤差を出さない(t *testing.T) {
	t.Parallel()

	t.Run("ドル", func(t *testing.T) {
		t.Parallel()
		trades := []dto.TradeRow{histApple(1, dto.TradeSideBuy, 9999999, 9999999999, "2026-01-10")}

		got := buildHistory(t, trades, nil, nil)

		if a := amountOf(t, got, 0); a != 99999989990000001 {
			t.Errorf("AmountHundredths: got %d, want 99999989990000001", a)
		}
	})

	t.Run("円", func(t *testing.T) {
		t.Parallel()
		trades := []dto.TradeRow{histToyota(1, dto.TradeSideBuy, 9999999, 9999999990, "2026-01-10")}

		got := buildHistory(t, trades, nil, nil)

		if a := amountOf(t, got, 0); a != 99999989900000010 {
			t.Errorf("AmountHundredths: got %d, want 99999989900000010", a)
		}
	})
}

// 基準20（要件15・要件17・要件18）: 7203 の分割が「1→5 2021-09-29」「1→3 2026-03-31」、Aの売買履歴が「買 2,500円 100株 2026-01-10」「買 1,000円 30株 2026-03-31」
// → Entries は上から 2026-03-31 の買（30株）、2026-03-31 の分割（1→3。同じ日の売買より後）、2026-01-10 の買（100株）の3つで、2021-09-29 の分割は出ない。
// 最初の売買より前の分割は割る数にも入らない（2026-01-10 の割る数は 3）。
func Test基準20_分割の行は最初の売買の日より後のものだけを同じ日の売買の後に出す(t *testing.T) {
	t.Parallel()
	trades := []dto.TradeRow{
		histToyota(1, dto.TradeSideBuy, 100, 250000, "2026-01-10"),
		histToyota(2, dto.TradeSideBuy, 30, 100000, "2026-03-31"),
	}
	splits := []dto.StockSplitRow{histSplit("7203", "2021-09-29", 5), histSplit("7203", "2026-03-31", 3)}

	got := buildHistory(t, trades, splits, nil)

	assertEntryOrder(t, got, []string{"2026-03-31 trade buy 30", "2026-03-31 split 1→3", "2026-01-10 trade buy 100"})
	assertTradeEntry(t, got.Entries, 2, "3", 83330)
}

// 補助（5.2。要件17 の境目）: 分割「1→2 2026-01-10」「1→3 2026-03-31」と「買 2,500円 100株 2026-01-10」で、
// 分割の行は 2026-03-31 の1つだけ（最初の売買の日と同じ日の分割は出さない）。
// 2026-01-10 の売買の割る数は3（同じ日の分割は、その日の売買の「後」の分割ではないので掛けない。要件9）。
func Test補助_要件17_最初の売買の日と同じ日の分割は出さず割る数にも入れない(t *testing.T) {
	t.Parallel()
	trades := []dto.TradeRow{histToyota(1, dto.TradeSideBuy, 100, 250000, "2026-01-10")}
	splits := []dto.StockSplitRow{histSplit("7203", "2026-01-10", 2), histSplit("7203", "2026-03-31", 3)}

	got := buildHistory(t, trades, splits, nil)

	assertEntryOrder(t, got, []string{"2026-03-31 split 1→3", "2026-01-10 trade buy 100"})
	assertTradeEntry(t, got.Entries, 1, "3", 83330)
}

// 補助（5.2。要件9 の境目）: 分割「1→2 2026-03-31」。分割の日の前の日（2026-03-30）の終値と売買は割る（割る数2）、
// 分割の日と同じ日（2026-03-31）の終値と売買は割らない（割る数1）。
func Test補助_要件9_分割の日と同じ日の終値と売買は割らず前の日は割る(t *testing.T) {
	t.Parallel()
	trades := []dto.TradeRow{
		histToyota(1, dto.TradeSideBuy, 1, 100000, "2026-01-10"),
		histToyota(2, dto.TradeSideBuy, 1, 100000, "2026-03-30"),
		histToyota(3, dto.TradeSideBuy, 1, 100000, "2026-03-31"),
	}
	splits := []dto.StockSplitRow{histSplit("7203", "2026-03-31", 2)}
	closes := []dto.ClosingPriceRow{histClose("2026-03-30", 200000), histClose("2026-03-31", 200000)}

	got := buildHistory(t, trades, splits, closes)

	assertEntryOrder(t, got, []string{
		"2026-03-31 trade buy 1",
		"2026-03-31 split 1→2",
		"2026-03-30 trade buy 1",
		"2026-01-10 trade buy 1",
	})
	assertTradeEntry(t, got.Entries, 0, "1", 100000)
	assertTradeEntry(t, got.Entries, 2, "2", 50000)
	assertTradeEntry(t, got.Entries, 3, "2", 50000)
	assertClose(t, got.Closes, 0, "2026-03-30", "2", 100000)
	assertClose(t, got.Closes, 1, "2026-03-31", "1", 200000)
}

// 補助（5.2。4.3 の3。math/big）: 割る数が int64 を超える分割（1→1000 を7回。10 の21乗）でも、割る数と割った価格が正しい。
// 7回の分割より前の売買と終値は、割る数が 10 の21乗（"1" と 0 が21個）で、割った価格は 0（0.01 単位で四捨五入すると 0）。
// 7回の分割より後（2026-03-01）の売買は割る数1。
func Test補助_割る数がint64を超える分割でも割る数と割った価格が正しい(t *testing.T) {
	t.Parallel()
	trades := []dto.TradeRow{
		histToyota(1, dto.TradeSideBuy, 100, 250000, "2026-01-10"),
		histToyota(2, dto.TradeSideBuy, 1, 100000, "2026-03-01"),
	}
	splits := make([]dto.StockSplitRow, 0, 7)
	for day := 1; day <= 7; day++ {
		splits = append(splits, histSplit("7203", fmt.Sprintf("2026-02-%02d", day), 1000))
	}
	closes := []dto.ClosingPriceRow{histClose("2026-01-10", 245000)}

	got := buildHistory(t, trades, splits, closes)

	wantDivisor := "1" + strings.Repeat("0", 21)
	// 新しい順: 2026-03-01 の売買が先頭、最後（末尾）が 2026-01-10 の売買。
	assertTradeEntry(t, got.Entries, 0, "1", 100000)
	assertTradeEntry(t, got.Entries, len(got.Entries)-1, wantDivisor, 0)
	assertClose(t, got.Closes, 0, "2026-01-10", wantDivisor, 0)
}

// 補助（5.2。4.3 の4）: 割った価格の四捨五入の境目。円は 0.1円（0.01 単位で10）、ドルは 0.01ドル（0.01 単位で1）の単位に、
// 半分は切り上げ（0.05円→0.1円、0.005ドル→0.01ドル）、半分に満たなければ切り捨てる。
// 売買（2026-01-10）の後に分割「1→200 2026-02-01」を置き、割る数200で割る。
func Test補助_割った価格は円は0_1円ドルは0_01ドルの単位に四捨五入する(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		currency dto.Currency
		price    int64 // 0.01 単位
		want     int64 // 0.01 単位
	}{
		{"円 10円÷200=0.05円は0.1円に切り上がる", dto.CurrencyJPY, 1000, 10},
		{"円 9.8円÷200=0.049円は0に切り下がる", dto.CurrencyJPY, 980, 0},
		{"円 30円÷200=0.15円は0.2円", dto.CurrencyJPY, 3000, 20},
		{"円 50円÷200=0.25円は0.3円（偶数に丸めない）", dto.CurrencyJPY, 5000, 30},
		{"ドル 1.00ドル÷200=0.005ドルは0.01ドルに切り上がる", dto.CurrencyUSD, 100, 1},
		{"ドル 0.98ドル÷200=0.0049ドルは0に切り下がる", dto.CurrencyUSD, 98, 0},
		{"ドル 3.00ドル÷200=0.015ドルは0.02ドル", dto.CurrencyUSD, 300, 2},
		{"ドル 5.00ドル÷200=0.025ドルは0.03ドル（偶数に丸めない）", dto.CurrencyUSD, 500, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			code, name := "7203", "トヨタ自動車"
			if tc.currency == dto.CurrencyUSD {
				code, name = "AAPL", "Apple"
			}
			trades := []dto.TradeRow{histRow(code, name, tc.currency, 1, dto.TradeSideBuy, 1, tc.price, "2026-01-10")}
			splits := []dto.StockSplitRow{histSplit(code, "2026-02-01", 200)}
			closes := []dto.ClosingPriceRow{histClose("2026-01-10", tc.price)}

			got := buildHistory(t, trades, splits, closes)

			// 新しい順: 分割の行、売買の行。
			assertTradeEntry(t, got.Entries, 1, "200", tc.want)
			assertClose(t, got.Closes, 0, "2026-01-10", "200", tc.want)
		})
	}
}

// 補助（5.2。4.3 の5）: 金額（株数 × 価格）が int64 に収まらないときは ErrCalculationOverflow（DB は価格の上限を確かめない）。
// 境目: 2株 × 価格 2^62-1 は int64 に収まる（2^63-2）。2株 × 価格 2^62 は 2^63 で収まらない。
func Test補助_金額があふれるときはErrCalculationOverflow(t *testing.T) {
	t.Parallel()

	t.Run("収まるぎりぎりはエラーにならない", func(t *testing.T) {
		t.Parallel()
		trades := []dto.TradeRow{histApple(1, dto.TradeSideBuy, 2, 1<<62-1, "2026-01-10")}

		got := buildHistory(t, trades, nil, nil)

		if a := amountOf(t, got, 0); a != math.MaxInt64-1 {
			t.Errorf("AmountHundredths: got %d, want %d", a, int64(math.MaxInt64-1))
		}
	})

	t.Run("2の63乗はあふれる", func(t *testing.T) {
		t.Parallel()
		trades := []dto.TradeRow{histApple(1, dto.TradeSideBuy, 2, 1<<62, "2026-01-10")}

		_, err := query.BuildStockHistory(trades, nil, nil)

		if !errors.Is(err, query.ErrCalculationOverflow) {
			t.Errorf("err: got %v, want ErrCalculationOverflow", err)
		}
	})

	t.Run("株数も価格も大きいとあふれる", func(t *testing.T) {
		t.Parallel()
		trades := []dto.TradeRow{histToyota(1, dto.TradeSideBuy, 1<<40, 1<<40, "2026-01-10")}

		_, err := query.BuildStockHistory(trades, nil, nil)

		if !errors.Is(err, query.ErrCalculationOverflow) {
			t.Errorf("err: got %v, want ErrCalculationOverflow", err)
		}
	})
}

// 基準14 の API の土台（4.8: Closes は 0件のとき空のスライス。nil にしない。API で [] にするため）。
func Test基準14_終値が0件のときClosesはnilでない空のスライス(t *testing.T) {
	t.Parallel()
	trades := []dto.TradeRow{
		histRow("9984", "ソフトバンクグループ", dto.CurrencyJPY, 1, dto.TradeSideBuy, 100, 600000, "2026-01-10"),
		histRow("9984", "ソフトバンクグループ", dto.CurrencyJPY, 2, dto.TradeSideSell, 50, 700000, "2026-02-10"),
	}

	for name, closes := range map[string][]dto.ClosingPriceRow{
		"nil を渡す":   nil,
		"空のスライスを渡す": {},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := buildHistory(t, trades, nil, closes)

			if got.Closes == nil {
				t.Errorf("Closes が nil（空のスライスのはず）")
			}
			if len(got.Closes) != 0 {
				t.Errorf("Closes の件数: got %d, want 0", len(got.Closes))
			}
			assertEntryOrder(t, got, []string{"2026-02-10 trade sell 50", "2026-01-10 trade buy 100"})
		})
	}
}

// amountOf は、h.Entries[i] の AmountHundredths を返す。行がなければテストを止める（添字の範囲外で panic させない）。
func amountOf(t *testing.T, h dto.StockHistory, i int) int64 {
	t.Helper()
	if i >= len(h.Entries) {
		t.Fatalf("Entries[%d] がない（%d 行）", i, len(h.Entries))
	}
	return h.Entries[i].AmountHundredths
}
