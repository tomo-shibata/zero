package query_test

import (
	"errors"
	"reflect"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/application/query"
	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// 仕様: 価格の変動の通知の要件（docs/requirements/price-alerts.md）の要件9〜要件19・要件21と、
// プラン（docs/plans/price-alerts/plan.md）4.3（判定 query.EvaluateAlerts の手順1〜5と比べ方の表）、4.8 の PR① の境界
// （dto.AlertEvaluationInput・AlertEvaluationResult・AlertConditionRow・AlertJudgmentRow・AlertNotification・AlertNotificationDetails・
// AlertJudgment）、5.2 の evaluate_alerts_test.go の行。
// 金額は 0.01 単位の整数（2,500円は 250000、150.00ドルは 15000）。率は 0.1% 単位（-20.0% は -200）。値（threshold）は 0.01 単位（20% は 2000）。
// 期待値は要件・プランに書かれた値をそのまま使い、テストの中で計算しない。
// 判定の純粋関数なので DB は使わない。入力は 4.8 の AlertEvaluationInput のとおりに作る（終値の取引日 D は 2026-11-10 が基本）。

var (
	alUserA = uuid.MustParse("00000000-0000-0000-0000-00000000000a")
	alUserB = uuid.MustParse("00000000-0000-0000-0000-00000000000b")
	alUserC = uuid.MustParse("00000000-0000-0000-0000-00000000000c")
	alUserD = uuid.MustParse("00000000-0000-0000-0000-00000000000d")
)

// 条件の種類・対象・向きの文字列（4.8: model.AlertKind・AlertScope・AlertDirection と同じ値）。
const (
	alCost  = "cost"
	alPeak  = "peak"
	alPrev  = "prev"
	alPnl   = "pnl"
	alPrice = "price"

	alAll   = "all"
	alStock = "stock"

	alUp     = "up"
	alDown   = "down"
	alEither = "either"
	alLoss   = "loss"
	alGain   = "gain"
	alAbove  = "above"
	alBelow  = "below"

	alDate = "2026-11-10" // 判定する終値の取引日 D の基本
)

func alPtr[T any](v T) *T { return &v }

// alYen は、円の金額 n を 0.01 単位にする（2,500円 → 250000）。
func alYen(n int64) int64 { return n * 100 }

// alCond は、条件1件（足した日 addedOn は "YYYY-MM-DD"）を作る。
func alCond(id int64, user uuid.UUID, kind, scope, code, direction string, threshold int64, addedOn string) dto.AlertConditionRow {
	return dto.AlertConditionRow{
		ID: id, UserID: user, Kind: kind, Scope: scope, StockCode: code, Direction: direction,
		ThresholdHundredths: threshold, AddedOn: addedOn,
	}
}

// alAllCond は、全銘柄の条件（足した日は 2026-11-01）。
func alAllCond(id int64, user uuid.UUID, kind, direction string, threshold int64) dto.AlertConditionRow {
	return alCond(id, user, kind, alAll, "", direction, threshold, "2026-11-01")
}

// alStockCond は、銘柄 code の条件（足した日は 2026-11-01）。
func alStockCond(id int64, user uuid.UUID, kind, code, direction string, threshold int64) dto.AlertConditionRow {
	return alCond(id, user, kind, alStock, code, direction, threshold, "2026-11-01")
}

// alTrade は、銘柄 code の売買履歴1件（id は登録した順。銘柄名は判定に関係しない）。
func alTrade(code string, cur dto.Currency, id int64, side dto.TradeSide, quantity, priceHundredths int64, date string) dto.TradeRow {
	return dto.TradeRow{
		ID: id, Code: code, Name: "テスト銘柄" + code, Currency: cur, Side: side,
		Quantity: quantity, PriceHundredths: priceHundredths, TradeDate: date,
	}
}

func alBuy(code string, id, quantity, priceHundredths int64, date string) dto.TradeRow {
	return alTrade(code, dto.CurrencyJPY, id, dto.TradeSideBuy, quantity, priceHundredths, date)
}

func alSell(code string, id, quantity, priceHundredths int64, date string) dto.TradeRow {
	return alTrade(code, dto.CurrencyJPY, id, dto.TradeSideSell, quantity, priceHundredths, date)
}

// alNewInput は、銘柄 code・通貨 cur の、取引日 date・終値 closeHundredths の入力を作る。date がその銘柄の最新の取引日で、
// 分割・条件・売買・前の終値・終値の列・前の判定は空（テストが足す）。
func alNewInput(code string, cur dto.Currency, date string, closeHundredths int64) dto.AlertEvaluationInput {
	return dto.AlertEvaluationInput{
		StockCode:            code,
		Currency:             cur,
		TradingDate:          date,
		ClosePriceHundredths: closeHundredths,
		LatestTradingDate:    date,
		TradesByUser:         map[uuid.UUID][]dto.TradeRow{},
		PreviousJudgments:    map[int64]dto.AlertJudgmentRow{},
	}
}

// alToyotaInput は、7203（円）の、取引日 alDate・終値 closeHundredths の入力。Aが「買 2,500円 100株 2026-01-10」を持つ。
func alToyotaInput(closeHundredths int64) dto.AlertEvaluationInput {
	in := alNewInput("7203", dto.CurrencyJPY, alDate, closeHundredths)
	in.TradesByUser[alUserA] = []dto.TradeRow{alBuy("7203", 1, 100, alYen(2500), "2026-01-10")}
	return in
}

func alEvaluate(t *testing.T, in dto.AlertEvaluationInput) dto.AlertEvaluationResult {
	t.Helper()
	got, err := query.EvaluateAlerts(in)
	if err != nil {
		t.Fatalf("EvaluateAlerts: got err %v, want nil", err)
	}
	return got
}

func alJudgment(conditionID int64, code, date string, satisfied bool) dto.AlertJudgment {
	return dto.AlertJudgment{ConditionID: conditionID, StockCode: code, TradingDate: date, Satisfied: satisfied}
}

// alNotification は、条件 cond の通知（利用者は条件の利用者）。
func alNotification(cond dto.AlertConditionRow, code, date string, closeHundredths int64, details dto.AlertNotificationDetails) dto.AlertNotification {
	return dto.AlertNotification{
		UserID: cond.UserID, Condition: cond, StockCode: code, TradingDate: date,
		ClosePriceHundredths: closeHundredths, Details: details,
	}
}

// 通知の値（details）。nil のキーは付けない。
func alRate(rateTenths, basePrice int64) dto.AlertNotificationDetails {
	return dto.AlertNotificationDetails{RateTenths: alPtr(rateTenths), BasePriceHundredths: alPtr(basePrice)}
}

func alRateOn(rateTenths, basePrice int64, baseDate string) dto.AlertNotificationDetails {
	return dto.AlertNotificationDetails{RateTenths: alPtr(rateTenths), BasePriceHundredths: alPtr(basePrice), BaseDate: alPtr(baseDate)}
}

func alPnlDetails(gain, basePrice, quantity int64) dto.AlertNotificationDetails {
	return dto.AlertNotificationDetails{
		UnrealizedGainHundredths: alPtr(gain), BasePriceHundredths: alPtr(basePrice), Quantity: alPtr(quantity),
	}
}

func alAdjusted(adjustedThreshold int64) dto.AlertNotificationDetails {
	return dto.AlertNotificationDetails{AdjustedThresholdHundredths: alPtr(adjustedThreshold)}
}

func alNormalize[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// alAssertResult は、判定の結果と作る通知が want と同じ（並びも条件 ID の昇順で同じ）ことを確かめる。
func alAssertResult(t *testing.T, got dto.AlertEvaluationResult, wantJudgments []dto.AlertJudgment, wantNotifications []dto.AlertNotification) {
	t.Helper()
	if !reflect.DeepEqual(alNormalize(got.Judgments), alNormalize(wantJudgments)) {
		t.Errorf("Judgments:\n got %+v\nwant %+v", got.Judgments, wantJudgments)
	}
	if !reflect.DeepEqual(alNormalize(got.Notifications), alNormalize(wantNotifications)) {
		t.Errorf("Notifications:\n got %s\nwant %s", alDescribe(got.Notifications), alDescribe(wantNotifications))
	}
}

// alDescribe は、通知の比べた結果を読みやすくする（details はポインターなので値にして出す）。
func alDescribe(ns []dto.AlertNotification) string {
	s := "["
	for i, n := range ns {
		if i > 0 {
			s += ", "
		}
		s += "{cond:" + strconv.FormatInt(n.Condition.ID, 10) + " user:" + n.UserID.String() + " " + n.StockCode + " " + n.TradingDate +
			" close:" + strconv.FormatInt(n.ClosePriceHundredths, 10) + " details:" + alDetailsString(n.Details) + "}"
	}
	return s + "]"
}

func alDetailsString(d dto.AlertNotificationDetails) string {
	s := "{"
	add := func(name string, v *int64) {
		if v != nil {
			s += name + "=" + strconv.FormatInt(*v, 10) + " "
		}
	}
	add("rateTenths", d.RateTenths)
	add("basePrice", d.BasePriceHundredths)
	if d.BaseDate != nil {
		s += "baseDate=" + *d.BaseDate + " "
	}
	add("gain", d.UnrealizedGainHundredths)
	add("quantity", d.Quantity)
	add("adjusted", d.AdjustedThresholdHundredths)
	return s + "}"
}

// ---- 5種類の境目（5.2 の1行目。統合の基準は向きの一部だけなので、ここで全部の向きを見る）----

// alBoundaryCase は、1つの条件と終値で、当てはまるか（当てはまるなら通知の値）を確かめる1件。
type alBoundaryCase struct {
	name          string
	in            dto.AlertEvaluationInput // Conditions は run が cond で上書きする
	cond          dto.AlertConditionRow
	wantSatisfied bool
	wantDetails   dto.AlertNotificationDetails // 当てはまるときだけ
}

func alRunBoundaryCases(t *testing.T, cases []alBoundaryCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			in := tc.in
			in.Conditions = []dto.AlertConditionRow{tc.cond}

			got := alEvaluate(t, in)

			var wantJudgments []dto.AlertJudgment
			if tc.cond.Kind != alPrev { // 前の取引日からの判定は書かない（4.2）
				wantJudgments = []dto.AlertJudgment{alJudgment(tc.cond.ID, in.StockCode, in.TradingDate, tc.wantSatisfied)}
			}
			var wantNotifications []dto.AlertNotification
			if tc.wantSatisfied {
				wantNotifications = []dto.AlertNotification{alNotification(tc.cond, in.StockCode, in.TradingDate, in.ClosePriceHundredths, tc.wantDetails)}
			}
			alAssertResult(t, got, wantJudgments, wantNotifications)
		})
	}
}

