package query_test

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/application/query"
	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// 仕様: 要件（docs/requirements/trade-history.md）の基準1〜5・基準12・基準14（保有の計算と通貨ごとの表示。要件6〜8・要件10〜15）と、
// プラン（docs/plans/trade-history/plan.md）4.3（保有の計算）・4.6（PR② の境界）・5.1（PR② の行）・5.3・5.5（PR② の補助のテスト）。
// 確かめる関数は 4.6 の query.CalculateHoldings。PR② で、金額は円もドルも 0.01 単位の整数（名前の末尾が Hundredths。
// 2,500円 = 250000、150.25ドル = 15025）にし、一覧は通貨ごとの合計 Totals を持つ。
// 保有株式一覧の AC-6（並び）・AC-10（評価額の切り捨て）と、集約の補助のテスト（FR-4・FR-5・FR-6・FR-9）は、
// PR① の 5.2 のとおりこのファイルに移してある（保有データをまとめる AggregateHoldings はなくなったため）。
//
// 円の単価は必ず10の倍数（0.1円単位）になる。期待値は要件・プランに書かれた値をそのまま使い、テストの中で計算しない。

// 要件定義「成功基準」の銘柄マスタとダミー価格（7203=3,000円・6758=3,500円・AAPL=180.00ドル、どれも 2026-10-06。
// 9984・MSFT は価格なし）。
const (
	toyotaCode = "7203"
	toyotaName = "トヨタ自動車"
	sonyCode   = "6758"
	sonyName   = "ソニーグループ"
	sbgCode    = "9984"
	sbgName    = "ソフトバンクグループ"
	appleCode  = "AAPL"
	appleName  = "Apple"
	msftCode   = "MSFT"
	msftName   = "Microsoft"
	priceDate  = "2026-10-06"
)

// stockInfo は、売買履歴の行に付く、銘柄ごとに決まる値（銘柄名・通貨と最新の終値・基準日。4.6 の TradeRow）。
type stockInfo struct {
	code, name      string
	currency        dto.Currency
	closeHundredths *int64  // 価格が一度も保存されていなければ nil
	priceDate       *string // 同上
}

func ptr[T any](v T) *T { return &v }

// pricedAt は、円の銘柄 code・name の最新の終値が closeHundredths（0.01 単位）・基準日 2026-10-06 の銘柄を返す。
func pricedAt(code, name string, closeHundredths int64) stockInfo {
	return stockInfo{code: code, name: name, currency: dto.CurrencyJPY, closeHundredths: ptr(closeHundredths), priceDate: ptr(priceDate)}
}

// pricedUSDAt は、ドルの銘柄 code・name の最新の終値が closeHundredths（0.01 単位）・基準日 2026-10-06 の銘柄を返す。
func pricedUSDAt(code, name string, closeHundredths int64) stockInfo {
	return stockInfo{code: code, name: name, currency: dto.CurrencyUSD, closeHundredths: ptr(closeHundredths), priceDate: ptr(priceDate)}
}

// unpriced は、価格が一度も保存されていない円の銘柄を返す。
func unpriced(code, name string) stockInfo {
	return stockInfo{code: code, name: name, currency: dto.CurrencyJPY}
}

// unpricedUSD は、価格が一度も保存されていないドルの銘柄を返す。
func unpricedUSD(code, name string) stockInfo {
	return stockInfo{code: code, name: name, currency: dto.CurrencyUSD}
}

// toyota・sony は価格が保存済みの円の銘柄（3,000円・3,500円）、softbank は価格が一度も保存されていない円の銘柄。
// apple は価格が保存済みのドルの銘柄（180.00ドル）、msft は価格が一度も保存されていないドルの銘柄。
func toyota() stockInfo   { return pricedAt(toyotaCode, toyotaName, 300000) }
func sony() stockInfo     { return pricedAt(sonyCode, sonyName, 350000) }
func softbank() stockInfo { return unpriced(sbgCode, sbgName) }
func apple() stockInfo    { return pricedUSDAt(appleCode, appleName, 18000) }
func msft() stockInfo     { return unpricedUSD(msftCode, msftName) }

// row は、この銘柄の売買履歴1件（id は登録した順）を作る。priceHundredths は価格（1株あたり）を 0.01 単位にした値。
func (s stockInfo) row(id int64, side dto.TradeSide, quantity, priceHundredths int64, tradeDate string) dto.TradeRow {
	r := dto.TradeRow{
		ID:              id,
		Code:            s.code,
		Name:            s.name,
		Currency:        s.currency,
		Side:            side,
		Quantity:        quantity,
		PriceHundredths: priceHundredths,
		TradeDate:       tradeDate,
	}
	if s.closeHundredths != nil {
		r.ClosePriceHundredths = ptr(*s.closeHundredths)
	}
	if s.priceDate != nil {
		r.PriceDate = ptr(*s.priceDate)
	}
	return r
}

func (s stockInfo) buy(id, quantity, priceHundredths int64, tradeDate string) dto.TradeRow {
	return s.row(id, dto.TradeSideBuy, quantity, priceHundredths, tradeDate)
}

func (s stockInfo) sell(id, quantity, priceHundredths int64, tradeDate string) dto.TradeRow {
	return s.row(id, dto.TradeSideSell, quantity, priceHundredths, tradeDate)
}

// calculate は CalculateHoldings を呼び、エラーならテストを止める。
func calculate(t *testing.T, rows []dto.TradeRow) dto.HoldingsList {
	t.Helper()
	list, err := query.CalculateHoldings(rows, nil)
	if err != nil {
		t.Fatalf("CalculateHoldings: got err %v, want nil", err)
	}
	return list
}

