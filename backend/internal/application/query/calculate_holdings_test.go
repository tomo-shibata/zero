package query_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/application/query"
	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// 仕様: 要件（docs/requirements/trade-history.md）の基準1・基準2・基準4・基準5（保有の計算。要件6・要件7・要件8）と、
// プラン（docs/plans/trade-history/plan.md）4.3（保有の計算）・4.6（PR① の境界）・5.1（PR① の行）・5.2（既存のテストの直し）・
// 5.3（補助のテスト。PR①）。確かめる関数は 4.6 の query.CalculateHoldings（円だけ・0.1円単位 Tenths のまま。判断13）。
// 保有株式一覧の AC-6（並び）・AC-10（評価額の切り捨て）と、集約の補助のテスト（FR-4・FR-5・FR-6・FR-9）は、
// 5.2 のとおりこのファイルに移した（保有データをまとめる AggregateHoldings はなくなったため）。
//
// 単価は 0.1円単位の整数（名前の末尾が Tenths。2,500円 = 25000）、評価額は円の整数。
// 期待値は要件・プランに書かれた値をそのまま使い、テストの中で計算しない。

// 要件定義「成功基準」の銘柄マスタとダミー価格（7203=3,000円・6758=3,500円、どちらも 2026-10-06。9984 は価格なし）。
const (
	toyotaCode = "7203"
	toyotaName = "トヨタ自動車"
	sonyCode   = "6758"
	sonyName   = "ソニーグループ"
	sbgCode    = "9984"
	sbgName    = "ソフトバンクグループ"
	priceDate  = "2026-10-06"
)

// stockInfo は、売買履歴の行に付く、銘柄ごとに決まる値（銘柄名と最新の終値・基準日。4.6 の TradeRow）。
type stockInfo struct {
	code, name  string
	closeTenths *int64  // 価格が一度も保存されていなければ nil
	priceDate   *string // 同上
}

func ptr[T any](v T) *T { return &v }

// pricedAt は、銘柄 code・name の最新の終値が closeTenths（0.1円単位）・基準日 2026-10-06 の銘柄を返す。
func pricedAt(code, name string, closeTenths int64) stockInfo {
	return stockInfo{code: code, name: name, closeTenths: ptr(closeTenths), priceDate: ptr(priceDate)}
}

// unpriced は、価格が一度も保存されていない銘柄を返す。
func unpriced(code, name string) stockInfo {
	return stockInfo{code: code, name: name}
}

// toyota・sony は価格が保存済みの銘柄（3,000円・3,500円）、softbank は価格が一度も保存されていない銘柄。
func toyota() stockInfo   { return pricedAt(toyotaCode, toyotaName, 30000) }
func sony() stockInfo     { return pricedAt(sonyCode, sonyName, 35000) }
func softbank() stockInfo { return unpriced(sbgCode, sbgName) }

// row は、この銘柄の売買履歴1件（id は登録した順）を作る。priceTenths は価格（1株あたり）を 0.1円単位にした値。
func (s stockInfo) row(id int64, side dto.TradeSide, quantity, priceTenths int64, tradeDate string) dto.TradeRow {
	r := dto.TradeRow{
		ID:          id,
		Code:        s.code,
		Name:        s.name,
		Side:        side,
		Quantity:    quantity,
		PriceTenths: priceTenths,
		TradeDate:   tradeDate,
	}
	if s.closeTenths != nil {
		r.ClosePriceTenths = ptr(*s.closeTenths)
	}
	if s.priceDate != nil {
		r.PriceDate = ptr(*s.priceDate)
	}
	return r
}

func (s stockInfo) buy(id, quantity, priceTenths int64, tradeDate string) dto.TradeRow {
	return s.row(id, dto.TradeSideBuy, quantity, priceTenths, tradeDate)
}

func (s stockInfo) sell(id, quantity, priceTenths int64, tradeDate string) dto.TradeRow {
	return s.row(id, dto.TradeSideSell, quantity, priceTenths, tradeDate)
}