// 取得価格から（要件11）: Aが 7203「買 2,500円 100株」。率は小数第2位を絶対値で四捨五入した値で比べる。
// 下がった 20%: 2,000円は -20.0%、2,001円は -19.96% → -20.0%（当てはまる）、2,002円は -19.92% → -19.9%（0.1% 手前で当てはまらない）。
// 上がった 20%: 3,000円は +20.0%、2,999円は +19.96% → +20.0%、2,998円は +19.92% → +19.9%。向きが逆なら当てはまらない。
func Test要件11_取得価格からは丸めた率が値に届いたとき当てはまる(t *testing.T) {
	t.Parallel()
	down := alAllCond(1, alUserA, alCost, alDown, 2000)
	up := alAllCond(1, alUserA, alCost, alUp, 2000)
	alRunBoundaryCases(t, []alBoundaryCase{
		{"下がった_ちょうど-20.0%", alToyotaInput(alYen(2000)), down, true, alRate(-200, alYen(2500))},
		{"下がった_-19.96%は-20.0%に丸めて当てはまる", alToyotaInput(alYen(2001)), down, true, alRate(-200, alYen(2500))},
		{"下がった_-19.92%は-19.9%で当てはまらない", alToyotaInput(alYen(2002)), down, false, dto.AlertNotificationDetails{}},
		{"下がった_上がっていれば当てはまらない", alToyotaInput(alYen(3000)), down, false, dto.AlertNotificationDetails{}},
		{"上がった_ちょうど+20.0%", alToyotaInput(alYen(3000)), up, true, alRate(200, alYen(2500))},
		{"上がった_+19.96%は+20.0%に丸めて当てはまる", alToyotaInput(alYen(2999)), up, true, alRate(200, alYen(2500))},
		{"上がった_+19.92%は+19.9%で当てはまらない", alToyotaInput(alYen(2998)), up, false, dto.AlertNotificationDetails{}},
		{"上がった_下がっていれば当てはまらない", alToyotaInput(alYen(2000)), up, false, dto.AlertNotificationDetails{}},
	})
}

// alPeakInput は、7203 の最高値の判定の入力。Aが「買 2,500円 100株 2026-01-10」。終値の列は 2026-06-01 4,000円と D の終値（最高値は 4,000円）。
func alPeakInput(closeHundredths int64) dto.AlertEvaluationInput {
	in := alToyotaInput(closeHundredths)
	in.Closes = []dto.ClosingPriceRow{
		{TradingDate: "2026-06-01", ClosePriceHundredths: alYen(4000)},
		{TradingDate: alDate, ClosePriceHundredths: closeHundredths},
	}
	return in
}

// 最高値から（要件12）: 最高値 4,000円（2026-06-01）。15%: 3,400円は -15.0%（当てはまる）、3,404円は -14.9%（0.1% 手前で当てはまらない）。
// 3,402円は -14.95% → -15.0% に丸めて当てはまる（率がちょうど x.x5% のときの丸め）。
func Test要件12_最高値からは丸めた率が値に届いたとき当てはまる(t *testing.T) {
	t.Parallel()
	cond := alAllCond(1, alUserA, alPeak, alDown, 1500)
	alRunBoundaryCases(t, []alBoundaryCase{
		{"ちょうど-15.0%", alPeakInput(alYen(3400)), cond, true, alRateOn(-150, alYen(4000), "2026-06-01")},
		{"-14.95%は-15.0%に丸めて当てはまる", alPeakInput(alYen(3402)), cond, true, alRateOn(-150, alYen(4000), "2026-06-01")},
		{"-15.1%", alPeakInput(alYen(3396)), cond, true, alRateOn(-151, alYen(4000), "2026-06-01")},
		{"-14.9%は当てはまらない", alPeakInput(alYen(3404)), cond, false, dto.AlertNotificationDetails{}},
	})
}

// alPrevInput は、7203 の前の取引日からの判定の入力。前の終値は 2026-11-09 の 3,000円。
func alPrevInput(closeHundredths int64) dto.AlertEvaluationInput {
	in := alToyotaInput(closeHundredths)
	in.PreviousClose = &dto.ClosingPriceRow{TradingDate: "2026-11-09", ClosePriceHundredths: alYen(3000)}
	return in
}