// codes は、一覧の行の銘柄コードを並び順のまま返す。
func codes(list dto.HoldingsList) []string {
	got := make([]string, 0, len(list.Holdings))
	for _, h := range list.Holdings {
		got = append(got, h.Code)
	}
	return got
}

// onlyHolding は、一覧がちょうど1行で、その銘柄コードが code であることを確かめて、その行を返す。
func onlyHolding(t *testing.T, list dto.HoldingsList, code string) dto.Holding {
	t.Helper()
	if got := codes(list); !slices.Equal(got, []string{code}) {
		t.Fatalf("一覧の銘柄コード: got %v, want [%s]", got, code)
	}
	return list.Holdings[0]
}

// assertQuantityAndAcquisition は、h の保有数量と取得価格（0.01 単位）が want と同じことを確かめる。
func assertQuantityAndAcquisition(t *testing.T, h dto.Holding, wantQuantity, wantAcquisitionHundredths int64) {
	t.Helper()
	if h.Quantity != wantQuantity {
		t.Errorf("保有数量: got %d, want %d", h.Quantity, wantQuantity)
	}
	if h.AcquisitionPriceHundredths != wantAcquisitionHundredths {
		t.Errorf("取得価格（0.01 単位）: got %d, want %d", h.AcquisitionPriceHundredths, wantAcquisitionHundredths)
	}
}

// assertTotals は、list.Totals が want（この並び・この長さ）と同じであることを、含み損益と率（ポインタ）も値で比べて確かめる
// （slices.Equal はポインタをアドレスで比べるので使わない。含み損益・率を比べから外さない。カード表示のプラン 5.2）。
func assertTotals(t *testing.T, list dto.HoldingsList, want []dto.CurrencyTotal) {
	t.Helper()
	if !reflect.DeepEqual(list.Totals, want) {
		t.Errorf("Totals:\n got %s\nwant %s", formatTotals(list.Totals), formatTotals(want))
	}
}

// formatTotals は、失敗のメッセージ用に、合計の含み損益と率（ポインタ）を値で読める形にする。
func formatTotals(totals []dto.CurrencyTotal) string {
	s := "["
	for _, tt := range totals {
		s += fmt.Sprintf("{%s 評価額:%d 注記:%v 含み損益:%s 率:%s} ", tt.Currency, tt.ValuationHundredths, tt.ExcludesUnpriced,
			formatOptional(tt.UnrealizedGainHundredths), formatOptional(tt.UnrealizedGainRateTenths))
	}
	return s + "]"
}

// formatOptional は、ポインタの整数を、nil なら "nil"、そうでなければ値の文字列にする。
func formatOptional(p *int64) string {
	if p == nil {
		return "nil"
	}
	return fmt.Sprintf("%d", *p)
}

// assertGain は、h の含み損益（0.01 単位）と率（0.1% 単位）が want と同じであることを確かめる。
func assertGain(t *testing.T, h dto.Holding, wantGainHundredths, wantRateTenths int64) {
	t.Helper()
	if h.UnrealizedGainHundredths == nil {
		t.Errorf("%s の含み損益: got nil, want %d", h.Code, wantGainHundredths)
	} else if *h.UnrealizedGainHundredths != wantGainHundredths {
		t.Errorf("%s の含み損益（0.01 単位）: got %d, want %d", h.Code, *h.UnrealizedGainHundredths, wantGainHundredths)
	}
	if h.UnrealizedGainRateTenths == nil {
		t.Errorf("%s の率: got nil, want %d", h.Code, wantRateTenths)
	} else if *h.UnrealizedGainRateTenths != wantRateTenths {
		t.Errorf("%s の率（0.1%% 単位）: got %d, want %d", h.Code, *h.UnrealizedGainRateTenths, wantRateTenths)
	}
}

// jpyTotal・usdTotal は、通貨の合計（評価額・注記のフラグ・含み損益と率）を作る。含み損益と率は、価格のある銘柄がなければ nil。
func jpyTotal(valuation int64, excludes bool, gain, rate *int64) dto.CurrencyTotal {
	return dto.CurrencyTotal{
		Currency: dto.CurrencyJPY, ValuationHundredths: valuation, ExcludesUnpriced: excludes,
		UnrealizedGainHundredths: gain, UnrealizedGainRateTenths: rate,
	}
}

func usdTotal(valuation int64, excludes bool, gain, rate *int64) dto.CurrencyTotal {
	return dto.CurrencyTotal{
		Currency: dto.CurrencyUSD, ValuationHundredths: valuation, ExcludesUnpriced: excludes,
		UnrealizedGainHundredths: gain, UnrealizedGainRateTenths: rate,
	}
}

// ---- 要件定義の基準 ----

// 基準1（要件6）: Aの 7203 の売買履歴が「買 2,000円 100株 2026-01-10」「買 3,000円 100株 2026-06-01」「売 3,500円 50株 2026-07-01」
// → 150株・2,500円。さらに「買 4,000円 50株 2026-08-01」を足すと 200株・2,875円（(150×2,500＋50×4,000)÷200）。
func Test基準1_移動平均法で保有数量と取得価格を計算し売りでは取得価格を変えない(t *testing.T) {
	t.Parallel()
	s := toyota()
	rows := []dto.TradeRow{
		s.buy(1, 100, 200000, "2026-01-10"),
		s.buy(2, 100, 300000, "2026-06-01"),
		s.sell(3, 50, 350000, "2026-07-01"),
	}

	h := onlyHolding(t, calculate(t, rows), toyotaCode)
	assertQuantityAndAcquisition(t, h, 150, 250000)
	if h.Name != toyotaName {
		t.Errorf("銘柄名: got %q, want %q", h.Name, toyotaName)
	}
	if h.Currency != dto.CurrencyJPY {
		t.Errorf("通貨: got %q, want %q", h.Currency, dto.CurrencyJPY)
	}

	withMore := append(slices.Clone(rows), s.buy(4, 50, 400000, "2026-08-01"))
	h = onlyHolding(t, calculate(t, withMore), toyotaCode)
	assertQuantityAndAcquisition(t, h, 200, 287500)
}

