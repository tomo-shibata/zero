package model_test

import (
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
)

// 仕様: プラン（docs/plans/stock-holdings/plan.md）4.4 の domain/model
// （NewClosingPrice は priceHundredths <= 0、code が空、tradingDate が YYYY-MM-DD の実在する日付でないときはエラー）と、
// 5.1「補助のテスト（バックエンド）」の 4.4 NewClosingPrice（PR③ の EC-P1-2）の行
// （エラー: priceHundredths 0・-1、code が空、tradingDate "2026-02-30"・"2026/10/06"・"2026-1-6"・""。
// 成功: priceHundredths 1・tradingDate "2026-10-06"）。
// 背景は docs/plans/stock-holdings/reviews/pr3-step6.md の EC-P1-2。
//
// エラーのケースは、成功のケース（7203・"2026-10-06"・1）から1項目だけ変えたものにする（ほかの項目が理由でエラーになっていないことを
// 成功のケースで確かめられるように）。銘柄コード 7203 はプランに指定がないので、要件のダミー価格の銘柄にした。

// validCode・validTradingDate・minPriceHundredths は、プラン 5.1 の成功のケースの値（銘柄コードは上のとおり選んだ値）。
const (
	validCode          = "7203"
	validTradingDate   = "2026-10-06"
	minPriceHundredths = 1 // 0.01（0.01 単位。プラン 4.3）
)

// 4.4 NewClosingPrice（PR③ の EC-P1-2）: 0.01 単位の終値が 1 以上、銘柄コードが空でない、取引日が YYYY-MM-DD の実在する日付なら
// 終値を作れ、そのどれかを満たさなければエラーになる。
func TestPlan4_4_PR3_EC_P1_2_NewClosingPriceは0以下の終値や空の銘柄コードやYYYY_MM_DDの実在する日付でない取引日をエラーにする(t *testing.T) {
	t.Parallel()
	errorCases := []struct {
		name            string
		code            string
		tradingDate     string
		priceHundredths int64
	}{
		{name: "priceHundredths0", code: validCode, tradingDate: validTradingDate, priceHundredths: 0},
		{name: "priceHundredths-1", code: validCode, tradingDate: validTradingDate, priceHundredths: -1},
		{name: "codeが空", code: "", tradingDate: validTradingDate, priceHundredths: minPriceHundredths},
		{name: "tradingDate_2026-02-30_実在しない日付", code: validCode, tradingDate: "2026-02-30", priceHundredths: minPriceHundredths},
		{name: "tradingDate_2026/10/06_区切りが違う", code: validCode, tradingDate: "2026/10/06", priceHundredths: minPriceHundredths},
		{name: "tradingDate_2026-1-6_月日が2桁でない", code: validCode, tradingDate: "2026-1-6", priceHundredths: minPriceHundredths},
		{name: "tradingDateが空", code: validCode, tradingDate: "", priceHundredths: minPriceHundredths},
	}
	for _, tc := range errorCases {
		t.Run("エラー_"+tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := model.NewClosingPrice(tc.code, tc.tradingDate, tc.priceHundredths)
			if err == nil {
				t.Errorf("model.NewClosingPrice(%q, %q, %d): err が nil（want エラー）。返った値: %+v",
					tc.code, tc.tradingDate, tc.priceHundredths, got)
			}
		})
	}

	t.Run("成功_priceHundredths1_tradingDate_2026-10-06", func(t *testing.T) {
		t.Parallel()
		got, err := model.NewClosingPrice(validCode, validTradingDate, minPriceHundredths)
		if err != nil {
			t.Fatalf("model.NewClosingPrice(%q, %q, %d): got err %v, want nil",
				validCode, validTradingDate, minPriceHundredths, err)
		}
		want := model.ClosingPrice{StockCode: "7203", TradingDate: "2026-10-06", PriceHundredths: 1}
		if got != want {
			t.Errorf("model.NewClosingPrice(%q, %q, %d): got %+v, want %+v",
				validCode, validTradingDate, minPriceHundredths, got, want)
		}
	})
}