// 前の取引日から（要件13）: 前の終値 3,000円（2026-11-09）。5% で、上がった・下がった・動いた（絶対値）のすべて。
// 3,150円は +5.0%（当てはまる）、3,147円は +4.9%（0.1% 手前）、2,850円は -5.0%、2,853円は -4.9%。
// 3,148.5円は +4.95% → +5.0%、2,851.5円は -4.95% → -5.0% に丸めて当てはまる（率がちょうど x.x5% のときの丸め）。
func Test要件13_前の取引日からは向きごとに丸めた率が値に届いたとき当てはまる(t *testing.T) {
	t.Parallel()
	up := alAllCond(1, alUserA, alPrev, alUp, 500)
	down := alAllCond(1, alUserA, alPrev, alDown, 500)
	either := alAllCond(1, alUserA, alPrev, alEither, 500)
	none := dto.AlertNotificationDetails{}
	alRunBoundaryCases(t, []alBoundaryCase{
		{"上がった_+5.0%", alPrevInput(alYen(3150)), up, true, alRateOn(50, alYen(3000), "2026-11-09")},
		{"上がった_+4.95%は+5.0%に丸めて当てはまる", alPrevInput(314850), up, true, alRateOn(50, alYen(3000), "2026-11-09")},
		{"下がった_-4.95%は-5.0%に丸めて当てはまる", alPrevInput(285150), down, true, alRateOn(-50, alYen(3000), "2026-11-09")},
		{"上がった_+4.9%は当てはまらない", alPrevInput(alYen(3147)), up, false, none},
		{"上がった_下がっていれば当てはまらない", alPrevInput(alYen(2850)), up, false, none},
		{"下がった_-5.0%", alPrevInput(alYen(2850)), down, true, alRateOn(-50, alYen(3000), "2026-11-09")},
		{"下がった_-4.9%は当てはまらない", alPrevInput(alYen(2853)), down, false, none},
		{"下がった_上がっていれば当てはまらない", alPrevInput(alYen(3150)), down, false, none},
		{"動いた_+5.0%", alPrevInput(alYen(3150)), either, true, alRateOn(50, alYen(3000), "2026-11-09")},
		{"動いた_-5.0%", alPrevInput(alYen(2850)), either, true, alRateOn(-50, alYen(3000), "2026-11-09")},
		{"動いた_+4.9%は当てはまらない", alPrevInput(alYen(3147)), either, false, none},
		{"動いた_-4.9%は当てはまらない", alPrevInput(alYen(2853)), either, false, none},
	})
}

// alPnlInput は、7203 の含み損益の金額の判定の入力。Aが「買 4,000円 200株」（取得額 800,000円）。
func alPnlInput(closeHundredths int64) dto.AlertEvaluationInput {
	in := alNewInput("7203", dto.CurrencyJPY, alDate, closeHundredths)
	in.TradesByUser[alUserA] = []dto.TradeRow{alBuy("7203", 1, 200, alYen(4000), "2026-01-10")}
	return in
}

// 含み損益の金額（要件14）: 含み損 100,000円: 3,500円は -100,000円（当てはまる）、3,505円は -99,000円。
// 含み益 100,000円: 4,500円は +100,000円（当てはまる）、4,495円は +99,000円。
func Test要件14_含み損益の金額は含み損益が値に届いたとき当てはまる(t *testing.T) {
	t.Parallel()
	loss := alStockCond(1, alUserA, alPnl, "7203", alLoss, alYen(100000))
	gain := alStockCond(1, alUserA, alPnl, "7203", alGain, alYen(100000))
	none := dto.AlertNotificationDetails{}
	alRunBoundaryCases(t, []alBoundaryCase{
		{"含み損_ちょうど-100,000円", alPnlInput(alYen(3500)), loss, true, alPnlDetails(alYen(-100000), alYen(4000), 200)},
		{"含み損_-99,000円は当てはまらない", alPnlInput(alYen(3505)), loss, false, none},
		{"含み損_含み益なら当てはまらない", alPnlInput(alYen(4500)), loss, false, none},
		{"含み益_ちょうど+100,000円", alPnlInput(alYen(4500)), gain, true, alPnlDetails(alYen(100000), alYen(4000), 200)},
		{"含み益_+99,000円は当てはまらない", alPnlInput(alYen(4495)), gain, false, none},
		{"含み益_含み損なら当てはまらない", alPnlInput(alYen(3500)), gain, false, none},
	})
}

// 含み損益の金額（要件14。ドルはドルのまま）: Aが AAPL「買 150.00ドル 10株」、含み益 300.00ドル。180.00ドルは +300.00ドル（当てはまる）、
// 179.90ドルは +299.00ドル（当てはまらない）。
func Test要件14_含み損益の金額はドルのままドルの値と比べる(t *testing.T) {
	t.Parallel()
	in := func(closeHundredths int64) dto.AlertEvaluationInput {
		i := alNewInput("AAPL", dto.CurrencyUSD, alDate, closeHundredths)
		i.TradesByUser[alUserA] = []dto.TradeRow{alTrade("AAPL", dto.CurrencyUSD, 1, dto.TradeSideBuy, 10, 15000, "2026-01-10")}
		return i
	}
	gain := alStockCond(1, alUserA, alPnl, "AAPL", alGain, 30000)
	alRunBoundaryCases(t, []alBoundaryCase{
		{"含み益_ちょうど+300.00ドル", in(18000), gain, true, alPnlDetails(30000, 15000, 10)},
		{"含み益_+299.00ドルは当てはまらない", in(17990), gain, false, dto.AlertNotificationDetails{}},
	})
}

// 決めた価格（要件15）: 3,000円。以上は 3,000円（ちょうど）と 3,000.1円が当てはまり、2,999.9円は当てはまらない。
// 以下は 3,000円と 2,999.9円が当てはまり、3,000.1円は当てはまらない。通知の値（details）は、割ったとき以外は何もない。
func Test要件15_決めた価格は終値が決めた価格に届いたとき当てはまる(t *testing.T) {
	t.Parallel()
	above := alStockCond(1, alUserA, alPrice, "7203", alAbove, alYen(3000))
	below := alStockCond(1, alUserA, alPrice, "7203", alBelow, alYen(3000))
	none := dto.AlertNotificationDetails{}
	alRunBoundaryCases(t, []alBoundaryCase{
		{"以上_ちょうど", alToyotaInput(alYen(3000)), above, true, none},
		{"以上_0.1円上", alToyotaInput(300010), above, true, none},
		{"以上_0.1円手前は当てはまらない", alToyotaInput(299990), above, false, none},
		{"以下_ちょうど", alToyotaInput(alYen(3000)), below, true, none},
		{"以下_0.1円下", alToyotaInput(299990), below, true, none},
		{"以下_0.1円上は当てはまらない", alToyotaInput(300010), below, false, none},
	})
}

// ---- 前の判定（要件16・要件17）----

// alCostDownInput は、取得価格から 20% 下がったら（全銘柄・Aの条件 ID 1）の判定の入力。Aが「買 2,500円 100株 2026-01-10」で、
// 2026-11-11 の終値が 1,950円（-22.0%。当てはまる）。
func alCostDownInput() dto.AlertEvaluationInput {
	in := alNewInput("7203", dto.CurrencyJPY, "2026-11-11", alYen(1950))
	in.TradesByUser[alUserA] = []dto.TradeRow{alBuy("7203", 1, 100, alYen(2500), "2026-01-10")}
	in.Conditions = []dto.AlertConditionRow{alAllCond(1, alUserA, alCost, alDown, 2000)}
	return in
}