// 基準2（要件7）: Aの 7203 の売買履歴が「買 2,000円 1株 2026-01-10」「買 2,001円 2株 2026-01-11」
// → 3株・2,000.7円（2,000.666…円の小数第2位を四捨五入）。さらに「買 2,000円 3株 2026-01-12」を足すと 6株・2,000.3円
// （途中の 2,000.666…円を丸めずに使い、12,002÷6＝2,000.333…円を四捨五入。途中で丸めた 2,000.7円を使うと 2,000.4円になってしまう）。
func Test基準2_取得価格は途中で丸めず表示の値だけを四捨五入する(t *testing.T) {
	t.Parallel()
	s := toyota()
	rows := []dto.TradeRow{
		s.buy(1, 1, 200000, "2026-01-10"),
		s.buy(2, 2, 200100, "2026-01-11"),
	}

	h := onlyHolding(t, calculate(t, rows), toyotaCode)
	assertQuantityAndAcquisition(t, h, 3, 200070)

	withMore := append(slices.Clone(rows), s.buy(3, 3, 200000, "2026-01-12"))
	h = onlyHolding(t, calculate(t, withMore), toyotaCode)
	assertQuantityAndAcquisition(t, h, 6, 200030)
}

// 基準3（要件7。PR②）: Aの AAPL の売買履歴が「買 100.00ドル 1株 2026-01-10」「買 100.01ドル 2株 2026-01-11」
// → 3株・100.01ドル（100.00666…ドルの小数第3位を四捨五入。ドルは 0.01ドルに丸める）。
func Test基準3_ドルの取得価格は小数第3位を四捨五入して小数第2位にする(t *testing.T) {
	t.Parallel()
	s := apple()
	rows := []dto.TradeRow{
		s.buy(1, 1, 10000, "2026-01-10"),
		s.buy(2, 2, 10001, "2026-01-11"),
	}

	h := onlyHolding(t, calculate(t, rows), appleCode)

	assertQuantityAndAcquisition(t, h, 3, 10001)
	if h.Currency != dto.CurrencyUSD {
		t.Errorf("通貨: got %q, want %q", h.Currency, dto.CurrencyUSD)
	}
}

// 基準4（要件6・要件7・要件8）: Aの 7203 の売買履歴が「買 2,000円 100株 2026-03-02」「売 2,000円 100株 2026-03-02」
// 「買 3,000円 100株 2026-03-02」「買 1,000円 10株 2026-03-01」（この順に登録）→ 110株・2,900.8円。
// 取引日が古い 2026-03-01 の買いが先（10株・1,000円）、2026-03-02 は登録した順（買いで 110株・1,909.09…円、売りで 10株、買いで 110株・2,900.82…円）。
// 同じ日を買いから先にたどると 2,428.6円 になる。登録した順は id で決まるので、入力の並びを変えても同じ結果になる（プラン 5.1）。
func Test基準4_取引日の古い順にたどり同じ取引日は登録した順にたどる(t *testing.T) {
	t.Parallel()
	s := toyota()
	registered := []dto.TradeRow{
		s.buy(1, 100, 200000, "2026-03-02"),
		s.sell(2, 100, 200000, "2026-03-02"),
		s.buy(3, 100, 300000, "2026-03-02"),
		s.buy(4, 10, 100000, "2026-03-01"),
	}
	cases := []struct {
		name string
		rows []dto.TradeRow
	}{
		{"登録した順に渡す", registered},
		{"逆の並びで渡す", []dto.TradeRow{registered[3], registered[2], registered[1], registered[0]}},
		{"並べ替えて渡す", []dto.TradeRow{registered[2], registered[0], registered[3], registered[1]}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := onlyHolding(t, calculate(t, tc.rows), toyotaCode)

			assertQuantityAndAcquisition(t, h, 110, 290080)
		})
	}
}

// 基準5（要件6）: Aの 7203 の売買履歴が「買 2,000円 100株 2026-01-10」「売 2,500円 100株 2026-02-10」「買 3,000円 50株 2026-03-10」
// → 50株・3,000円（売り切った後の買いは、その買いの価格）。
func Test基準5_売り切った後の買いの取得価格はその買いの価格になる(t *testing.T) {
	t.Parallel()
	s := toyota()
	rows := []dto.TradeRow{
		s.buy(1, 100, 200000, "2026-01-10"),
		s.sell(2, 100, 250000, "2026-02-10"),
		s.buy(3, 50, 300000, "2026-03-10"),
	}

	h := onlyHolding(t, calculate(t, rows), toyotaCode)

	assertQuantityAndAcquisition(t, h, 50, 300000)
}