// calculate は CalculateHoldings を呼び、エラーならテストを止める。
func calculate(t *testing.T, rows []dto.TradeRow) dto.HoldingsList {
	t.Helper()
	list, err := query.CalculateHoldings(rows)
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

// assertQuantityAndAcquisition は、h の保有数量と取得価格（0.1円単位）が want と同じことを確かめる。
func assertQuantityAndAcquisition(t *testing.T, h dto.Holding, wantQuantity, wantAcquisitionTenths int64) {
	t.Helper()
	if h.Quantity != wantQuantity {
		t.Errorf("保有数量: got %d, want %d", h.Quantity, wantQuantity)
	}
	if h.AcquisitionPriceTenths != wantAcquisitionTenths {
		t.Errorf("取得価格（0.1円単位）: got %d, want %d", h.AcquisitionPriceTenths, wantAcquisitionTenths)
	}
}

// ---- 要件定義の基準 ----

// 基準1（要件6）: Aの 7203 の売買履歴が「買 2,000円 100株 2026-01-10」「買 3,000円 100株 2026-06-01」「売 3,500円 50株 2026-07-01」
// → 150株・2,500円。さらに「買 4,000円 50株 2026-08-01」を足すと 200株・2,875円（(150×2,500＋50×4,000)÷200）。
func Test基準1_移動平均法で保有数量と取得価格を計算し売りでは取得価格を変えない(t *testing.T) {
	t.Parallel()
	s := toyota()
	rows := []dto.TradeRow{
		s.buy(1, 100, 20000, "2026-01-10"),
		s.buy(2, 100, 30000, "2026-06-01"),
		s.sell(3, 50, 35000, "2026-07-01"),
	}

	h := onlyHolding(t, calculate(t, rows), toyotaCode)
	assertQuantityAndAcquisition(t, h, 150, 25000)
	if h.Name != toyotaName {
		t.Errorf("銘柄名: got %q, want %q", h.Name, toyotaName)
	}

	withMore := append(slices.Clone(rows), s.buy(4, 50, 40000, "2026-08-01"))
	h = onlyHolding(t, calculate(t, withMore), toyotaCode)
	assertQuantityAndAcquisition(t, h, 200, 28750)
}

// 基準2（要件7）: Aの 7203 の売買履歴が「買 2,000円 1株 2026-01-10」「買 2,001円 2株 2026-01-11」
// → 3株・2,000.7円（2,000.666…円の小数第2位を四捨五入）。さらに「買 2,000円 3株 2026-01-12」を足すと 6株・2,000.3円
// （途中の 2,000.666…円を丸めずに使い、12,002÷6＝2,000.333…円を四捨五入。途中で丸めた 2,000.7円を使うと 2,000.4円になってしまう）。
func Test基準2_取得価格は途中で丸めず表示の値だけを四捨五入する(t *testing.T) {
	t.Parallel()
	s := toyota()
	rows := []dto.TradeRow{
		s.buy(1, 1, 20000, "2026-01-10"),
		s.buy(2, 2, 20010, "2026-01-11"),
	}

	h := onlyHolding(t, calculate(t, rows), toyotaCode)
	assertQuantityAndAcquisition(t, h, 3, 20007)

	withMore := append(slices.Clone(rows), s.buy(3, 3, 20000, "2026-01-12"))
	h = onlyHolding(t, calculate(t, withMore), toyotaCode)
	assertQuantityAndAcquisition(t, h, 6, 20003)
}

// 基準4（要件6・要件7・要件8）: Aの 7203 の売買履歴が「買 2,000円 100株 2026-03-02」「売 2,000円 100株 2026-03-02」
// 「買 3,000円 100株 2026-03-02」「買 1,000円 10株 2026-03-01」（この順に登録）→ 110株・2,900.8円。
// 取引日が古い 2026-03-01 の買いが先（10株・1,000円）、2026-03-02 は登録した順（買いで 110株・1,909.09…円、売りで 10株、買いで 110株・2,900.82…円）。
// 同じ日を買いから先にたどると 2,428.6円 になる。登録した順は id で決まるので、入力の並びを変えても同じ結果になる（プラン 5.1）。
func Test基準4_取引日の古い順にたどり同じ取引日は登録した順にたどる(t *testing.T) {
	t.Parallel()
	s := toyota()
	registered := []dto.TradeRow{
		s.buy(1, 100, 20000, "2026-03-02"),
		s.sell(2, 100, 20000, "2026-03-02"),
		s.buy(3, 100, 30000, "2026-03-02"),
		s.buy(4, 10, 10000, "2026-03-01"),
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

			assertQuantityAndAcquisition(t, h, 110, 29008)
		})
	}
}