// 要件16: 前の判定が「当てはまった」なら通知を作らず、「当てはまらなかった」「ない」なら作る。判定の結果は、どの場合も今回の分を書く。
func Test要件16_前の判定が当てはまっていなければ当てはまった今回に通知を作る(t *testing.T) {
	t.Parallel()
	cond := alAllCond(1, alUserA, alCost, alDown, 2000)
	notified := []dto.AlertNotification{alNotification(cond, "7203", "2026-11-11", alYen(1950), alRate(-220, alYen(2500)))}
	judgments := []dto.AlertJudgment{alJudgment(1, "7203", "2026-11-11", true)}
	cases := []struct {
		name     string
		previous *dto.AlertJudgmentRow
		want     []dto.AlertNotification
	}{
		{"前の判定が当てはまった", &dto.AlertJudgmentRow{ConditionID: 1, TradingDate: "2026-11-10", Satisfied: true}, nil},
		{"前の判定が当てはまらなかった", &dto.AlertJudgmentRow{ConditionID: 1, TradingDate: "2026-11-10", Satisfied: false}, notified},
		{"前の判定がない", nil, notified},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			in := alCostDownInput()
			if tc.previous != nil {
				in.PreviousJudgments[1] = *tc.previous
			}

			alAssertResult(t, alEvaluate(t, in), judgments, tc.want)
		})
	}
}

// 要件16: 今の保有が始まった日 S より前の判定は、前の判定に数えない（売り切って買い直したら前の判定はないものとする）。S 以後の判定は数える（S ちょうども含む）。
// Aは「買 100株 2026-01-10」「売 100株 2026-02-10」「買 100株 2026-03-10」（S は 2026-03-10）。
func Test要件16_前の判定が今の保有が始まる前なら前の判定に数えない(t *testing.T) {
	t.Parallel()
	trades := []dto.TradeRow{
		alBuy("7203", 1, 100, alYen(2500), "2026-01-10"),
		alSell("7203", 2, 100, alYen(2800), "2026-02-10"),
		alBuy("7203", 3, 100, alYen(2500), "2026-03-10"),
	}
	cond := alAllCond(1, alUserA, alCost, alDown, 2000)
	notified := []dto.AlertNotification{alNotification(cond, "7203", "2026-11-11", alYen(1950), alRate(-220, alYen(2500)))}
	cases := []struct {
		name     string
		previous dto.AlertJudgmentRow
		want     []dto.AlertNotification
	}{
		{"Sの前日に当てはまっていても数えない", dto.AlertJudgmentRow{ConditionID: 1, TradingDate: "2026-03-09", Satisfied: true}, notified},
		{"Sの日に当てはまっていたら数える", dto.AlertJudgmentRow{ConditionID: 1, TradingDate: "2026-03-10", Satisfied: true}, nil},
		{"Sより後に当てはまっていたら数える", dto.AlertJudgmentRow{ConditionID: 1, TradingDate: "2026-11-10", Satisfied: true}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			in := alCostDownInput()
			in.TradesByUser[alUserA] = trades
			in.PreviousJudgments[1] = tc.previous

			alAssertResult(t, alEvaluate(t, in), []dto.AlertJudgment{alJudgment(1, "7203", "2026-11-11", true)}, tc.want)
		})
	}
}

// 要件17: 前の取引日からは、前の判定によらず、当てはまった取引日ごとに通知を作る。前の取引日からの判定の結果は書かない（4.2）。
func Test要件17_前の取引日からは前の判定によらず当てはまれば通知を作り判定の結果は書かない(t *testing.T) {
	t.Parallel()
	cond := alAllCond(1, alUserA, alPrev, alUp, 500)
	for _, previous := range []*dto.AlertJudgmentRow{
		nil,
		{ConditionID: 1, TradingDate: "2026-11-09", Satisfied: true},
		{ConditionID: 1, TradingDate: "2026-11-09", Satisfied: false},
	} {
		in := alPrevInput(alYen(3150))
		in.Conditions = []dto.AlertConditionRow{cond}
		name := "前の判定がない"
		if previous != nil {
			in.PreviousJudgments[1] = *previous
			name = "前の判定が当てはまらなかった"
			if previous.Satisfied {
				name = "前の判定が当てはまった"
			}
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			alAssertResult(t, alEvaluate(t, in), nil,
				[]dto.AlertNotification{alNotification(cond, "7203", alDate, alYen(3150), alRateOn(50, alYen(3000), "2026-11-09"))})
		})
	}
}

// ---- 最高値の境目（要件12）----

// alSonyInput は、6758（円）の最高値の判定の入力。Aが「買 3,000円 100株 2026-03-02」「売 3,500円 100株 2026-07-01」「買 3,400円 100株 2026-09-01」で、
// 今の保有が始まった日 S は 2026-09-01。終値の列は 2026-08-31 4,100円、09-01 3,800円、10-01 3,600円、D（2026-11-10）3,230円。
func alSonyInput() dto.AlertEvaluationInput {
	in := alNewInput("6758", dto.CurrencyJPY, alDate, alYen(3230))
	in.TradesByUser[alUserA] = []dto.TradeRow{
		alBuy("6758", 1, 100, alYen(3000), "2026-03-02"),
		alSell("6758", 2, 100, alYen(3500), "2026-07-01"),
		alBuy("6758", 3, 100, alYen(3400), "2026-09-01"),
	}
	in.Closes = []dto.ClosingPriceRow{
		{TradingDate: "2026-08-31", ClosePriceHundredths: alYen(4100)},
		{TradingDate: "2026-09-01", ClosePriceHundredths: alYen(3800)},
		{TradingDate: "2026-10-01", ClosePriceHundredths: alYen(3600)},
		{TradingDate: alDate, ClosePriceHundredths: alYen(3230)},
	}
	return in
}

// 要件12: 最高値の範囲は、今の保有が始まった日 S から（S を含む。S より前の終値は含まない）。
// 条件は最高値から 15% 下がったら → 1件、rateTenths -150、basePriceHundredths 380000、baseDate 2026-09-01
// （S を含めない実装は 3,600円で -10.3% になり0件、S より前を含める実装は 4,100円）。
func Test要件12_最高値の範囲は今の保有が始まった日を含みそれより前を含まない(t *testing.T) {
	t.Parallel()
	cond := alAllCond(1, alUserA, alPeak, alDown, 1500)
	in := alSonyInput()
	in.Conditions = []dto.AlertConditionRow{cond}

	alAssertResult(t, alEvaluate(t, in),
		[]dto.AlertJudgment{alJudgment(1, "6758", alDate, true)},
		[]dto.AlertNotification{alNotification(cond, "6758", alDate, alYen(3230), alRateOn(-150, alYen(3800), "2026-09-01"))})
}

// 要件12・要件16: 同じデータで、前の判定が S ちょうど（2026-09-01）で当てはまっていたら通知を作らず、S の前日（2026-08-31）で当てはまっていたら作る。
func Test要件16_最高値からの前の判定はSちょうどなら数えSの前なら数えない(t *testing.T) {
	t.Parallel()
	cond := alAllCond(1, alUserA, alPeak, alDown, 1500)
	notified := []dto.AlertNotification{alNotification(cond, "6758", alDate, alYen(3230), alRateOn(-150, alYen(3800), "2026-09-01"))}
	cases := []struct {
		name     string
		previous string
		want     []dto.AlertNotification
	}{
		{"Sちょうどで当てはまっていたら作らない", "2026-09-01", nil},
		{"Sの前日で当てはまっていたら作る", "2026-08-31", notified},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			in := alSonyInput()
			in.Conditions = []dto.AlertConditionRow{cond}
			in.PreviousJudgments[1] = dto.AlertJudgmentRow{ConditionID: 1, TradingDate: tc.previous, Satisfied: true}

			alAssertResult(t, alEvaluate(t, in), []dto.AlertJudgment{alJudgment(1, "6758", alDate, true)}, tc.want)
		})
	}
}