// 基準12（要件12。PR②）: Aの保有が 7203 だけ（円だけ）→ Totals は円の1件だけ（ドルの行は作らない）。
// 7203 は 100株・終値 3,000円（評価額 300,000円 = 30000000）、価格はあるので ExcludesUnpriced は false。
func Test基準12_保有が円だけなら合計は円の1件だけになる(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{toyota().buy(1, 100, 250000, "2026-01-10")}

	list := calculate(t, rows)

	// 取得価格 2,500円×100株 = 250,000円、評価額 300,000円 → 含み損益 +50,000円（5000000）・率 20.0%（200）
	assertTotals(t, list, []dto.CurrencyTotal{
		jpyTotal(30000000, false, ptr[int64](5000000), ptr[int64](200)),
	})
}

// 基準14（要件15。PR②）: Aの保有が MSFT、9984、AAPL、7203、130A（この順に渡す）→ 130A、7203、9984、AAPL、MSFT の順に並ぶ
// （通貨によらず銘柄コードの文字列としての昇順。数字で始まるコードは英字で始まるコードより先）。
// 130A は銘柄マスタにない銘柄だが、この関数は DB を使わないので、仮の銘柄名の円の銘柄で渡す。
func Test基準14_一覧は通貨によらず銘柄コードの文字列の昇順に並ぶ(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{
		msft().buy(1, 5, 40000, "2026-01-10"),
		softbank().buy(2, 100, 450000, "2026-01-10"),
		apple().buy(3, 10, 15025, "2026-01-10"),
		toyota().buy(4, 100, 250000, "2026-01-10"),
		unpriced("130A", "銘柄130A").buy(5, 10, 100000, "2026-01-10"),
	}

	got := codes(calculate(t, rows))

	if want := []string{"130A", toyotaCode, sbgCode, appleCode, msftCode}; !slices.Equal(got, want) {
		t.Errorf("並び順: got %v, want %v", got, want)
	}
}

// ---- 通貨ごとの合計（要件12・要件13。PR②） ----

// 基準10・基準11 の単体の部分（要件12・要件13）: 7203（100株・終値 3,000円）と AAPL（10株・終値 180.00ドル）→
// Totals は円、ドルの順に1件ずつで、円は 300,000円（30000000）、ドルは 1,800.00ドル（180000）。どちらも価格があるので ExcludesUnpriced は false。
func Test補助_合計は通貨ごとに円ドルの順で出し価格がそろっていれば注記のフラグはどちらもfalse(t *testing.T) {
	t.Parallel()
	// ドルの銘柄を先に渡しても、円、ドルの順になる。
	rows := []dto.TradeRow{
		apple().buy(1, 10, 15025, "2026-01-10"),
		toyota().buy(2, 100, 250000, "2026-01-10"),
	}

	list := calculate(t, rows)

	// 円: +50,000円・20.0%。ドル: 取得額 1,502.50ドル（150.25×10）・評価額 1,800.00ドル → +297.50ドル（29750）・19.8%（198）
	assertTotals(t, list, []dto.CurrencyTotal{
		jpyTotal(30000000, false, ptr[int64](5000000), ptr[int64](200)),
		usdTotal(180000, false, ptr[int64](29750), ptr[int64](198)),
	})
}

// 基準11 の単体の部分（要件13）: 7203（価格あり）と MSFT（価格なし）→ 円の合計は 300,000円で注記なし、
// ドルの合計は 0（0.00ドル）で、価格のない銘柄があるので ExcludesUnpriced は true（通貨ごとに決める）。
func Test補助_価格のない銘柄がある通貨だけ注記のフラグが立ちその通貨の銘柄がすべて価格なしなら合計は0になる(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{
		toyota().buy(1, 100, 250000, "2026-01-10"),
		msft().buy(2, 5, 40000, "2026-01-10"),
	}

	list := calculate(t, rows)

	// 円は +50,000円・20.0%。ドルは価格のある銘柄がないので、含み損益も率も nil（要件16）。
	assertTotals(t, list, []dto.CurrencyTotal{
		jpyTotal(30000000, false, ptr[int64](5000000), ptr[int64](200)),
		usdTotal(0, true, nil, nil),
	})
}

// 5.5: 売り切った銘柄が合計に響かない。7203 を売り切り（価格なし）、AAPL を売り切り（価格あり）、6758 を保有（価格あり）
// → 一覧は 6758 の1行だけで、Totals は円の1件だけ（ExcludesUnpriced は false。ドルの行は作らない）。
func Test補助_売り切った銘柄は合計の通貨にも価格なしの注記にも響かない(t *testing.T) {
	t.Parallel()
	soldOutYen := unpriced(toyotaCode, toyotaName) // 価格は一度も保存されていない
	soldOutUSD := apple()
	rows := []dto.TradeRow{
		soldOutYen.buy(1, 100, 200000, "2026-01-10"),
		soldOutYen.sell(2, 100, 250000, "2026-02-10"),
		soldOutUSD.buy(3, 10, 15025, "2026-01-10"),
		soldOutUSD.sell(4, 10, 17000, "2026-02-10"),
		sony().buy(5, 10, 320000, "2026-01-10"),
	}

	list := calculate(t, rows)

	h := onlyHolding(t, list, sonyCode)
	if h.ValuationHundredths == nil || *h.ValuationHundredths != 3500000 {
		t.Errorf("6758 の評価額: got %v, want 3500000（10株 × 3,500円 = 35,000円）", h.ValuationHundredths)
	}
	// 6758: 取得額 32,000円（3,200円×10株）・評価額 35,000円 → +3,000円（300000）・9.375% を四捨五入して 9.4%（94）
	assertTotals(t, list, []dto.CurrencyTotal{
		jpyTotal(3500000, false, ptr[int64](300000), ptr[int64](94)),
	})
}

