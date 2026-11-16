package model_test

import (
	"errors"
	"math"
	"math/big"
	"slices"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
)

// 仕様: 要件（docs/requirements/trade-history.md）の要件3（株数は1以上で1,000万株未満、価格は0より大きく1億未満で円は小数第1位まで・
// ドルは小数第2位まで、取引日は取り込んだ日（日本時間）以前）・要件8（取引日の古い順、同じ取引日は登録した順）・
// 要件9（持っている株数より多く売らない）・要件24・基準25・基準27・基準28 と、
// プラン（docs/plans/trade-history/plan.md）4.6（PR③）の model.Trade・NewTrade・FindOversells・Oversell、
// 4.7 の CheckQuantity・CheckPrice・Currency.PriceHundredths・CheckTradeDate・ErrQuantityNotPositive・ErrQuantityTooLarge、
// 5.3（PR③）の「NewStock・NewTrade（単体）: 各エラーの境界」「FindOversells（単体）」。
// 価格は 0.01 単位の整数（Hundredths）。例: 250000 は 2,500円または 2,500.00ドル。

// today は、「今日」が 2026-11-16（日本時間）のテスト（要件定義の成功基準の前置き）。
const today = "2026-11-16"

// rat は、10進数の文字列 s を *big.Rat にする。
func rat(t *testing.T, s string) *big.Rat {
	t.Helper()
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		t.Fatalf("テストの値 %q を数にできない", s)
	}
	return r
}

// stockOf は、通貨 currency の銘柄 code を作る（NewStock を通さず、構造体で直接作る）。
func stockOf(code string, currency model.Currency) model.Stock {
	return model.Stock{Code: code, Name: "テスト銘柄" + code, Currency: currency}
}

// 株数の確かめ（要件3）。1以上で1,000万株未満（9,999,999株まで）。
func Test基準25_CheckQuantityは1以上9999999以下を受け付ける(t *testing.T) {
	t.Parallel()
	for _, q := range []int64{1, 2, 100, 9_999_999} {
		if err := model.CheckQuantity(q); err != nil {
			t.Errorf("CheckQuantity(%d): got %v, want nil", q, err)
		}
	}
	for _, q := range []int64{0, -1, math.MinInt64} {
		if err := model.CheckQuantity(q); !errors.Is(err, model.ErrQuantityNotPositive) {
			t.Errorf("CheckQuantity(%d): got %v, want ErrQuantityNotPositive", q, err)
		}
	}
	for _, q := range []int64{10_000_000, 10_000_001, math.MaxInt64} {
		if err := model.CheckQuantity(q); !errors.Is(err, model.ErrQuantityTooLarge) {
			t.Errorf("CheckQuantity(%d): got %v, want ErrQuantityTooLarge", q, err)
		}
	}
}

// 価格の確かめ（要件3）。0より大きく1億未満。精度（小数第何位まで）は通貨ごとなので PriceHundredths で確かめる。
func Test基準25_CheckPriceは0より大きく1億未満を受け付ける(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"0.01", "0.1", "1", "2500", "150.25", "99999999.9", "99999999.99"} {
		if err := model.CheckPrice(rat(t, s)); err != nil {
			t.Errorf("CheckPrice(%s): got %v, want nil", s, err)
		}
	}
	for _, s := range []string{"0", "0.00", "-1", "-0.01"} {
		if err := model.CheckPrice(rat(t, s)); !errors.Is(err, model.ErrPriceNotPositive) {
			t.Errorf("CheckPrice(%s): got %v, want ErrPriceNotPositive", s, err)
		}
	}
	for _, s := range []string{"100000000", "100000000.00", "100000000.5", "999999999999"} {
		if err := model.CheckPrice(rat(t, s)); !errors.Is(err, model.ErrPriceTooLarge) {
			t.Errorf("CheckPrice(%s): got %v, want ErrPriceTooLarge", s, err)
		}
	}
}