// 基準5（要件6）: Aの 7203 の売買履歴が「買 2,000円 100株 2026-01-10」「売 2,500円 100株 2026-02-10」「買 3,000円 50株 2026-03-10」
// → 50株・3,000円（売り切った後の買いは、その買いの価格）。
func Test基準5_売り切った後の買いの取得価格はその買いの価格になる(t *testing.T) {
	t.Parallel()
	s := toyota()
	rows := []dto.TradeRow{
		s.buy(1, 100, 20000, "2026-01-10"),
		s.sell(2, 100, 25000, "2026-02-10"),
		s.buy(3, 50, 30000, "2026-03-10"),
	}

	h := onlyHolding(t, calculate(t, rows), toyotaCode)

	assertQuantityAndAcquisition(t, h, 50, 30000)
}

// ---- 保有株式一覧の既存のテストの移し先（5.2） ----

// AC-6（FR-5）: 9984、7203、6758 の順の行 → 6758、7203、9984 の順に並ぶ。
func TestAC6_一覧は銘柄コードの昇順に並ぶ(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{
		softbank().buy(1, 10, 90000, "2026-01-10"),
		toyota().buy(2, 100, 25000, "2026-01-10"),
		sony().buy(3, 10, 32000, "2026-01-10"),
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
		unpriced("130A", "銘柄130A").buy(1, 10, 10000, "2026-01-10"),
		unpriced("1301", "銘柄1301").buy(2, 10, 10000, "2026-01-10"),
		toyota().buy(3, 10, 25000, "2026-01-10"),
	}

	got := codes(calculate(t, rows))

	if want := []string{"1301", "130A", toyotaCode}; !slices.Equal(got, want) {
		t.Errorf("並び順: got %v, want %v", got, want)
	}
}

// AC-10（FR-9）: 現在の価格 1,234.5円（Tenths で 12345）、保有数量3株 → 評価額は 3,703円（3,703.5円の1円未満を切り捨て）。
func TestAC10_評価額は現在の価格と保有数量の積の1円未満を切り捨てる(t *testing.T) {
	t.Parallel()
	s := pricedAt(toyotaCode, toyotaName, 12345)
	rows := []dto.TradeRow{s.buy(1, 3, 20000, "2026-01-10")}

	h := onlyHolding(t, calculate(t, rows), toyotaCode)

	if h.Valuation == nil {
		t.Fatal("評価額: got nil, want 3703（価格のある銘柄）")
	}
	if *h.Valuation != 3703 {
		t.Errorf("評価額: got %d, want 3703", *h.Valuation)
	}
}

// 補助 FR-4・FR-9（SR-4、TC-12）: 7203 を1株＋1株、価格 1,234.5円（Tenths で 12345）→ 1行・2株・評価額 2,469円。
// 評価額は、買いが2件でも、保有数量（2株）で1回だけ計算する（買いごとに切り捨ててから足すと 1,234 + 1,234 = 2,468円 になり、誤り）。
func TestFR4_FR9_SR4_TC12_評価額は買いごとではなく保有数量で1回だけ計算する(t *testing.T) {
	t.Parallel()
	s := pricedAt(toyotaCode, toyotaName, 12345)
	rows := []dto.TradeRow{
		s.buy(1, 1, 20000, "2026-01-10"),
		s.buy(2, 1, 20000, "2026-01-11"),
	}

	h := onlyHolding(t, calculate(t, rows), toyotaCode)

	if h.Quantity != 2 {
		t.Errorf("保有数量: got %d, want 2", h.Quantity)
	}
	if h.Valuation == nil {
		t.Fatal("評価額: got nil, want 2469（価格のある銘柄）")
	}
	if *h.Valuation != 2469 {
		t.Errorf("評価額: got %d, want 2469", *h.Valuation)
	}
}