// 5.5: ドルだけ。AAPL だけを保有 → Totals はドルの1件だけ（円の行は作らない）。10株・終値 180.00ドル → 1,800.00ドル（180000）。
func Test補助_保有がドルだけなら合計はドルの1件だけになる(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{apple().buy(1, 10, 15025, "2026-01-10")}

	list := calculate(t, rows)

	h := onlyHolding(t, list, appleCode)
	assertQuantityAndAcquisition(t, h, 10, 15025)
	if h.CurrentPriceHundredths == nil || *h.CurrentPriceHundredths != 18000 {
		t.Errorf("AAPL の現在の価格: got %v, want 18000（180.00ドル）", h.CurrentPriceHundredths)
	}
	assertTotals(t, list, []dto.CurrencyTotal{
		usdTotal(180000, false, ptr[int64](29750), ptr[int64](198)),
	})
}

// 5.5: ドルを切り捨てない。終値 180.25ドル・3株 → 評価額 540.75ドル（54075。円のように1円未満を切り捨てない。要件11）。合計も 54075。
func Test補助_ドルの評価額は切り捨てずそのままの値になる(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{pricedUSDAt(appleCode, appleName, 18025).buy(1, 3, 15025, "2026-01-10")}

	list := calculate(t, rows)

	h := onlyHolding(t, list, appleCode)
	if h.ValuationHundredths == nil || *h.ValuationHundredths != 54075 {
		t.Errorf("AAPL の評価額: got %v, want 54075（3株 × 180.25ドル = 540.75ドル）", h.ValuationHundredths)
	}
	// 取得額 450.75ドル（150.25×3）・評価額 540.75ドル → +90.00ドル（9000）・19.967…% を四捨五入して 20.0%（200）
	assertTotals(t, list, []dto.CurrencyTotal{
		usdTotal(54075, false, ptr[int64](9000), ptr[int64](200)),
	})
}

// 5.3: 四捨五入のちょうど半分。
//   - 円: 2,000.6円を1株と 2,000.7円を1株 → 平均 2,000.65円 → 2,000.7円（200070）
//   - ドル: 100.00ドルを1株と 100.01ドルを1株 → 平均 100.005ドル → 100.01ドル（10001）
func Test補助_取得価格の四捨五入はちょうど半分なら切り上げる(t *testing.T) {
	t.Parallel()

	t.Run("円は2000.65円が2000.7円になる", func(t *testing.T) {
		t.Parallel()
		s := toyota()
		rows := []dto.TradeRow{
			s.buy(1, 1, 200060, "2026-01-10"),
			s.buy(2, 1, 200070, "2026-01-11"),
		}

		h := onlyHolding(t, calculate(t, rows), toyotaCode)

		assertQuantityAndAcquisition(t, h, 2, 200070)
	})

	t.Run("ドルは100.005ドルが100.01ドルになる", func(t *testing.T) {
		t.Parallel()
		s := apple()
		rows := []dto.TradeRow{
			s.buy(1, 1, 10000, "2026-01-10"),
			s.buy(2, 1, 10001, "2026-01-11"),
		}

		h := onlyHolding(t, calculate(t, rows), appleCode)

		assertQuantityAndAcquisition(t, h, 2, 10001)
	})
}

// ---- 保有株式一覧の既存のテストの移し先（PR① の 5.2） ----

// AC-6（FR-5）: 9984、7203、6758 の順の行 → 6758、7203、9984 の順に並ぶ。
func TestAC6_一覧は銘柄コードの昇順に並ぶ(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{
		softbank().buy(1, 10, 900000, "2026-01-10"),
		toyota().buy(2, 100, 250000, "2026-01-10"),
		sony().buy(3, 10, 320000, "2026-01-10"),
	}

	got := codes(calculate(t, rows))

	if want := []string{sonyCode, toyotaCode, sbgCode}; !slices.Equal(got, want) {
		t.Errorf("並び順: got %v, want %v", got, want)
	}
}

// 補助 FR-5（TC-17）: 130A・1301・7203 の行 → 1301、130A、7203 の順（文字列としての昇順。数字の「1」は英字の「A」より前）。
// 1301・130A は銘柄マスタにない銘柄だが、この関数は DB を使わないので、仮の銘柄名で渡す。
func TestFR5_TC17_英字を含む銘柄コードも文字列としての昇順に並ぶ(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{
		unpriced("130A", "銘柄130A").buy(1, 10, 100000, "2026-01-10"),
		unpriced("1301", "銘柄1301").buy(2, 10, 100000, "2026-01-10"),
		toyota().buy(3, 10, 250000, "2026-01-10"),
	}

	got := codes(calculate(t, rows))

	if want := []string{"1301", "130A", toyotaCode}; !slices.Equal(got, want) {
		t.Errorf("並び順: got %v, want %v", got, want)
	}
}

// AC-10（FR-9）: 現在の価格 1,234.5円（Hundredths で 123450）、保有数量3株 → 評価額は 3,703円（3,703.5円の1円未満を切り捨てた 370300）。
func TestAC10_評価額は現在の価格と保有数量の積の1円未満を切り捨てる(t *testing.T) {
	t.Parallel()
	s := pricedAt(toyotaCode, toyotaName, 123450)
	rows := []dto.TradeRow{s.buy(1, 3, 200000, "2026-01-10")}

	h := onlyHolding(t, calculate(t, rows), toyotaCode)

	if h.ValuationHundredths == nil {
		t.Fatal("評価額: got nil, want 370300（価格のある銘柄）")
	}
	if *h.ValuationHundredths != 370300 {
		t.Errorf("評価額（0.01 単位）: got %d, want 370300（3,703円）", *h.ValuationHundredths)
	}
}