// 要件12: 最高値が同じ値で2日あるときは、いちばん新しい日を最高値の日にする。
// Aが「買 2,500円 100株 2026-01-10」、終値は 2026-08-01 4,000円、2026-09-01 4,000円、D 3,400円 → baseDate 2026-09-01、-15.0%。条件は 10%。
func Test要件12_最高値が同じ値で2日あるときは新しい日を最高値の日にする(t *testing.T) {
	t.Parallel()
	cond := alAllCond(1, alUserA, alPeak, alDown, 1000)
	in := alToyotaInput(alYen(3400))
	in.Closes = []dto.ClosingPriceRow{
		{TradingDate: "2026-08-01", ClosePriceHundredths: alYen(4000)},
		{TradingDate: "2026-09-01", ClosePriceHundredths: alYen(4000)},
		{TradingDate: alDate, ClosePriceHundredths: alYen(3400)},
	}
	in.Conditions = []dto.AlertConditionRow{cond}

	alAssertResult(t, alEvaluate(t, in),
		[]dto.AlertJudgment{alJudgment(1, "7203", alDate, true)},
		[]dto.AlertNotification{alNotification(cond, "7203", alDate, alYen(3400), alRateOn(-150, alYen(4000), "2026-09-01"))})
}

// 要件12: 最高値が D の終値なら、率は 0.0% で当てはまらない（判定の結果は当てはまらないで書く）。
func Test要件12_最高値が判定する日の終値なら率は0で当てはまらない(t *testing.T) {
	t.Parallel()
	cond := alAllCond(1, alUserA, alPeak, alDown, 1000)
	in := alToyotaInput(alYen(3500))
	in.Closes = []dto.ClosingPriceRow{
		{TradingDate: "2026-06-01", ClosePriceHundredths: alYen(3000)},
		{TradingDate: alDate, ClosePriceHundredths: alYen(3500)},
	}
	in.Conditions = []dto.AlertConditionRow{cond}

	alAssertResult(t, alEvaluate(t, in), []dto.AlertJudgment{alJudgment(1, "7203", alDate, false)}, nil)
}

// 要件12（判定する日の値は、いつも ClosePriceHundredths を使う）: 終値の列の D の行が 4,500円で、ClosePriceHundredths が 3,400円のとき
// （同じ取引日に違う値が同時に保存された場合）。前の日までの最高値は 4,000円（2026-06-01）。条件は最高値から 15% 下がったら
// → 最高値は 4,000円（2026-06-01）で -15.0%（列の D の行 4,500円を最高値にして -24.4% にしない）。
func Test要件12_最高値の列の判定する日の行は使わず判定する日の値はClosePriceHundredthsを使う(t *testing.T) {
	t.Parallel()
	cond := alAllCond(1, alUserA, alPeak, alDown, 1500)
	in := alToyotaInput(alYen(3400))
	in.Closes = []dto.ClosingPriceRow{
		{TradingDate: "2026-06-01", ClosePriceHundredths: alYen(4000)},
		{TradingDate: alDate, ClosePriceHundredths: alYen(4500)},
	}
	in.Conditions = []dto.AlertConditionRow{cond}

	alAssertResult(t, alEvaluate(t, in),
		[]dto.AlertJudgment{alJudgment(1, "7203", alDate, true)},
		[]dto.AlertNotification{alNotification(cond, "7203", alDate, alYen(3400), alRateOn(-150, alYen(4000), "2026-06-01"))})
}

// 要件12: 分割の前の終値は、その日より後の分割の株数で割って比べる。7203 の分割 1→2 2026-09-30、終値は 2026-09-29 4,000円、2026-10-01 1,900円、
// D 1,750円 → 最高値 2,000円（2026-09-29）、-12.5%。
func Test要件12_最高値は分割の前の終値を分割の株数で割って比べる(t *testing.T) {
	t.Parallel()
	cond := alAllCond(1, alUserA, alPeak, alDown, 1000)
	in := alToyotaInput(alYen(1750))
	in.Splits = []dto.StockSplitRow{{Code: "7203", SplitDate: "2026-09-30", Ratio: 2}}
	in.Closes = []dto.ClosingPriceRow{
		{TradingDate: "2026-09-29", ClosePriceHundredths: alYen(4000)},
		{TradingDate: "2026-10-01", ClosePriceHundredths: alYen(1900)},
		{TradingDate: alDate, ClosePriceHundredths: alYen(1750)},
	}
	in.Conditions = []dto.AlertConditionRow{cond}

	alAssertResult(t, alEvaluate(t, in),
		[]dto.AlertJudgment{alJudgment(1, "7203", alDate, true)},
		[]dto.AlertNotification{alNotification(cond, "7203", alDate, alYen(1750), alRateOn(-125, alYen(2000), "2026-09-29"))})
}

// 要件12: 分割の日と同じ日の終値は割らない（分割の日は分割の後の値段で売買が始まる日）。7203 の分割 1→2 2026-09-30、
// 終値は 2026-09-29 4,000円（→ 2,000円）、2026-09-30 2,100円（割らない）、D 1,750円 → 最高値 2,100円（2026-09-30）、-16.7%。
func Test要件12_最高値は分割の日と同じ日の終値を割らない(t *testing.T) {
	t.Parallel()
	cond := alAllCond(1, alUserA, alPeak, alDown, 1000)
	in := alToyotaInput(alYen(1750))
	in.Splits = []dto.StockSplitRow{{Code: "7203", SplitDate: "2026-09-30", Ratio: 2}}
	in.Closes = []dto.ClosingPriceRow{
		{TradingDate: "2026-09-29", ClosePriceHundredths: alYen(4000)},
		{TradingDate: "2026-09-30", ClosePriceHundredths: alYen(2100)},
		{TradingDate: alDate, ClosePriceHundredths: alYen(1750)},
	}
	in.Conditions = []dto.AlertConditionRow{cond}

	alAssertResult(t, alEvaluate(t, in),
		[]dto.AlertJudgment{alJudgment(1, "7203", alDate, true)},
		[]dto.AlertNotification{alNotification(cond, "7203", alDate, alYen(1750), alRateOn(-167, alYen(2100), "2026-09-30"))})
}

// ---- 決めた価格の割る数（要件15）----

// alPriceDivisorCase は、決めた価格の条件に、分割を渡して判定する1件。
type alPriceDivisorCase struct {
	name          string
	cond          dto.AlertConditionRow
	splits        []dto.StockSplitRow
	closeHundreds int64
	wantSatisfied bool
	wantDetails   dto.AlertNotificationDetails
}

func alSplit(date string, ratio int64) dto.StockSplitRow {
	return dto.StockSplitRow{Code: "7203", SplitDate: date, Ratio: ratio}
}