// 補助 FR-6・FR-9（TC-12）: 7203 を1株、6758 を1株、どちらも価格 1,234.5円 → 合計評価額 2,468円（行ごとに切り捨てた評価額 1,234円の合計。
// 全体を掛けてから切り捨てると 2,469円 になり、誤り）。
func TestFR6_FR9_TC12_合計評価額は行ごとに切り捨てた評価額の合計(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{
		pricedAt(toyotaCode, toyotaName, 12345).buy(1, 1, 20000, "2026-01-10"),
		pricedAt(sonyCode, sonyName, 12345).buy(2, 1, 20000, "2026-01-10"),
	}

	list := calculate(t, rows)

	if list.TotalValuation != 2468 {
		t.Errorf("合計評価額: got %d, want 2468", list.TotalValuation)
	}
}

// 補助 FR-4（PR② の EC-2、TC-02）: 取得価格の四捨五入の境界（小数第2位がちょうど 5 になる値と、その手前）。
//   - {1株・20000、1株・20001} → 20001（平均 2,000.05円。5 は切り上げる）
//   - {1株・20000、1株・20003} → 20002（平均 2,000.15円。5 は切り上げる）
//   - {2株・20000、1株・20001} → 20000（平均 2,000.033…円。4 以下は切り捨てる）
func TestFR4_TC02_取得価格は小数第2位がちょうど5なら切り上げそれ未満なら切り捨てる(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                string
		firstQty, secondQty int64
		firstTen, secondTen int64
		want                int64
	}{
		{"1株20000と1株20001は20001", 1, 1, 20000, 20001, 20001},
		{"1株20000と1株20003は20002", 1, 1, 20000, 20003, 20002},
		{"2株20000と1株20001は20000", 2, 1, 20000, 20001, 20000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := toyota()
			rows := []dto.TradeRow{
				s.buy(1, tc.firstQty, tc.firstTen, "2026-01-10"),
				s.buy(2, tc.secondQty, tc.secondTen, "2026-01-11"),
			}

			h := onlyHolding(t, calculate(t, rows), toyotaCode)

			if h.AcquisitionPriceTenths != tc.want {
				t.Errorf("取得価格（0.1円単位）: got %d, want %d", h.AcquisitionPriceTenths, tc.want)
			}
		})
	}
}

// ---- 補助のテスト（プラン 5.3。PR①） ----

// 5.3: 売買履歴が0件 → Holdings は空のスライス（nil ではない）。合計評価額は 0、注記のフラグは false。
// 0件は、nil のスライスと長さ0のスライスのどちらで渡されても同じ結果になる。
func Test補助_売買履歴が0件なら一覧は空のスライスになる(t *testing.T) {
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
			if list.TotalValuation != 0 {
				t.Errorf("合計評価額: got %d, want 0", list.TotalValuation)
			}
			if list.TotalExcludesUnpriced {
				t.Error("TotalExcludesUnpriced: got true, want false（価格のない行がない）")
			}
		})
	}
}

// 基準6（要件10）の単体の部分: 7203 を売り切り、6758 を保有する → 一覧は 6758 の1行だけ。
// 売り切った 7203 は価格がなくても、合計評価額に入れず、「価格を取得できない銘柄を含みません」のフラグ（TotalExcludesUnpriced）も立てない
// （5.5: 価格のない売り切りの銘柄は注記を出さない）。
func Test補助_売り切った銘柄は一覧に出さず合計評価額にも価格なしの注記にも響かない(t *testing.T) {
	t.Parallel()
	soldOut := unpriced(toyotaCode, toyotaName) // 価格は一度も保存されていない
	rows := []dto.TradeRow{
		soldOut.buy(1, 100, 20000, "2026-01-10"),
		soldOut.sell(2, 100, 25000, "2026-02-10"),
		sony().buy(3, 10, 32000, "2026-01-10"),
	}

	list := calculate(t, rows)

	h := onlyHolding(t, list, sonyCode)
	if h.Valuation == nil || *h.Valuation != 35000 {
		t.Errorf("6758 の評価額: got %v, want 35000（10株 × 3,500円）", h.Valuation)
	}
	if list.TotalValuation != 35000 {
		t.Errorf("合計評価額: got %d, want 35000", list.TotalValuation)
	}
	if list.TotalExcludesUnpriced {
		t.Error("TotalExcludesUnpriced: got true, want false（価格のない銘柄は売り切っていて一覧にない）")
	}
}