// 通貨ごとの価格の精度（要件3・要件24）。円は小数第1位まで、ドルは小数第2位まで。結果は 0.01 単位の整数。
func Test基準25_PriceHundredthsは通貨の精度に合う価格を0_01単位の整数にする(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		currency model.Currency
		price    string
		want     int64
		wantErr  error
	}{
		{"円_2500は250000", model.CurrencyJPY, "2500", 250000, nil},
		{"円_0.1は10", model.CurrencyJPY, "0.1", 10, nil},
		{"円_2000.5は200050", model.CurrencyJPY, "2000.5", 200050, nil},
		{"円_2000.0は200000", model.CurrencyJPY, "2000.0", 200000, nil},
		{"円_99999999.9は9999999990", model.CurrencyJPY, "99999999.9", 9_999_999_990, nil},
		{"円_0.05は小数第2位を含む", model.CurrencyJPY, "0.05", 0, model.ErrPriceTooFine},
		{"円_0.01は小数第2位を含む", model.CurrencyJPY, "0.01", 0, model.ErrPriceTooFine},
		{"円_2000.25は小数第2位を含む", model.CurrencyJPY, "2000.25", 0, model.ErrPriceTooFine},
		{"円_2000.255は小数第3位を含む", model.CurrencyJPY, "2000.255", 0, model.ErrPriceTooFine},
		{"円_99999999.99は小数第2位を含む", model.CurrencyJPY, "99999999.99", 0, model.ErrPriceTooFine},
		{"ドル_150.25は15025", model.CurrencyUSD, "150.25", 15025, nil},
		{"ドル_0.01は1", model.CurrencyUSD, "0.01", 1, nil},
		{"ドル_400は40000", model.CurrencyUSD, "400", 40000, nil},
		{"ドル_150.5は15050", model.CurrencyUSD, "150.5", 15050, nil},
		{"ドル_99999999.99は9999999999", model.CurrencyUSD, "99999999.99", 9_999_999_999, nil},
		{"ドル_150.255は小数第3位を含む", model.CurrencyUSD, "150.255", 0, model.ErrPriceTooFine},
		{"ドル_0.001は小数第3位を含む", model.CurrencyUSD, "0.001", 0, model.ErrPriceTooFine},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := tc.currency.PriceHundredths(rat(t, tc.price))
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("%s.PriceHundredths(%s): got (%d, %v), want err %v", tc.currency, tc.price, got, err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("%s.PriceHundredths(%s): got (%d, %v), want (%d, nil)", tc.currency, tc.price, got, err, tc.want)
			}
		})
	}
}

// 取引日の確かめ（要件3）。取り込んだ日（日本時間）以前。今日（2026-11-16）は受け付け、明日（2026-11-17）は受け付けない。
func Test基準25_CheckTradeDateは今日以前を受け付け未来を受け付けない(t *testing.T) {
	t.Parallel()
	for _, d := range []string{"2026-11-16", "2026-11-15", "2026-01-06", "2025-12-31", "2024-02-29", "0001-01-01"} {
		if err := model.CheckTradeDate(d, today); err != nil {
			t.Errorf("CheckTradeDate(%s, %s): got %v, want nil", d, today, err)
		}
	}
	for _, d := range []string{"2026-11-17", "2026-12-01", "2027-01-01", "2026-12-31", "9999-12-31"} {
		if err := model.CheckTradeDate(d, today); !errors.Is(err, model.ErrFutureTradeDate) {
			t.Errorf("CheckTradeDate(%s, %s): got %v, want ErrFutureTradeDate", d, today, err)
		}
	}
	// 今日が変われば境界も変わる（今日は引数。実行した日に頼らない）。
	if err := model.CheckTradeDate("2026-11-17", "2026-11-17"); err != nil {
		t.Errorf("CheckTradeDate(2026-11-17, 今日が 2026-11-17): got %v, want nil", err)
	}
	if err := model.CheckTradeDate("2026-11-17", "2026-11-16"); !errors.Is(err, model.ErrFutureTradeDate) {
		t.Errorf("CheckTradeDate(2026-11-17, 今日が 2026-11-16): got %v, want ErrFutureTradeDate", err)
	}
}