// 要件15: 決めた価格は、条件を足した日（日本時間）より後で D 以前の分割の株数の積で割った値（丸めない）と終値を比べる。
// 足した日と同じ日の分割では割らない。通知の値は、割る数が1より大きいときだけ、割った値（表示の丸め）を持つ。
func Test要件15_決めた価格は足した日より後で判定する日以前の分割の株数の積で割る(t *testing.T) {
	t.Parallel()
	cases := []alPriceDivisorCase{
		{
			name:   "足した日の翌日の分割は割る（8,000円 → 4,000円。4,100円は当てはまる）",
			cond:   alCond(1, alUserA, alPrice, alStock, "7203", alAbove, alYen(8000), "2026-11-02"),
			splits: []dto.StockSplitRow{alSplit("2026-11-03", 2)}, closeHundreds: alYen(4100),
			wantSatisfied: true, wantDetails: alAdjusted(alYen(4000)),
		},
		{
			name:   "足した日の翌日の分割は割る（3,999.9円は当てはまらない）",
			cond:   alCond(1, alUserA, alPrice, alStock, "7203", alAbove, alYen(8000), "2026-11-02"),
			splits: []dto.StockSplitRow{alSplit("2026-11-03", 2)}, closeHundreds: 399990,
			wantSatisfied: false,
		},
		{
			name:   "足した日と同じ日の分割では割らない（8,200円のまま。4,100円は当てはまらない）",
			cond:   alCond(1, alUserA, alPrice, alStock, "7203", alAbove, alYen(8200), "2026-11-10"),
			splits: []dto.StockSplitRow{alSplit("2026-11-10", 2)}, closeHundreds: alYen(4100),
			wantSatisfied: false,
		},
		{
			name:   "足した日と同じ日の分割では割らない（8,200円は当てはまり、割っていないので割った値は持たない）",
			cond:   alCond(1, alUserA, alPrice, alStock, "7203", alAbove, alYen(8200), "2026-11-10"),
			splits: []dto.StockSplitRow{alSplit("2026-11-10", 2)}, closeHundreds: alYen(8200),
			wantSatisfied: true, wantDetails: dto.AlertNotificationDetails{},
		},
		{
			name:   "足した日より前の分割では割らない",
			cond:   alCond(1, alUserA, alPrice, alStock, "7203", alAbove, alYen(3000), "2026-11-02"),
			splits: []dto.StockSplitRow{alSplit("2026-10-01", 2)}, closeHundreds: alYen(3000),
			wantSatisfied: true, wantDetails: dto.AlertNotificationDetails{},
		},
		{
			name:   "複数の分割は株数をすべて掛けて割る（6,000円 ÷ 2 ÷ 3 ＝ 1,000円。1,000円は当てはまる）",
			cond:   alCond(1, alUserA, alPrice, alStock, "7203", alAbove, alYen(6000), "2026-11-02"),
			splits: []dto.StockSplitRow{alSplit("2026-11-03", 2), alSplit("2026-11-05", 3)}, closeHundreds: alYen(1000),
			wantSatisfied: true, wantDetails: alAdjusted(alYen(1000)),
		},
		{
			name:   "複数の分割は株数をすべて掛けて割る（999.9円は当てはまらない）",
			cond:   alCond(1, alUserA, alPrice, alStock, "7203", alAbove, alYen(6000), "2026-11-02"),
			splits: []dto.StockSplitRow{alSplit("2026-11-03", 2), alSplit("2026-11-05", 3)}, closeHundreds: 99990,
			wantSatisfied: false,
		},
		{
			name:   "割った値は丸めずに比べる（10,000円 ÷ 3 ＝ 3,333.33…円。3,333.3円は届かない）",
			cond:   alCond(1, alUserA, alPrice, alStock, "7203", alAbove, alYen(10000), "2026-11-02"),
			splits: []dto.StockSplitRow{alSplit("2026-11-03", 3)}, closeHundreds: 333330,
			wantSatisfied: false,
		},
		{
			name:   "割った値は丸めずに比べる（3,333.4円は届く。通知の割った値は 3,333.3円に丸めて持つ）",
			cond:   alCond(1, alUserA, alPrice, alStock, "7203", alAbove, alYen(10000), "2026-11-02"),
			splits: []dto.StockSplitRow{alSplit("2026-11-03", 3)}, closeHundreds: 333340,
			wantSatisfied: true, wantDetails: alAdjusted(333330),
		},
		{
			name:   "以下も割る（8,000円 ÷ 2 ＝ 4,000円以下。3,900円は当てはまる）",
			cond:   alCond(1, alUserA, alPrice, alStock, "7203", alBelow, alYen(8000), "2026-11-02"),
			splits: []dto.StockSplitRow{alSplit("2026-11-03", 2)}, closeHundreds: alYen(3900),
			wantSatisfied: true, wantDetails: alAdjusted(alYen(4000)),
		},
	}
	var boundary []alBoundaryCase
	for _, tc := range cases {
		in := alToyotaInput(tc.closeHundreds)
		in.Splits = tc.splits
		boundary = append(boundary, alBoundaryCase{tc.name, in, tc.cond, tc.wantSatisfied, tc.wantDetails})
	}
	alRunBoundaryCases(t, boundary)
}

// ---- 手順1（要件9）----

// 要件9: D がその銘柄の最新の取引日でないとき（古い日の終値を保存し直したとき）は、何もしない（判定の結果も通知も空）。
func Test要件9_判定する日が最新の取引日でなければ何もしない(t *testing.T) {
	t.Parallel()
	in := alToyotaInput(alYen(1950)) // 取得価格から 20% 下がったら当てはまる終値
	in.TradingDate = "2026-11-12"
	in.LatestTradingDate = "2026-11-13"
	in.Conditions = []dto.AlertConditionRow{alAllCond(1, alUserA, alCost, alDown, 2000)}

	got := alEvaluate(t, in)

	if got.Judgments == nil || got.Notifications == nil {
		t.Errorf("0件は空のスライス（nil ではない）: Judgments=%v Notifications=%v", got.Judgments, got.Notifications)
	}
	alAssertResult(t, got, nil, nil)
}

// 要件9: その銘柄の最新の分割の日が D より後のとき（分割の後の価格をまだ取得していないとき）は、何もしない。
// 7203 の分割 1→2 2026-11-17、D は 2026-11-16（最新の取引日）で 2,900円、条件は 3,000円 以下。
func Test要件9_最新の分割の日が判定する日より後なら何もしない(t *testing.T) {
	t.Parallel()
	in := alToyotaInput(alYen(2900))
	in.TradingDate = "2026-11-16"
	in.LatestTradingDate = "2026-11-16"
	in.Splits = []dto.StockSplitRow{alSplit("2026-11-17", 2)}
	in.Conditions = []dto.AlertConditionRow{alStockCond(1, alUserA, alPrice, "7203", alBelow, alYen(3000))}

	got := alEvaluate(t, in)

	if got.Judgments == nil || got.Notifications == nil {
		t.Errorf("0件は空のスライス（nil ではない）: Judgments=%v Notifications=%v", got.Judgments, got.Notifications)
	}
	alAssertResult(t, got, nil, nil)
}

// ---- 利用者ごとの計算（要件10・4.3 の手順2）----

// 要件10: 保有数量が0の利用者（売り切った・売買履歴がない）の条件は飛ばし、判定の結果も書かない。
// 全銘柄の条件で、2人の利用者の別々の結果が出る。
// Aは「買 2,500円 100株」（1,950円で -22.0%。当てはまる）、Bは「買 2,000円 100株」（-2.5%。当てはまらない）、
// Cは「買 100株」「売 100株」（保有なし）、Dは売買履歴がない。全員が全銘柄・取得価格から 20% 下がったら。
func Test要件10_保有数量が0の利用者の条件は飛ばし保有している利用者ごとに結果を出す(t *testing.T) {
	t.Parallel()
	condA := alAllCond(1, alUserA, alCost, alDown, 2000)
	condB := alAllCond(2, alUserB, alCost, alDown, 2000)
	condC := alAllCond(3, alUserC, alCost, alDown, 2000)
	condD := alAllCond(4, alUserD, alCost, alDown, 2000)
	in := alNewInput("7203", dto.CurrencyJPY, alDate, alYen(1950))
	in.Conditions = []dto.AlertConditionRow{condA, condB, condC, condD}
	in.TradesByUser[alUserA] = []dto.TradeRow{alBuy("7203", 1, 100, alYen(2500), "2026-01-10")}
	in.TradesByUser[alUserB] = []dto.TradeRow{alBuy("7203", 2, 100, alYen(2000), "2026-01-10")}
	in.TradesByUser[alUserC] = []dto.TradeRow{
		alBuy("7203", 3, 100, alYen(2500), "2026-01-10"),
		alSell("7203", 4, 100, alYen(2800), "2026-02-10"),
	}

	alAssertResult(t, alEvaluate(t, in),
		[]dto.AlertJudgment{alJudgment(1, "7203", alDate, true), alJudgment(2, "7203", alDate, false)},
		[]dto.AlertNotification{alNotification(condA, "7203", alDate, alYen(1950), alRate(-220, alYen(2500)))})
}