// 5.3: 売り切った銘柄だけ → 一覧は空のスライス、合計評価額 0、注記のフラグは false。
func Test補助_売り切った銘柄だけなら一覧は空になる(t *testing.T) {
	t.Parallel()
	s := toyota()
	rows := []dto.TradeRow{
		s.buy(1, 100, 20000, "2026-01-10"),
		s.sell(2, 100, 25000, "2026-02-10"),
	}

	list := calculate(t, rows)

	if list.Holdings == nil || len(list.Holdings) != 0 {
		t.Errorf("Holdings: got %v, want 空のスライス（nil ではない）", list.Holdings)
	}
	if list.TotalValuation != 0 || list.TotalExcludesUnpriced {
		t.Errorf("合計評価額・注記のフラグ: got %d・%v, want 0・false", list.TotalValuation, list.TotalExcludesUnpriced)
	}
}

// 5.3: 価格のない銘柄は合計評価額に入れない。7203（価格あり）と 9984（価格なし）→ 合計評価額は 7203 の 300,000円だけ、
// TotalExcludesUnpriced は true。9984 の現在の価格・基準日・評価額は nil。
func Test補助_価格のない銘柄は合計評価額に入れず注記のフラグを立てる(t *testing.T) {
	t.Parallel()
	rows := []dto.TradeRow{
		toyota().buy(1, 100, 25000, "2026-01-10"),
		softbank().buy(2, 10, 90000, "2026-01-10"),
	}

	list := calculate(t, rows)

	if got, want := codes(list), []string{toyotaCode, sbgCode}; !slices.Equal(got, want) {
		t.Fatalf("銘柄コード: got %v, want %v", got, want)
	}
	if list.TotalValuation != 300000 {
		t.Errorf("合計評価額: got %d, want 300000", list.TotalValuation)
	}
	if !list.TotalExcludesUnpriced {
		t.Error("TotalExcludesUnpriced: got false, want true")
	}
	noPrice := list.Holdings[1]
	if noPrice.CurrentPriceTenths != nil || noPrice.PriceDate != nil || noPrice.Valuation != nil {
		t.Errorf("価格のない銘柄の現在の価格・基準日・評価額: got %v・%v・%v, want すべて nil",
			noPrice.CurrentPriceTenths, noPrice.PriceDate, noPrice.Valuation)
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
			s.sell(1, 10, 25000, "2026-01-10"),
		}},
		{"同じ取引日で売りが先", []dto.TradeRow{
			s.sell(1, 100, 25000, "2026-01-10"),
			s.buy(2, 100, 20000, "2026-01-10"),
		}},
		{"最後は0に戻るが途中で負になる", []dto.TradeRow{
			s.sell(1, 100, 25000, "2026-01-10"),
			s.buy(2, 100, 20000, "2026-02-10"),
		}},
		{"持っている株数より多く売る", []dto.TradeRow{
			s.buy(1, 100, 20000, "2026-01-10"),
			s.sell(2, 101, 25000, "2026-02-10"),
		}},
	}
}