// 補助 FR-4・FR-9（SR-4、TC-12）: 7203 を1株＋1株、価格 1,234.5円（Hundredths で 123450）→ 1行・2株・評価額 2,469円（246900）。
// 評価額は、買いが2件でも、保有数量（2株）で1回だけ計算する（買いごとに切り捨ててから足すと 1,234 + 1,234 = 2,468円 になり、誤り）。
func TestFR4_FR9_SR4_TC12_評価額は買いごとではなく保有数量で1回だけ計算する(t *testing.T) {
	t.Parallel()
	s := pricedAt(toyotaCode, toyotaName, 123450)
	rows := []dto.TradeRow{
		s.buy(1, 1, 200000, "2026-01-10"),
		s.buy(2, 1, 200000, "2026-01-11"),
	}

	h := onlyHolding(t, calculate(t, rows), toyotaCode)

	if h.Quantity != 2 {
		t.Errorf("保有数量: got %d, want 2", h.Quantity)
	}
	if h.ValuationHundredths == nil {
		t.Fatal("評価額: got nil, want 246900（価格のある銘柄）")
	}
	if *h.ValuationHundredths != 246900 {
		t.Errorf("評価額（0.01 単位）: got %d, want 246900（2,469円）", *h.ValuationHundredths)
	}
}

// 補助 FR-6・FR-9（TC-12）: 7203 を1株、6758 を1株、どちらも価格 1,234.5円 → 円の合計評価額 2,468円（246800。行ごとに切り捨てた評価額 1,234円の合計。
// 全体を掛けてから切り捨てると 2,469円 になり、誤り）。
func TestFR6_FR9_TC12_合計評価額は行ごとに切り捨てた評価額の合計(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{
		pricedAt(toyotaCode, toyotaName, 123450).buy(1, 1, 200000, "2026-01-10"),
		pricedAt(sonyCode, sonyName, 123450).buy(2, 1, 200000, "2026-01-10"),
	}

	list := calculate(t, rows)

	// 1行ごと: 取得額 2,000円・評価額 1,234円 → -766円。2行の合計: -1,532円（-153200）・-1,532÷4,000 = -38.3%（-383）
	assertTotals(t, list, []dto.CurrencyTotal{
		jpyTotal(246800, false, ptr[int64](-153200), ptr[int64](-383)),
	})
}

// 補助 FR-4（PR② の EC-2、TC-02）: 取得価格の四捨五入の境界（小数第2位がちょうど 5 になる値と、その手前）。
//   - {1株・200000、1株・200010} → 200010（平均 2,000.05円。5 は切り上げる）
//   - {1株・200000、1株・200030} → 200020（平均 2,000.15円。5 は切り上げる）
//   - {2株・200000、1株・200010} → 200000（平均 2,000.033…円。4 以下は切り捨てる）
func TestFR4_TC02_取得価格は小数第2位がちょうど5なら切り上げそれ未満なら切り捨てる(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                string
		firstQty, secondQty int64
		firstHun, secondHun int64
		want                int64
	}{
		{"1株200000と1株200010は200010", 1, 1, 200000, 200010, 200010},
		{"1株200000と1株200030は200020", 1, 1, 200000, 200030, 200020},
		{"2株200000と1株200010は200000", 2, 1, 200000, 200010, 200000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := toyota()
			rows := []dto.TradeRow{
				s.buy(1, tc.firstQty, tc.firstHun, "2026-01-10"),
				s.buy(2, tc.secondQty, tc.secondHun, "2026-01-11"),
			}

			h := onlyHolding(t, calculate(t, rows), toyotaCode)

			if h.AcquisitionPriceHundredths != tc.want {
				t.Errorf("取得価格（0.01 単位）: got %d, want %d", h.AcquisitionPriceHundredths, tc.want)
			}
		})
	}
}

// ---- 補助のテスト（プラン 5.3） ----

// 5.3: 売買履歴が0件 → Holdings・Totals は空のスライス（nil ではない）。
// 0件は、nil のスライスと長さ0のスライスのどちらで渡されても同じ結果になる。
func Test補助_売買履歴が0件なら一覧と合計は空のスライスになる(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		rows []dto.TradeRow
	}{
		{"nilのスライス", nil},
		{"長さ0のスライス", []dto.TradeRow{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			list := calculate(t, tc.rows)

			if list.Holdings == nil {
				t.Error("Holdings: got nil, want 空のスライス（nil ではない）")
			}
			if len(list.Holdings) != 0 {
				t.Errorf("Holdings の件数: got %d, want 0", len(list.Holdings))
			}
			if list.Totals == nil {
				t.Error("Totals: got nil, want 空のスライス（nil ではない）")
			}
			if len(list.Totals) != 0 {
				t.Errorf("Totals の件数: got %d, want 0", len(list.Totals))
			}
		})
	}
}