// 判定の結果と通知は、入力の条件の並びによらず、条件 ID の昇順に返る（4.8）。
func Test補助_結果は条件IDの昇順に返る(t *testing.T) {
	t.Parallel()
	c1 := alAllCond(1, alUserA, alCost, alDown, 500)
	c2 := alAllCond(2, alUserA, alCost, alDown, 1000)
	c3 := alAllCond(3, alUserA, alCost, alDown, 2000)
	in := alCostDownInput()
	in.Conditions = []dto.AlertConditionRow{c3, c1, c2}

	alAssertResult(t, alEvaluate(t, in),
		[]dto.AlertJudgment{
			alJudgment(1, "7203", "2026-11-11", true), alJudgment(2, "7203", "2026-11-11", true), alJudgment(3, "7203", "2026-11-11", true),
		},
		[]dto.AlertNotification{
			alNotification(c1, "7203", "2026-11-11", alYen(1950), alRate(-220, alYen(2500))),
			alNotification(c2, "7203", "2026-11-11", alYen(1950), alRate(-220, alYen(2500))),
			alNotification(c3, "7203", "2026-11-11", alYen(1950), alRate(-220, alYen(2500))),
		})
}

// ---- 分割と取得価格・含み損益（要件11・要件14）----

// 要件11・要件14: 取得価格からと含み損益の金額は、分割を使って計算する。Aが 7203「買 2,500円 100株」、分割 1→2 2026-06-01、D の終値 1,000円
// （200株・取得価格 1,250円）。取得価格から 20% 下がったら → -20.0%（rateTenths -200、basePriceHundredths 125000）。
// 含み損が 50,000円 → -50,000円（unrealizedGainHundredths -5000000、quantity 200、basePriceHundredths 125000）
// （分割を渡さない実装は -60.0%・-150,000円・100株）。
func Test要件11_要件14_取得価格からと含み損益の金額は分割を使って計算する(t *testing.T) {
	t.Parallel()
	costCond := alAllCond(1, alUserA, alCost, alDown, 2000)
	pnlCond := alStockCond(2, alUserA, alPnl, "7203", alLoss, alYen(50000))
	in := alToyotaInput(alYen(1000))
	in.Splits = []dto.StockSplitRow{alSplit("2026-06-01", 2)}
	in.Conditions = []dto.AlertConditionRow{costCond, pnlCond}

	alAssertResult(t, alEvaluate(t, in),
		[]dto.AlertJudgment{alJudgment(1, "7203", alDate, true), alJudgment(2, "7203", alDate, true)},
		[]dto.AlertNotification{
			alNotification(costCond, "7203", alDate, alYen(1000), alRate(-200, 125000)),
			alNotification(pnlCond, "7203", alDate, alYen(1000), alPnlDetails(alYen(-50000), 125000, 200)),
		})
}

// 要件11: 買い増しなどで取得価格が変わったら、変わった後の取得価格で判定する。Aが「買 1,000円 100株」「買 800円 100株」（取得価格 900円）、
// 終値 720円で -20.0%（条件は取得価格から 20% 下がったら）。
func Test要件11_取得価格は買い増しの後の移動平均で判定する(t *testing.T) {
	t.Parallel()
	cond := alAllCond(1, alUserA, alCost, alDown, 2000)
	in := alNewInput("7203", dto.CurrencyJPY, alDate, alYen(720))
	in.TradesByUser[alUserA] = []dto.TradeRow{
		alBuy("7203", 1, 100, alYen(1000), "2026-01-10"),
		alBuy("7203", 2, 100, alYen(800), "2026-11-11"),
	}
	in.Conditions = []dto.AlertConditionRow{cond}

	alAssertResult(t, alEvaluate(t, in),
		[]dto.AlertJudgment{alJudgment(1, "7203", alDate, true)},
		[]dto.AlertNotification{alNotification(cond, "7203", alDate, alYen(720), alRate(-200, alYen(900)))})
}

// ---- 前の取引日から（要件13）----

// 要件13: 前の終値と D の間に分割があるときは、前の終値をその分割後の株数で割る。7203「買 2,500円 100株」、前の終値 2026-11-12 3,000円、
// 分割 1→2 2026-11-13、D=2026-11-13 の終値 1,600円、条件は前の取引日から 5% 上がったら → 1件、rateTenths 67、
// basePriceHundredths 150000、baseDate 2026-11-12（分割で判定を飛ばす実装、basePrice に割る前の値を入れる実装で落ちる）。
func Test要件13_前の取引日からは前の終値と判定する日の間の分割で前の終値を割る(t *testing.T) {
	t.Parallel()
	cond := alAllCond(1, alUserA, alPrev, alUp, 500)
	in := alToyotaInput(alYen(1600))
	in.TradingDate = "2026-11-13"
	in.LatestTradingDate = "2026-11-13"
	in.PreviousClose = &dto.ClosingPriceRow{TradingDate: "2026-11-12", ClosePriceHundredths: alYen(3000)}
	in.Splits = []dto.StockSplitRow{alSplit("2026-11-13", 2)}
	in.Conditions = []dto.AlertConditionRow{cond}

	alAssertResult(t, alEvaluate(t, in), nil,
		[]dto.AlertNotification{alNotification(cond, "7203", "2026-11-13", alYen(1600), alRateOn(67, 150000, "2026-11-12"))})
}

// 要件13: 前の終値の取引日と同じ日の分割では、前の終値を割らない（前の終値はすでに分割の後の値段）。
// 前の終値 2026-11-12 1,500円、分割 1→2 2026-11-12、D=2026-11-13 の終値 1,600円 → 1,500円と比べて +6.7%（rateTenths 67）。
func Test要件13_前の取引日からは前の終値と同じ日の分割では割らない(t *testing.T) {
	t.Parallel()
	cond := alAllCond(1, alUserA, alPrev, alUp, 500)
	in := alToyotaInput(alYen(1600))
	in.TradingDate = "2026-11-13"
	in.LatestTradingDate = "2026-11-13"
	in.PreviousClose = &dto.ClosingPriceRow{TradingDate: "2026-11-12", ClosePriceHundredths: alYen(1500)}
	in.Splits = []dto.StockSplitRow{alSplit("2026-11-12", 2)}
	in.Conditions = []dto.AlertConditionRow{cond}

	alAssertResult(t, alEvaluate(t, in), nil,
		[]dto.AlertNotification{alNotification(cond, "7203", "2026-11-13", alYen(1600), alRateOn(67, alYen(1500), "2026-11-12"))})
}