// 5.3・判断6: 売買履歴をたどる途中で保有数量が負になる銘柄があれば ErrNegativeQuantity（誤った一覧を返さない）。
func Test補助_途中で保有数量が負になる売買履歴はErrNegativeQuantityになる(t *testing.T) {
	t.Parallel()
	for _, tc := range negativeCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := query.CalculateHoldings(tc.rows)

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
		sony().buy(1, 10, 32000, "2026-01-10"),
		toyota().sell(2, 10, 25000, "2026-01-10"),
	}

	_, err := query.CalculateHoldings(rows)

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
//  1. 終値 3,000円で 10^18 株（評価額があふれる）
//  2. 5×10^18 株の買い2件（保有数量があふれる）
//  3. 行ごとには収まり、合計評価額だけがあふれる2銘柄（どちらも評価額 5×10^18円。合計 10^19円）
//  4. 境界の1つ上: 保有数量が 2^63（2^62 株の買い2件）。終値 1.1円で int64 の最大の株数（評価額があふれる）
//
// 価格は 0.1円（Tenths で 1）とした（取得価格の値は結果に関係しない）。
func overflowCases() []overflowCase {
	return []overflowCase{
		{"1_終値3000円で10の18乗株は評価額があふれる", []dto.TradeRow{
			toyota().buy(1, 1000000000000000000, 1, "2026-01-10"),
		}},
		{"2_5かける10の18乗株の買い2件は保有数量があふれる", []dto.TradeRow{
			toyota().buy(1, 5000000000000000000, 1, "2026-01-10"),
			toyota().buy(2, 5000000000000000000, 1, "2026-01-11"),
		}},
		{"3_行ごとには収まり合計評価額だけがあふれる2銘柄", []dto.TradeRow{
			pricedAt(toyotaCode, toyotaName, 100).buy(1, 500000000000000000, 1, "2026-01-10"),
			pricedAt(sonyCode, sonyName, 100).buy(2, 500000000000000000, 1, "2026-01-10"),
		}},
		{"4a_境界の1つ上_保有数量が2の63乗", []dto.TradeRow{
			toyota().buy(1, 4611686018427387904, 1, "2026-01-10"),
			toyota().buy(2, 4611686018427387904, 1, "2026-01-11"),
		}},
		{"4b_境界の1つ上_int64の最大の株数で終値1.1円なら評価額があふれる", []dto.TradeRow{
			pricedAt(toyotaCode, toyotaName, 11).buy(1, 9223372036854775807, 1, "2026-01-10"),
		}},
	}
}

// 5.3・判断5: 最後の値（保有数量・評価額・合計評価額）が int64 に収まらなければ ErrCalculationOverflow（包んで返す）。
func Test補助_最後の値がint64に収まらなければErrCalculationOverflowになる(t *testing.T) {
	t.Parallel()
	for _, tc := range overflowCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := query.CalculateHoldings(tc.rows)

			if !errors.Is(err, query.ErrCalculationOverflow) {
				t.Errorf("CalculateHoldings: got err %v, want ErrCalculationOverflow", err)
			}
		})
	}
}

// 5.3・判断5（境界の対照）: ぎりぎり収まる値ならエラーにならず、正しい値を返す（黙って桁あふれした値を返さない）。
//   - 2^62 − 1 株と 2^62 株の買い（保有数量はちょうど int64 の最大 9223372036854775807）
//   - int64 の最大の株数を終値 1円で持つ（評価額はちょうど int64 の最大 9223372036854775807円）
func Test補助_int64にぎりぎり収まる値ならエラーにならない(t *testing.T) {
	t.Parallel()

	// 価格のない銘柄で確かめる（終値があると、評価額が先にあふれて保有数量の境界を見分けられないため）。
	t.Run("保有数量がint64の最大", func(t *testing.T) {
		t.Parallel()
		s := unpriced(toyotaCode, toyotaName)
		rows := []dto.TradeRow{
			s.buy(1, 4611686018427387903, 1, "2026-01-10"),
			s.buy(2, 4611686018427387904, 1, "2026-01-11"),
		}

		h := onlyHolding(t, calculate(t, rows), toyotaCode)

		if h.Quantity != 9223372036854775807 {
			t.Errorf("保有数量: got %d, want 9223372036854775807", h.Quantity)
		}
	})

	t.Run("評価額がint64の最大", func(t *testing.T) {
		t.Parallel()
		s := pricedAt(toyotaCode, toyotaName, 10)
		rows := []dto.TradeRow{s.buy(1, 9223372036854775807, 1, "2026-01-10")}

		list := calculate(t, rows)

		h := onlyHolding(t, list, toyotaCode)
		if h.Valuation == nil || *h.Valuation != 9223372036854775807 {
			t.Errorf("評価額: got %v, want 9223372036854775807", h.Valuation)
		}
		if list.TotalValuation != 9223372036854775807 {
			t.Errorf("合計評価額: got %d, want 9223372036854775807", list.TotalValuation)
		}
	})
}