// NewTrade は、銘柄 stock の売買を作る（要件2・3）。値はそのまま Trade に入る。
func Test要件3_NewTradeは正しい値から売買を作る(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		stock model.Stock
		side  model.TradeSide
		qty   int64
		price int64
		date  string
	}{
		{"円の買い", stockOf("7203", model.CurrencyJPY), model.TradeSideBuy, 100, 250000, "2026-01-10"},
		{"円の売り", stockOf("7203", model.CurrencyJPY), model.TradeSideSell, 50, 300050, "2026-02-10"},
		{"ドルの買い", stockOf("AAPL", model.CurrencyUSD), model.TradeSideBuy, 10, 15025, "2026-01-10"},
		{"取引日が今日", stockOf("7203", model.CurrencyJPY), model.TradeSideBuy, 1, 10, today},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := model.NewTrade(tc.stock, tc.side, tc.qty, tc.price, tc.date, today)
			if err != nil {
				t.Fatalf("NewTrade: got err %v, want nil", err)
			}
			want := model.Trade{StockCode: tc.stock.Code, Side: tc.side, Quantity: tc.qty, PriceHundredths: tc.price, TradeDate: tc.date}
			if got != want {
				t.Errorf("NewTrade: got %+v, want %+v", got, want)
			}
		})
	}
}

// 5.3: NewTrade の各エラーの境界。株数 9,999,999 と 10,000,000、価格 99,999,999.99 と 100,000,000.00、円の 0.1 と 0.05、取引日が今日と明日。
func Test基準25_NewTradeは株数と価格と取引日の境界でエラーを返す(t *testing.T) {
	t.Parallel()
	yen := stockOf("7203", model.CurrencyJPY)
	usd := stockOf("AAPL", model.CurrencyUSD)
	cases := []struct {
		name    string
		stock   model.Stock
		qty     int64
		price   int64
		date    string
		wantErr error
	}{
		{"株数 9,999,999 は受け付ける", usd, 9_999_999, 15025, "2026-01-10", nil},
		{"株数 10,000,000 は多すぎる", usd, 10_000_000, 15025, "2026-01-10", model.ErrQuantityTooLarge},
		{"株数 1 は受け付ける", usd, 1, 15025, "2026-01-10", nil},
		{"株数 0 は1未満", usd, 0, 15025, "2026-01-10", model.ErrQuantityNotPositive},
		{"株数 -1 は1未満", usd, -1, 15025, "2026-01-10", model.ErrQuantityNotPositive},
		{"ドルの価格 99,999,999.99 は受け付ける", usd, 10, 9_999_999_999, "2026-01-10", nil},
		{"ドルの価格 100,000,000.00 は1億以上", usd, 10, 10_000_000_000, "2026-01-10", model.ErrPriceTooLarge},
		{"ドルの価格 0.01 は受け付ける", usd, 10, 1, "2026-01-10", nil},
		{"ドルの価格 0 は0以下", usd, 10, 0, "2026-01-10", model.ErrPriceNotPositive},
		{"ドルの価格 -0.01 は0以下", usd, 10, -1, "2026-01-10", model.ErrPriceNotPositive},
		{"円の価格 99,999,999.9 は受け付ける", yen, 10, 9_999_999_990, "2026-01-10", nil},
		{"円の価格 100,000,000.0 は1億以上", yen, 10, 10_000_000_000, "2026-01-10", model.ErrPriceTooLarge},
		{"円の価格 0.1 は受け付ける", yen, 10, 10, "2026-01-10", nil},
		{"円の価格 0.05 は小数第2位を含む", yen, 10, 5, "2026-01-10", model.ErrPriceTooFine},
		{"円の価格 2,000.25 は小数第2位を含む", yen, 10, 200025, "2026-01-10", model.ErrPriceTooFine},
		{"ドルの価格 2,000.25 は受け付ける", usd, 10, 200025, "2026-01-10", nil},
		{"取引日が今日は受け付ける", yen, 10, 250000, "2026-11-16", nil},
		{"取引日が明日は未来", yen, 10, 250000, "2026-11-17", model.ErrFutureTradeDate},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := model.NewTrade(tc.stock, model.TradeSideBuy, tc.qty, tc.price, tc.date, today)
			if tc.wantErr == nil {
				if err != nil {
					t.Errorf("NewTrade: got err %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("NewTrade: got err %v, want %v", err, tc.wantErr)
			}
		})
	}
}

