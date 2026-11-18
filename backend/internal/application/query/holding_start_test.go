package query_test

import (
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/application/query"
	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// 仕様: 価格の変動の通知の要件（docs/requirements/price-alerts.md）の要件12（今の保有が始まった日。売買履歴と分割を順にたどって、
// 保有数量が0から1以上になった最後の買いの取引日。売り切った後に買い直したら、買い直した日）と、
// プラン（docs/plans/price-alerts/plan.md）4.3 の手順3、4.8 の PR① の境界（query.HoldingStartDate(trades, splits) (date, ok)）、
// 5.2 の holding_start_test.go の行（1回買っただけ・買い増し・一部の売り・売り切り・売り切って買い直し・分割をはさむ・保有なし）。
// たどる順は「日付の昇順 → 同じ日は分割が先 → 売買履歴の id の昇順」（株式分割の要件6。引数の並びには頼らない）。

// hsTrade は、7203 の売買履歴1件（id は登録した順。価格は始まりの日に関係しないので 2,500円）。
func hsTrade(id int64, side dto.TradeSide, quantity int64, date string) dto.TradeRow {
	return dto.TradeRow{
		ID: id, Code: "7203", Name: "トヨタ自動車", Currency: dto.CurrencyJPY,
		Side: side, Quantity: quantity, PriceHundredths: 250000, TradeDate: date,
	}
}

func hsBuy(id, quantity int64, date string) dto.TradeRow {
	return hsTrade(id, dto.TradeSideBuy, quantity, date)
}
func hsSell(id, quantity int64, date string) dto.TradeRow {
	return hsTrade(id, dto.TradeSideSell, quantity, date)
}

func hsSplit(date string, ratio int64) dto.StockSplitRow {
	return dto.StockSplitRow{Code: "7203", SplitDate: date, Ratio: ratio}
}

// 要件12: 保有している銘柄の、今の保有が始まった日。
func Test要件12_HoldingStartDateは保有数量が0から1以上になった最後の買いの取引日を返す(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		trades []dto.TradeRow
		splits []dto.StockSplitRow
		want   string
	}{
		{
			name:   "1回買っただけなら、その買いの日",
			trades: []dto.TradeRow{hsBuy(1, 100, "2026-01-10")},
			want:   "2026-01-10",
		},
		{
			name:   "買い増しても、最初の買いの日",
			trades: []dto.TradeRow{hsBuy(1, 100, "2026-01-10"), hsBuy(2, 50, "2026-02-10")},
			want:   "2026-01-10",
		},
		{
			name:   "一部だけ売っても、最初の買いの日",
			trades: []dto.TradeRow{hsBuy(1, 100, "2026-01-10"), hsSell(2, 40, "2026-02-10")},
			want:   "2026-01-10",
		},
		{
			name:   "売って買い増しても、最初の買いの日（0にはならない）",
			trades: []dto.TradeRow{hsBuy(1, 100, "2026-01-10"), hsSell(2, 40, "2026-02-10"), hsBuy(3, 10, "2026-03-10")},
			want:   "2026-01-10",
		},
		{
			name: "売り切って買い直したら、買い直した日",
			trades: []dto.TradeRow{
				hsBuy(1, 100, "2026-01-10"), hsSell(2, 100, "2026-02-10"), hsBuy(3, 100, "2026-03-10"),
			},
			want: "2026-03-10",
		},
		{
			name: "売り切って買い直し、さらに買い増したら、買い直した日（買い増した日ではない）",
			trades: []dto.TradeRow{
				hsBuy(1, 100, "2026-01-10"), hsSell(2, 100, "2026-02-10"), hsBuy(3, 100, "2026-03-10"), hsBuy(4, 100, "2026-04-10"),
			},
			want: "2026-03-10",
		},
		{
			name: "2回売り切って買い直したら、最後の買い直した日",
			trades: []dto.TradeRow{
				hsBuy(1, 100, "2026-01-10"), hsSell(2, 100, "2026-02-10"),
				hsBuy(3, 100, "2026-03-10"), hsSell(4, 100, "2026-04-10"),
				hsBuy(5, 10, "2026-05-10"),
			},
			want: "2026-05-10",
		},
		{
			name:   "分割をはさんでも、最初の買いの日",
			trades: []dto.TradeRow{hsBuy(1, 100, "2026-01-10"), hsBuy(3, 100, "2026-03-10")},
			splits: []dto.StockSplitRow{hsSplit("2026-02-10", 2)},
			want:   "2026-01-10",
		},
		{
			name:   "分割で増えた株数を売り切って買い直したら、買い直した日（分割の後の株数で0になる）",
			trades: []dto.TradeRow{hsBuy(1, 100, "2026-01-10"), hsSell(2, 200, "2026-03-10"), hsBuy(3, 10, "2026-04-10")},
			splits: []dto.StockSplitRow{hsSplit("2026-02-10", 2)},
			want:   "2026-04-10",
		},
		{
			name: "同じ日の売りと買いは id の順にたどる（売り切って同じ日に買い直したら、その日）",
			trades: []dto.TradeRow{
				hsBuy(1, 100, "2026-01-10"), hsSell(2, 100, "2026-02-10"), hsBuy(3, 100, "2026-02-10"),
			},
			want: "2026-02-10",
		},
		{
			name: "引数の並びには頼らない（日付の逆順で渡しても同じ）",
			trades: []dto.TradeRow{
				hsBuy(3, 100, "2026-03-10"), hsSell(2, 100, "2026-02-10"), hsBuy(1, 100, "2026-01-10"),
			},
			want: "2026-03-10",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := query.HoldingStartDate(tc.trades, tc.splits)

			if !ok {
				t.Fatalf("HoldingStartDate: got ok=false, want true（保有数量は1以上）")
			}
			if got != tc.want {
				t.Errorf("HoldingStartDate: got %q, want %q", got, tc.want)
			}
		})
	}
}

// 要件12: 保有数量が0（売買履歴がない・売り切った）なら ok=false。
func Test要件12_HoldingStartDateは保有していなければokがfalse(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		trades []dto.TradeRow
		splits []dto.StockSplitRow
	}{
		{name: "売買履歴がない", trades: nil},
		{name: "売買履歴が空", trades: []dto.TradeRow{}},
		{name: "売買履歴がなく分割だけある", trades: nil, splits: []dto.StockSplitRow{hsSplit("2026-02-10", 2)}},
		{name: "買って売り切った", trades: []dto.TradeRow{hsBuy(1, 100, "2026-01-10"), hsSell(2, 100, "2026-02-10")}},
		{
			name:   "分割の日の売りは、分割の後の株数で売り切る（同じ日は分割が先）",
			trades: []dto.TradeRow{hsBuy(1, 100, "2026-01-10"), hsSell(2, 200, "2026-02-10")},
			splits: []dto.StockSplitRow{hsSplit("2026-02-10", 2)},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got, ok := query.HoldingStartDate(tc.trades, tc.splits); ok {
				t.Errorf("HoldingStartDate: got (%q, true), want ok=false", got)
			}
		})
	}
}