// 基準6（要件10）の単体の部分: 7203 を売り切り、6758 を保有する → 一覧は 6758 の1行だけ。
// 売り切った 7203 は価格がなくても、合計評価額に入れず、「価格を取得できない銘柄を含みません」のフラグ（ExcludesUnpriced）も立てない
// （5.5: 価格のない売り切りの銘柄は注記を出さない）。合計は円の1件（35,000円 = 3500000）。
func Test補助_売り切った銘柄は一覧に出さず合計評価額にも価格なしの注記にも響かない(t *testing.T) {
	t.Parallel()
	soldOut := unpriced(toyotaCode, toyotaName) // 価格は一度も保存されていない
	rows := []dto.TradeRow{
		soldOut.buy(1, 100, 200000, "2026-01-10"),
		soldOut.sell(2, 100, 250000, "2026-02-10"),
		sony().buy(3, 10, 320000, "2026-01-10"),
	}

	list := calculate(t, rows)

	h := onlyHolding(t, list, sonyCode)
	if h.ValuationHundredths == nil || *h.ValuationHundredths != 3500000 {
		t.Errorf("6758 の評価額: got %v, want 3500000（10株 × 3,500円）", h.ValuationHundredths)
	}
	assertTotals(t, list, []dto.CurrencyTotal{
		jpyTotal(3500000, false, ptr[int64](300000), ptr[int64](94)),
	})
}

// 5.3: 売り切った銘柄だけ → 一覧と合計は空のスライス。
func Test補助_売り切った銘柄だけなら一覧も合計も空になる(t *testing.T) {
	t.Parallel()
	s := toyota()
	rows := []dto.TradeRow{
		s.buy(1, 100, 200000, "2026-01-10"),
		s.sell(2, 100, 250000, "2026-02-10"),
	}

	list := calculate(t, rows)

	if list.Holdings == nil || len(list.Holdings) != 0 {
		t.Errorf("Holdings: got %v, want 空のスライス（nil ではない）", list.Holdings)
	}
	if list.Totals == nil || len(list.Totals) != 0 {
		t.Errorf("Totals: got %v, want 空のスライス（nil ではない）", list.Totals)
	}
}

// 5.3: 価格のない銘柄は合計評価額に入れない。7203（価格あり）と 9984（価格なし）→ 円の合計は 7203 の 300,000円（30000000）だけ、
// ExcludesUnpriced は true。9984 の現在の価格・基準日・評価額は nil。
func Test補助_価格のない銘柄は合計評価額に入れず注記のフラグを立てる(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{
		toyota().buy(1, 100, 250000, "2026-01-10"),
		softbank().buy(2, 10, 900000, "2026-01-10"),
	}

	list := calculate(t, rows)

	if got, want := codes(list), []string{toyotaCode, sbgCode}; !slices.Equal(got, want) {
		t.Fatalf("銘柄コード: got %v, want %v", got, want)
	}
	// 含み損益と率は、価格のある 7203 だけで計算する（9984 は入れない）。+50,000円・20.0%。
	assertTotals(t, list, []dto.CurrencyTotal{
		jpyTotal(30000000, true, ptr[int64](5000000), ptr[int64](200)),
	})
	noPrice := list.Holdings[1]
	if noPrice.CurrentPriceHundredths != nil || noPrice.PriceDate != nil || noPrice.ValuationHundredths != nil {
		t.Errorf("価格のない銘柄の現在の価格・基準日・評価額: got %v・%v・%v, want すべて nil",
			noPrice.CurrentPriceHundredths, noPrice.PriceDate, noPrice.ValuationHundredths)
	}
}

// negativeCase は、たどる途中で保有数量が負になる売買履歴の例。
type negativeCase struct {
	name string
	rows []dto.TradeRow
}

// negativeCases は、プラン 5.3・判断6 の「途中で保有数量が負になる」例。
//   - 持っていない株を売る（買いより前に売りがある）
//   - 同じ取引日では登録した順にたどるので、売り（id 1）が買い（id 2）より先なら、同じ日に買っていても負になる
//   - 最後は0株に戻っても、途中で負になれば（売り → 買いの順）エラー
//   - 売った株数が持っている株数より多い
func negativeCases() []negativeCase {
	s := toyota()
	return []negativeCase{
		{"持っていない株を売る", []dto.TradeRow{
			s.sell(1, 10, 250000, "2026-01-10"),
		}},
		{"同じ取引日で売りが先", []dto.TradeRow{
			s.sell(1, 100, 250000, "2026-01-10"),
			s.buy(2, 100, 200000, "2026-01-10"),
		}},
		{"最後は0に戻るが途中で負になる", []dto.TradeRow{
			s.sell(1, 100, 250000, "2026-01-10"),
			s.buy(2, 100, 200000, "2026-02-10"),
		}},
		{"持っている株数より多く売る", []dto.TradeRow{
			s.buy(1, 100, 200000, "2026-01-10"),
			s.sell(2, 101, 250000, "2026-02-10"),
		}},
	}
}

// 5.3・判断6: 売買履歴をたどる途中で保有数量が負になる銘柄があれば ErrNegativeQuantity（誤った一覧を返さない）。
func Test補助_途中で保有数量が負になる売買履歴はErrNegativeQuantityになる(t *testing.T) {
	t.Parallel()
	for _, tc := range negativeCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := query.CalculateHoldings(tc.rows, nil)

			if !errors.Is(err, query.ErrNegativeQuantity) {
				t.Errorf("CalculateHoldings: got err %v, want ErrNegativeQuantity", err)
			}
		})
	}
}

// 判断6: 正しい銘柄（6758）と、途中で保有数量が負になる銘柄（7203。持っていない株を売る）が混ざるとき、
// 正しい銘柄の行だけを返さず、ErrNegativeQuantity を返す（誤った一覧を返さない）。
func Test補助_正しい銘柄と負になる銘柄が混ざるときも一覧を返さずErrNegativeQuantityになる(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{
		sony().buy(1, 10, 320000, "2026-01-10"),
		toyota().sell(2, 10, 250000, "2026-01-10"),
	}

	_, err := query.CalculateHoldings(rows, nil)

	if !errors.Is(err, query.ErrNegativeQuantity) {
		t.Errorf("CalculateHoldings: got err %v, want ErrNegativeQuantity（正しい銘柄の行だけを返さない）", err)
	}
}