// buy・sell は、FindOversells のテスト用の売買を作る（価格は結果に関係しないので固定）。
func buy(code string, qty int64, date string) model.Trade {
	return model.Trade{StockCode: code, Side: model.TradeSideBuy, Quantity: qty, PriceHundredths: 250000, TradeDate: date}
}

func sell(code string, qty int64, date string) model.Trade {
	return model.Trade{StockCode: code, Side: model.TradeSideSell, Quantity: qty, PriceHundredths: 250000, TradeDate: date}
}

// 基準27・要件9: 売りの株数が、その売りの直前の保有数量より多ければ、多すぎる売り（Index は渡したスライスの添字、Held・Sold は直前の保有数量と売る株数）。
func Test基準27_FindOversellsは直前の保有数量より多い売りを返す(t *testing.T) {
	t.Parallel()
	trades := []model.Trade{
		buy("7203", 100, "2026-01-10"),
		sell("7203", 150, "2026-02-10"),
	}
	got := model.FindOversells(trades)
	want := []model.Oversell{{Index: 1, Held: 100, Sold: 150}}
	if !slices.Equal(got, want) {
		t.Errorf("FindOversells: got %+v, want %+v", got, want)
	}
}

// 要件9: ちょうど持っている株数まで売るのは多すぎる売りではない。売った後の保有数量が次の売りの「直前の保有数量」になる。
func Test要件9_FindOversellsは持っている株数ちょうどまでの売りと売りの積み重ねを正しく数える(t *testing.T) {
	t.Parallel()
	exact := []model.Trade{buy("7203", 100, "2026-01-10"), sell("7203", 100, "2026-02-10")}
	if got := model.FindOversells(exact); len(got) != 0 {
		t.Errorf("FindOversells(ちょうど売り切り): got %+v, want なし", got)
	}
	// 買い 100 → 売り 40（残り 60）→ 売り 70（直前の保有数量は 60 なので多すぎる）。
	stacked := []model.Trade{buy("7203", 100, "2026-01-10"), sell("7203", 40, "2026-02-10"), sell("7203", 70, "2026-03-10")}
	got := model.FindOversells(stacked)
	want := []model.Oversell{{Index: 2, Held: 60, Sold: 70}}
	if !slices.Equal(got, want) {
		t.Errorf("FindOversells(売りの積み重ね): got %+v, want %+v", got, want)
	}
}

// 基準28・要件8: 取引日の順にたどる。スライスの順が逆（売りが先）でも、取引日の古い買いが先なので、多すぎる売りはない。
func Test基準28_FindOversellsは取引日の順にたどる(t *testing.T) {
	t.Parallel()
	trades := []model.Trade{
		sell("7203", 100, "2026-02-10"),
		buy("7203", 100, "2026-01-10"),
	}
	if got := model.FindOversells(trades); len(got) != 0 {
		t.Errorf("FindOversells(行の順が取引日の順と逆): got %+v, want なし（取引日の順にたどると売りの時点で100株持っている）", got)
	}
}