// 要件13: 前の終値がない（PreviousClose が nil）ときは、前の取引日からの条件だけを飛ばす（エラーにしない）。
// 取得価格から 20% 下がったら（当てはまる）は、通常どおり1件。前の取引日からの判定の結果は書かない。
func Test要件13_前の終値がなければ前の取引日からの条件だけを飛ばしエラーにしない(t *testing.T) {
	t.Parallel()
	prevCond := alAllCond(1, alUserA, alPrev, alUp, 500)
	costCond := alAllCond(2, alUserA, alCost, alDown, 2000)
	in := alToyotaInput(alYen(1950))
	in.PreviousClose = nil
	in.Conditions = []dto.AlertConditionRow{prevCond, costCond}

	got, err := query.EvaluateAlerts(in)

	if err != nil {
		t.Fatalf("EvaluateAlerts: got err %v, want nil（前の終値がないのはエラーにしない）", err)
	}
	alAssertResult(t, got,
		[]dto.AlertJudgment{alJudgment(2, "7203", alDate, true)},
		[]dto.AlertNotification{alNotification(costCond, "7203", alDate, alYen(1950), alRate(-220, alYen(2500)))})
}

// ---- 全銘柄の条件と同じ銘柄の条件（要件19）----

// 要件19: 全銘柄の条件と、同じ銘柄だけの条件は別々の条件として判定し、それぞれ通知を作る。
// Aが 7203「買 2,500円 100株」、条件は全銘柄・取得価格から 20% 下がったら と 7203・取得価格から 10% 下がったら。
// 前の判定なしで 1,950円（-22.0%）→ 同じ取引日に条件ごとの2件。
func Test要件19_全銘柄の条件と同じ銘柄の条件は同じ日に別々の通知を作る(t *testing.T) {
	t.Parallel()
	allCond := alAllCond(1, alUserA, alCost, alDown, 2000)
	stockCond := alStockCond(2, alUserA, alCost, "7203", alDown, 1000)
	in := alToyotaInput(alYen(1950))
	in.Conditions = []dto.AlertConditionRow{allCond, stockCond}

	alAssertResult(t, alEvaluate(t, in),
		[]dto.AlertJudgment{alJudgment(1, "7203", alDate, true), alJudgment(2, "7203", alDate, true)},
		[]dto.AlertNotification{
			alNotification(allCond, "7203", alDate, alYen(1950), alRate(-220, alYen(2500))),
			alNotification(stockCond, "7203", alDate, alYen(1950), alRate(-220, alYen(2500))),
		})
}

// ---- 利用者ごとの失敗（要件21・4.3）----

// alTwoUsers は、Aが正常（買 2,500円 100株。-22.0% で当てはまる）、Bの売買履歴が bTrades の入力。どちらも全銘柄・取得価格から 20% 下がったら。
func alTwoUsers(bTrades []dto.TradeRow) (dto.AlertEvaluationInput, dto.AlertConditionRow) {
	condA := alAllCond(1, alUserA, alCost, alDown, 2000)
	condB := alAllCond(2, alUserB, alCost, alDown, 2000)
	in := alToyotaInput(alYen(1950))
	in.Conditions = []dto.AlertConditionRow{condA, condB}
	in.TradesByUser[alUserB] = bTrades
	return in, condA
}

// 要件21: Bの売買履歴が売りすぎ（保有数量が負になる）でも、Aの判定の結果と通知は返り、エラーは ErrNegativeQuantity を含む。
// Bの条件は判定の結果も通知も作らない。
func Test要件21_ある利用者の売りすぎはその利用者の条件だけを飛ばしほかの利用者の結果を返す(t *testing.T) {
	t.Parallel()
	in, condA := alTwoUsers([]dto.TradeRow{
		alBuy("7203", 2, 100, alYen(2500), "2026-01-10"),
		alSell("7203", 3, 200, alYen(2600), "2026-02-10"),
	})

	got, err := query.EvaluateAlerts(in)

	if !errors.Is(err, query.ErrNegativeQuantity) {
		t.Errorf("EvaluateAlerts の err: got %v, want ErrNegativeQuantity を含む", err)
	}
	alAssertResult(t, got,
		[]dto.AlertJudgment{alJudgment(1, "7203", alDate, true)},
		[]dto.AlertNotification{alNotification(condA, "7203", alDate, alYen(1950), alRate(-220, alYen(2500)))})
}

// 要件21: Bの金額があふれる売買（保有数量が int64 に収まらない）でも同じ（ErrCalculationOverflow）。
func Test要件21_ある利用者の金額があふれてもその利用者の条件だけを飛ばしほかの利用者の結果を返す(t *testing.T) {
	t.Parallel()
	in, condA := alTwoUsers([]dto.TradeRow{
		alBuy("7203", 2, 4611686018427387904, 10, "2026-01-10"),
		alBuy("7203", 3, 4611686018427387904, 10, "2026-01-11"),
	})

	got, err := query.EvaluateAlerts(in)

	if !errors.Is(err, query.ErrCalculationOverflow) {
		t.Errorf("EvaluateAlerts の err: got %v, want ErrCalculationOverflow を含む", err)
	}
	alAssertResult(t, got,
		[]dto.AlertJudgment{alJudgment(1, "7203", alDate, true)},
		[]dto.AlertNotification{alNotification(condA, "7203", alDate, alYen(1950), alRate(-220, alYen(2500)))})
}

// 要件10（基準57。判定する取引日 D と今の保有が始まった日 S の比べ方。> と >= の書き間違いを見分ける）:
// Aは「買 3,000円 100株 2026-01-10」「売 3,000円 100株 2026-02-10」のあと、買い直す日 S に「買 3,000円 100株」。D は 2026-11-10、終値 2,000円、前の終値 3,000円（2026-11-09）。
// 条件は取得価格から 20% 下がったら（条件1）と、前の取引日から 5% 下がったら（条件2）で、どちらも当てはまる値（-33.3%）。
// S が D の翌日（2026-11-11）なら、判定の結果も通知も0件（今の保有が始まる前の日付は判定しない）。
// S が D と同じ日（2026-11-10）なら判定する（条件1の判定の結果「当てはまった」と、条件1・条件2の通知。前の取引日からは判定の結果を書かない）。
func Test要件10_判定する日が今の保有が始まった日より前なら判定せず同じ日なら判定する(t *testing.T) {
	t.Parallel()
	cost := alAllCond(1, alUserA, alCost, alDown, 2000)
	prev := alAllCond(2, alUserA, alPrev, alDown, 500)
	cases := []struct {
		name              string
		rebuyDate         string
		wantJudgments     []dto.AlertJudgment
		wantNotifications []dto.AlertNotification
	}{
		{"買い直しの日が判定する日の翌日なら判定しない", "2026-11-11", nil, nil},
		{
			"買い直しの日が判定する日と同じなら判定する", alDate,
			[]dto.AlertJudgment{alJudgment(1, "7203", alDate, true)},
			[]dto.AlertNotification{
				alNotification(cost, "7203", alDate, alYen(2000), alRate(-333, alYen(3000))),
				alNotification(prev, "7203", alDate, alYen(2000), alRateOn(-333, alYen(3000), "2026-11-09")),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			in := alNewInput("7203", dto.CurrencyJPY, alDate, alYen(2000))
			in.TradesByUser[alUserA] = []dto.TradeRow{
				alBuy("7203", 1, 100, alYen(3000), "2026-01-10"),
				alSell("7203", 2, 100, alYen(3000), "2026-02-10"),
				alBuy("7203", 3, 100, alYen(3000), tc.rebuyDate),
			}
			in.PreviousClose = &dto.ClosingPriceRow{TradingDate: "2026-11-09", ClosePriceHundredths: alYen(3000)}
			in.Conditions = []dto.AlertConditionRow{cost, prev}

			alAssertResult(t, alEvaluate(t, in), tc.wantJudgments, tc.wantNotifications)
		})
	}
}
