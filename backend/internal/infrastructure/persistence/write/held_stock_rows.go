package write

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/sqlcgen"
)

// 保有している銘柄の判定（プラン 4.3）に使う、DB の行から domain の値への変換と、1人の利用者の保有の読み出し。
// 判定そのものは domain の HeldStockCodes で行い、SQL は売買履歴と分割を読むだけにする。

// dateString は、DATE の値を "YYYY-MM-DD" にする。
// pgx は DATE を UTC の0時の time.Time で返すので、そのまま書式にすれば、サーバーのタイムゾーンで日付がずれない。
func dateString(d pgtype.Date) (string, error) {
	if !d.Valid || d.InfinityModifier != pgtype.Finite {
		return "", fmt.Errorf("日付 %v は有限の日付ではありません", d)
	}
	return d.Time.Format(time.DateOnly), nil
}

// toTrade は、売買履歴の行から、保有の判定に使う model.Trade を作る（価格は判定に使わないので持たない）。
func toTrade(code, side string, quantity int64, tradeDate pgtype.Date) (model.Trade, error) {
	date, err := dateString(tradeDate)
	if err != nil {
		return model.Trade{}, fmt.Errorf("売買履歴（銘柄 %q）の取引日: %w", code, err)
	}
	return model.Trade{StockCode: code, Side: model.TradeSide(side), Quantity: quantity, TradeDate: date}, nil
}

// toStockSplit は、分割の行から model.StockSplit を作る。
func toStockSplit(code string, splitDate pgtype.Date, ratio int32) (model.StockSplit, error) {
	date, err := dateString(splitDate)
	if err != nil {
		return model.StockSplit{}, fmt.Errorf("分割（銘柄 %q）の分割の日: %w", code, err)
	}
	return model.StockSplit{StockCode: code, SplitDate: date, Ratio: int64(ratio)}, nil
}

// heldOfUser は、利用者 owner の売買履歴と、その銘柄の分割を読み、保有数量が1以上の銘柄コード（昇順）と、売買履歴にある銘柄の通貨を返す。
func heldOfUser(ctx context.Context, q *sqlcgen.Queries, owner pgtype.UUID) (heldCodes []string, currencies map[string]model.Currency, err error) {
	tradeRows, err := q.ListTradesOfUser(ctx, owner)
	if err != nil {
		return nil, nil, fmt.Errorf("売買履歴の問い合わせに失敗しました: %w", err)
	}
	trades := make([]model.Trade, 0, len(tradeRows))
	currencies = make(map[string]model.Currency)
	for _, row := range tradeRows {
		t, err := toTrade(row.StockCode, row.Side, row.Quantity, row.TradeDate)
		if err != nil {
			return nil, nil, err
		}
		trades = append(trades, t)
		currencies[row.StockCode] = model.Currency(row.Currency)
	}
	splitRows, err := q.ListStockSplitsOfUser(ctx, owner)
	if err != nil {
		return nil, nil, fmt.Errorf("分割の問い合わせに失敗しました: %w", err)
	}
	splits := make([]model.StockSplit, 0, len(splitRows))
	for _, row := range splitRows {
		s, err := toStockSplit(row.StockCode, row.SplitDate, row.Ratio)
		if err != nil {
			return nil, nil, err
		}
		splits = append(splits, s)
	}
	return model.HeldStockCodes(trades, splits), currencies, nil
}