// overflowCase は、最後の値が int64 に収まらない売買履歴の例。
type overflowCase struct {
	name string
	rows []dto.TradeRow
}

// overflowCases は、プラン 5.3・判断5 の3つの例と、int64 の境界の1つ上（ぎりぎり収まる側は Test補助_int64にぎりぎり収まる値ならエラーにならない）。
// 値はどれも 0.01 単位の整数（int64 に収まるかは、この単位の値で見る。4.3 の 6）。
//  1. 終値 3,000円で 10^18 株（評価額があふれる）
//  2. 5×10^18 株の買い2件（保有数量があふれる）
//  3. 行ごとには収まり、合計評価額だけがあふれる2銘柄（どちらも終値 0.1円・5×10^17 株で評価額 5×10^18。合計 10^19）
//  4. 境界の1つ上: 保有数量が 2^63（2^62 株の買い2件）。終値 1.1円で int64 の最大の株数（評価額があふれる）
//
// 価格は 0.1円（Hundredths で 10。円の最小の単位）とした（取得価格の値は結果に関係しない）。
func overflowCases() []overflowCase {
	return []overflowCase{
		{"1_終値3000円で10の18乗株は評価額があふれる", []dto.TradeRow{
			toyota().buy(1, 1000000000000000000, 10, "2026-01-10"),
		}},
		{"2_5かける10の18乗株の買い2件は保有数量があふれる", []dto.TradeRow{
			toyota().buy(1, 5000000000000000000, 10, "2026-01-10"),
			toyota().buy(2, 5000000000000000000, 10, "2026-01-11"),
		}},
		{"3_行ごとには収まり合計評価額だけがあふれる2銘柄", []dto.TradeRow{
			pricedAt(toyotaCode, toyotaName, 10).buy(1, 500000000000000000, 10, "2026-01-10"),
			pricedAt(sonyCode, sonyName, 10).buy(2, 500000000000000000, 10, "2026-01-10"),
		}},
		{"4a_境界の1つ上_保有数量が2の63乗", []dto.TradeRow{
			toyota().buy(1, 4611686018427387904, 10, "2026-01-10"),
			toyota().buy(2, 4611686018427387904, 10, "2026-01-11"),
		}},
		{"4b_境界の1つ上_int64の最大の株数で終値1.1円なら評価額があふれる", []dto.TradeRow{
			pricedAt(toyotaCode, toyotaName, 110).buy(1, 9223372036854775807, 10, "2026-01-10"),
		}},
	}
}

// 5.3・判断5: 最後の値（保有数量・評価額・合計評価額）が int64 に収まらなければ ErrCalculationOverflow（包んで返す）。
func Test補助_最後の値がint64に収まらなければErrCalculationOverflowになる(t *testing.T) {
	t.Parallel()
	for _, tc := range overflowCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := query.CalculateHoldings(tc.rows, nil)

			if !errors.Is(err, query.ErrCalculationOverflow) {
				t.Errorf("CalculateHoldings: got err %v, want ErrCalculationOverflow", err)
			}
		})
	}
}

// 5.3・判断5（境界の対照）: ぎりぎり収まる値ならエラーにならず、正しい値を返す（黙って桁あふれした値を返さない）。
//   - 2^62 − 1 株と 2^62 株の買い（保有数量はちょうど int64 の最大 9223372036854775807）
//   - int64 の最大の株数を終値 0.01ドルで持つ（評価額はちょうど int64 の最大 9223372036854775807。0.01 単位の 1 × 最大の株数。
//     円は最小の単価が 0.1円（10）なので、この境界はドルの銘柄で確かめる）
func Test補助_int64にぎりぎり収まる値ならエラーにならない(t *testing.T) {
	t.Parallel()

	// 価格のない銘柄で確かめる（終値があると、評価額が先にあふれて保有数量の境界を見分けられないため）。
	t.Run("保有数量がint64の最大", func(t *testing.T) {
		t.Parallel()
		s := unpriced(toyotaCode, toyotaName)
		rows := []dto.TradeRow{
			s.buy(1, 4611686018427387903, 10, "2026-01-10"),
			s.buy(2, 4611686018427387904, 10, "2026-01-11"),
		}

		h := onlyHolding(t, calculate(t, rows), toyotaCode)

		if h.Quantity != 9223372036854775807 {
			t.Errorf("保有数量: got %d, want 9223372036854775807", h.Quantity)
		}
	})

	t.Run("評価額がint64の最大", func(t *testing.T) {
		t.Parallel()
		s := pricedUSDAt(appleCode, appleName, 1)
		rows := []dto.TradeRow{s.buy(1, 9223372036854775807, 1, "2026-01-10")}

		list := calculate(t, rows)

		h := onlyHolding(t, list, appleCode)
		if h.ValuationHundredths == nil || *h.ValuationHundredths != 9223372036854775807 {
			t.Errorf("評価額: got %v, want 9223372036854775807", h.ValuationHundredths)
		}
		// 取得額（0.01ドル × 最大の株数）と評価額が同じなので、含み損益は 0・率も 0。
		assertTotals(t, list, []dto.CurrencyTotal{
			usdTotal(9223372036854775807, false, ptr[int64](0), ptr[int64](0)),
		})
	})
}
