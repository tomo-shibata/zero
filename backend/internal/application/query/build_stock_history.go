package query

import (
	"cmp"
	"fmt"
	"math/big"
	"slices"
	"sort"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// BuildStockHistory は、1つの銘柄の売買履歴 trades・分割 splits・終値 closes から、売買履歴の表の行と終値の列を作る純粋関数
// （プラン 4.3）。trades は1件以上で、どれも同じ銘柄。splits も同じ銘柄のものだけを渡す。closes は取引日の昇順。
//
//  1. 表の行（Entries）は、最初の売買の取引日より後の分割と売買を、traceEvents の順（日付の昇順 → 同じ日は分割が先 → id の昇順）に並べ、
//     逆にしたもの（新しい順。要件17・要件18）。trades・splits の並びには頼らない。
//  2. 売買と終値の割る数は、その日より後（同じ日は含めない）の分割の、分割後の株数の積（要件9）。分割が多いと int64 を超えうるので math/big で数える。
//  3. 割った価格は、価格 ÷ 割る数を、円は 0.1円、ドルは 0.01ドルの単位に四捨五入した値（半分は切り上げ。要件10・要件11）。割る数が1なら価格のまま。
//  4. 金額は、株数 × 価格（要件16）。int64 に収まらなければ ErrCalculationOverflow を（包んで）返す。値そのものは資産の額なので、エラー（ログ）には入れない。
//  5. 終値の列（Closes）は、渡された順（取引日の昇順）のまま、各終値に割る数と割った価格を付ける。0件のときは空のスライス。
func BuildStockHistory(trades []dto.TradeRow, splits []dto.StockSplitRow, closes []dto.ClosingPriceRow) (dto.StockHistory, error) {
	if len(trades) == 0 {
		return dto.StockHistory{}, ErrStockHistoryNotFound
	}
	head := trades[0]
	priceUnit, _, err := roundingUnits(head.Currency)
	if err != nil {
		return dto.StockHistory{}, err
	}
	divisors := newSplitDivisors(splits)

	// 最初の売買の取引日と同じかそれより前の分割は、保有に響かないので表に出さない（要件17）。
	first := firstTradeDate(trades)
	shown := make([]dto.StockSplitRow, 0, len(splits))
	for _, s := range splits {
		if s.SplitDate > first {
			shown = append(shown, s)
		}
	}

	events := traceEvents(trades, shown)
	entries := make([]dto.StockHistoryEntry, 0, len(events))
	for _, e := range slices.Backward(events) {
		if !e.isTrade {
			entries = append(entries, dto.StockHistoryEntry{Kind: dto.StockHistoryEntrySplit, Date: e.date, Ratio: e.ratio})
			continue
		}
		entry, err := tradeEntry(e.trade, divisors.after(e.date), priceUnit)
		if err != nil {
			return dto.StockHistory{}, fmt.Errorf("売買（%s）: %w", e.date, err)
		}
		entries = append(entries, entry)
	}

	closeList := make([]dto.StockHistoryClose, 0, len(closes))
	for _, c := range closes {
		divisor := divisors.after(c.TradingDate)
		adjusted, err := adjustPrice(c.ClosePriceHundredths, divisor, priceUnit)
		if err != nil {
			return dto.StockHistory{}, fmt.Errorf("終値（%s）: %w", c.TradingDate, err)
		}
		closeList = append(closeList, dto.StockHistoryClose{
			Date: c.TradingDate, PriceHundredths: c.ClosePriceHundredths, SplitDivisor: divisor, AdjustedPriceHundredths: adjusted,
		})
	}

	return dto.StockHistory{
		Code: head.Code, Name: head.Name, Currency: head.Currency,
		Entries: entries, Closes: closeList,
	}, nil
}

// firstTradeDate は、売買履歴 trades（1件以上）の最も早い取引日を返す。
func firstTradeDate(trades []dto.TradeRow) string {
	first := trades[0].TradeDate
	for _, t := range trades[1:] {
		first = min(first, t.TradeDate)
	}
	return first
}

// tradeEntry は、売買1件 t を表の行にする。divisor は、その取引日に対する割る数。
func tradeEntry(t dto.TradeRow, divisor *big.Int, priceUnit int64) (dto.StockHistoryEntry, error) {
	amount := new(big.Int).Mul(big.NewInt(t.Quantity), big.NewInt(t.PriceHundredths))
	if !amount.IsInt64() {
		return dto.StockHistoryEntry{}, fmt.Errorf("金額: %w", ErrCalculationOverflow)
	}
	adjusted, err := adjustPrice(t.PriceHundredths, divisor, priceUnit)
	if err != nil {
		return dto.StockHistoryEntry{}, err
	}
	return dto.StockHistoryEntry{
		Kind: dto.StockHistoryEntryTrade, Date: t.TradeDate, Side: t.Side, Quantity: t.Quantity,
		PriceHundredths: t.PriceHundredths, AmountHundredths: amount.Int64(),
		SplitDivisor: divisor, AdjustedPriceHundredths: adjusted,
	}, nil
}

// adjustPrice は、価格 price（0.01 単位）を divisor で割り、unit（0.01 単位。円は10、ドルは1）の倍数に四捨五入した値を返す。
// divisor が1なら price のまま（丸めない）。
func adjustPrice(price int64, divisor *big.Int, unit int64) (int64, error) {
	if divisor.IsInt64() && divisor.Int64() == 1 {
		return price, nil
	}
	rounded := roundHalfUp(new(big.Rat).SetFrac(big.NewInt(price), divisor), unit)
	if !rounded.IsInt64() {
		return 0, fmt.Errorf("割った価格: %w", ErrCalculationOverflow)
	}
	return rounded.Int64(), nil
}

// splitDivisors は、日付から「その日より後の分割の、分割後の株数の積」を引くための表。
type splitDivisors struct {
	splits []dto.StockSplitRow // 分割の日の昇順
	suffix []*big.Int          // suffix[i] は splits[i:] の分割後の株数の積。最後の要素（splits の後）は 1
}

func newSplitDivisors(splits []dto.StockSplitRow) *splitDivisors {
	sorted := slices.Clone(splits)
	slices.SortFunc(sorted, func(a, b dto.StockSplitRow) int { return cmp.Compare(a.SplitDate, b.SplitDate) })
	suffix := make([]*big.Int, len(sorted)+1)
	suffix[len(sorted)] = big.NewInt(1)
	for i := len(sorted) - 1; i >= 0; i-- {
		suffix[i] = new(big.Int).Mul(suffix[i+1], big.NewInt(sorted[i].Ratio))
	}
	return &splitDivisors{splits: sorted, suffix: suffix}
}

// after は、日付 date（"YYYY-MM-DD"）より後の分割の、分割後の株数の積を返す（同じ日の分割は含めない。要件9）。分割がなければ1。
// 返す値は呼び出しごとの新しい値（受け取った側が書き換えても、ほかの行に響かない）。
func (d *splitDivisors) after(date string) *big.Int {
	i := sort.Search(len(d.splits), func(i int) bool { return d.splits[i].SplitDate > date })
	return new(big.Int).Set(d.suffix[i])
}
