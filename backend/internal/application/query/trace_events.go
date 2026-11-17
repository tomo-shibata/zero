package query

import (
	"cmp"
	"slices"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// traceEvent は、同じ銘柄をたどるときの1つの出来事（売買履歴1件か、分割1件）。
type traceEvent struct {
	date    string
	isTrade bool
	id      int64        // 売買履歴の id（分割では0）
	trade   dto.TradeRow // 売買履歴のとき
	ratio   int64        // 分割後の株数（分割のとき）
}

// traceEvents は、同じ銘柄の売買履歴 rows と分割 splits を、たどる順（日付の昇順 → 同じ日は分割が先 → id の昇順）に並べて返す
// （株式分割の要件6。rows と splits の並びには頼らない）。呼び出し側の rows・splits は並べ替えない。
// 分割は、rows と同じ銘柄のものだけを渡す。
func traceEvents(rows []dto.TradeRow, splits []dto.StockSplitRow) []traceEvent {
	events := make([]traceEvent, 0, len(rows)+len(splits))
	for _, r := range rows {
		events = append(events, traceEvent{date: r.TradeDate, isTrade: true, id: r.ID, trade: r})
	}
	for _, s := range splits {
		events = append(events, traceEvent{date: s.SplitDate, ratio: s.Ratio})
	}
	slices.SortFunc(events, func(a, b traceEvent) int {
		// 日付は "YYYY-MM-DD" なので、文字列の昇順が日付の古い順になる。同じ日は分割（isTrade が false）が先。
		return cmp.Or(cmp.Compare(a.date, b.date), compareIsTrade(a.isTrade, b.isTrade), cmp.Compare(a.id, b.id))
	})
	return events
}

// compareIsTrade は、分割（false）を売買履歴（true）より前にして比べる。
func compareIsTrade(a, b bool) int {
	switch {
	case a == b:
		return 0
	case !a:
		return -1
	default:
		return 1
	}
}

// latestSplitDate は、splits の中で最も新しい分割の日を返す。splits が空なら ok が false。
func latestSplitDate(splits []dto.StockSplitRow) (date string, ok bool) {
	for _, s := range splits {
		if !ok || s.SplitDate > date {
			date, ok = s.SplitDate, true
		}
	}
	return date, ok
}

// splitsByCode は、分割 splits を銘柄コードごとにまとめる。
func splitsByCode(splits []dto.StockSplitRow) map[string][]dto.StockSplitRow {
	byCode := make(map[string][]dto.StockSplitRow)
	for _, s := range splits {
		byCode[s.Code] = append(byCode[s.Code], s)
	}
	return byCode
}