// 基準28 の対照: 取引日の順にたどると売りが先になるとき、Index は渡したスライスの添字のまま（並べ替えた後の添字ではない）。
func Test基準28_FindOversellsのIndexは渡したスライスの添字である(t *testing.T) {
	t.Parallel()
	trades := []model.Trade{
		sell("7203", 150, "2026-02-10"), // 添字 0。取引日の順では 2番目。直前の保有数量は 100
		buy("7203", 100, "2026-01-10"),  // 添字 1。取引日の順では最初
	}
	got := model.FindOversells(trades)
	want := []model.Oversell{{Index: 0, Held: 100, Sold: 150}}
	if !slices.Equal(got, want) {
		t.Errorf("FindOversells: got %+v, want %+v", got, want)
	}
}

// 要件8: 同じ取引日は、スライスの順（登録した順）にたどる。売り→買いなら売りの時点で0株、買い→売りなら多すぎない。
func Test要件8_FindOversellsは同じ取引日をスライスの順にたどる(t *testing.T) {
	t.Parallel()
	sellFirst := []model.Trade{sell("7203", 100, "2026-03-02"), buy("7203", 100, "2026-03-02")}
	got := model.FindOversells(sellFirst)
	want := []model.Oversell{{Index: 0, Held: 0, Sold: 100}}
	if !slices.Equal(got, want) {
		t.Errorf("FindOversells(同じ日に売りが先): got %+v, want %+v", got, want)
	}
	buyFirst := []model.Trade{buy("7203", 100, "2026-03-02"), sell("7203", 100, "2026-03-02")}
	if got := model.FindOversells(buyFirst); len(got) != 0 {
		t.Errorf("FindOversells(同じ日に買いが先): got %+v, want なし", got)
	}
}

// 5.3: 2銘柄でそれぞれ最初の1件だけ返す。結果は Index の昇順（銘柄コードの順ではない）。
// 6758 の多すぎる売り（添字 3）は 7203 の最初の多すぎる売り（添字 2）より後。7203 の2回目の多すぎる売り（添字 4）は返さない。
func Test補助_FindOversellsは銘柄ごとに最初の1件だけをIndexの昇順で返す(t *testing.T) {
	t.Parallel()
	trades := []model.Trade{
		buy("6758", 10, "2026-01-10"),   // 0
		buy("7203", 100, "2026-01-10"),  // 1
		sell("7203", 150, "2026-02-10"), // 2: 多すぎる（直前 100）
		sell("6758", 20, "2026-02-10"),  // 3: 多すぎる（直前 10）
		sell("7203", 200, "2026-03-10"), // 4: これも多すぎるが、7203 は最初の1件だけ
	}
	got := model.FindOversells(trades)
	want := []model.Oversell{
		{Index: 2, Held: 100, Sold: 150},
		{Index: 3, Held: 10, Sold: 20},
	}
	if !slices.Equal(got, want) {
		t.Errorf("FindOversells: got %+v, want %+v", got, want)
	}
}

// 銘柄が違えば保有数量は別々に数える（他の銘柄の買いで売れるようにならない）。
func Test補助_FindOversellsは銘柄ごとに保有数量を数える(t *testing.T) {
	t.Parallel()
	trades := []model.Trade{
		buy("7203", 100, "2026-01-10"),
		sell("6758", 10, "2026-02-10"), // 6758 は持っていない
	}
	got := model.FindOversells(trades)
	want := []model.Oversell{{Index: 1, Held: 0, Sold: 10}}
	if !slices.Equal(got, want) {
		t.Errorf("FindOversells: got %+v, want %+v", got, want)
	}
}

// 売買履歴が0件でも壊れない。
func Test補助_FindOversellsは売買履歴が0件なら何も返さない(t *testing.T) {
	t.Parallel()
	if got := model.FindOversells(nil); len(got) != 0 {
		t.Errorf("FindOversells(nil): got %+v, want なし", got)
	}
	if got := model.FindOversells([]model.Trade{}); len(got) != 0 {
		t.Errorf("FindOversells(空): got %+v, want なし", got)
	}
}
