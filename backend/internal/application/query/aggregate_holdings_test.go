package query_test

import (
	"slices"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/application/query"
	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// 仕様: 要件（docs/requirements/stock-holdings.md）の AC-4・AC-5（FR-4）、AC-6（FR-5）と、
// プラン（docs/plans/stock-holdings/plan.md）5.1「補助のテスト（バックエンド）」の PR② の単体の行
// （FR-4・FR-9（SR-4、TC-12）、FR-6・FR-9（TC-12）、FR-5（TC-17）、FR-7（SR-5）、FR-4（PR② の EC-2、TC-02））。
// 確かめる関数は 4.4 の AggregateHoldings（「集約の決まり」に従う）。
//
// 単価は 0.1円単位の整数（名前の末尾が Tenths）、評価額は円の整数（プラン 4.1）。
// 期待値は要件・プランに書かれた値をそのまま使い、テストの中で計算しない。

// 要件「成功基準」の銘柄マスタとダミー価格（7203=3,000円・6758=3,500円、どちらも 2026-10-06。9984 は価格なし）。
const (
	toyotaCode = "7203"
	toyotaName = "トヨタ自動車"
	sonyCode   = "6758"
	sonyName   = "ソニーグループ"
	sbgCode    = "9984"
	sbgName    = "ソフトバンクグループ"
	priceDate  = "2026-10-06"
)

// pricedRow は、価格が保存済みの銘柄の保有データ1件（集約前）を作る。
// 同じ銘柄の行は、DB から読むと同じ銘柄名・同じ最新の終値と基準日を持つ（4.4 の HoldingRow）。
func pricedRow(code, name string, quantity, acquisitionTenths, closeTenths int64, date string) dto.HoldingRow {
	return dto.HoldingRow{
		Code:                   code,
		Name:                   name,
		Quantity:               quantity,
		AcquisitionPriceTenths: acquisitionTenths,
		ClosePriceTenths:       new(closeTenths),
		PriceDate:              new(date),
	}
}

// unpricedRow は、価格が一度も保存されていない銘柄の保有データ1件（集約前）を作る（4.4: 価格がなければ nil）。
func unpricedRow(code, name string, quantity, acquisitionTenths int64) dto.HoldingRow {
	return dto.HoldingRow{
		Code:                   code,
		Name:                   name,
		Quantity:               quantity,
		AcquisitionPriceTenths: acquisitionTenths,
	}
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
		t.Fatalf("一覧の銘柄コード: got %v, want [%s]（同じ銘柄は1行にまとめる。FR-4）", got, code)
	}
	return list.Holdings[0]
}

// AC-4（FR-4）: 7203 を100株・取得価格2,000円と、300株・取得価格3,000円の2件で保有している
// → 7203 は1行で、保有数量400株、取得価格2,750円（Tenths で 27500）。
func TestAC4_同じ銘柄の2件は1行にまとめ保有数量は合計し取得価格は数量による加重平均になる(t *testing.T) {
	t.Parallel()
	rows := []dto.HoldingRow{
		pricedRow(toyotaCode, toyotaName, 100, 20000, 30000, priceDate),
		pricedRow(toyotaCode, toyotaName, 300, 30000, 30000, priceDate),
	}

	h := onlyHolding(t, query.AggregateHoldings(rows), toyotaCode)

	if h.Quantity != 400 {
		t.Errorf("保有数量: got %d, want 400", h.Quantity)
	}
	if h.AcquisitionPriceTenths != 27500 {
		t.Errorf("取得価格（0.1円単位）: got %d, want 27500（2,750円）", h.AcquisitionPriceTenths)
	}
	if h.Name != toyotaName {
		t.Errorf("銘柄名: got %q, want %q", h.Name, toyotaName)
	}
}

// AC-5（FR-4）: 7203 を1株・取得価格2,000円と、2株・取得価格2,001円の2件で保有している
// → 取得価格は2,000.7円（2,000.666…円の小数第2位を四捨五入。Tenths で 20007）。
func TestAC5_加重平均の取得価格は小数第2位を四捨五入して小数第1位までにする(t *testing.T) {
	t.Parallel()
	rows := []dto.HoldingRow{
		pricedRow(toyotaCode, toyotaName, 1, 20000, 30000, priceDate),
		pricedRow(toyotaCode, toyotaName, 2, 20010, 30000, priceDate),
	}

	h := onlyHolding(t, query.AggregateHoldings(rows), toyotaCode)

	if h.Quantity != 3 {
		t.Errorf("保有数量: got %d, want 3", h.Quantity)
	}
	if h.AcquisitionPriceTenths != 20007 {
		t.Errorf("取得価格（0.1円単位）: got %d, want 20007（2,000.7円）", h.AcquisitionPriceTenths)
	}
}

// AC-6（FR-5）: 9984、7203、6758 の順の行 → 6758、7203、9984 の順に並ぶ。
func TestAC6_一覧は銘柄コードの昇順に並ぶ(t *testing.T) {
	t.Parallel()
	rows := []dto.HoldingRow{
		unpricedRow(sbgCode, sbgName, 10, 90000),
		pricedRow(toyotaCode, toyotaName, 100, 25000, 30000, priceDate),
		pricedRow(sonyCode, sonyName, 10, 32000, 35000, priceDate),
	}

	got := codes(query.AggregateHoldings(rows))

	if want := []string{sonyCode, toyotaCode, sbgCode}; !slices.Equal(got, want) {
		t.Errorf("並び順: got %v, want %v", got, want)
	}
}

// 補助 FR-4（PR② の EC-2、TC-02）: 加重平均の取得価格の四捨五入の境界（小数第2位がちょうど 5 になる値と、その手前）。
// 単価は Tenths（20000 = 2,000.0円）。期待値はプラン 5.1 の行の値。
//   - {1株・20000、1株・20001} → 20001（平均 2,000.05円。5 は切り上げる。偶数への丸め・切り捨てなら 20000 になり誤り）
//   - {1株・20000、1株・20003} → 20002（平均 2,000.15円。5 は切り上げる。5 を切り捨てる丸めなら 20001 になり誤り）
//   - {2株・20000、1株・20001} → 20000（平均 2,000.033…円。4 以下は切り捨てる。切り上げなら 20001 になり誤り）
func TestFR4_PR2_EC2_TC02_加重平均の取得価格は小数第2位がちょうど5なら切り上げそれ未満なら切り捨てる(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		rows []dto.HoldingRow
		want int64
	}{
		{
			name: "1株20000と1株20001は20001",
			rows: []dto.HoldingRow{
				pricedRow(toyotaCode, toyotaName, 1, 20000, 30000, priceDate),
				pricedRow(toyotaCode, toyotaName, 1, 20001, 30000, priceDate),
			},
			want: 20001,
		},
		{
			name: "1株20000と1株20003は20002",
			rows: []dto.HoldingRow{
				pricedRow(toyotaCode, toyotaName, 1, 20000, 30000, priceDate),
				pricedRow(toyotaCode, toyotaName, 1, 20003, 30000, priceDate),
			},
			want: 20002,
		},
		{
			name: "2株20000と1株20001は20000",
			rows: []dto.HoldingRow{
				pricedRow(toyotaCode, toyotaName, 2, 20000, 30000, priceDate),
				pricedRow(toyotaCode, toyotaName, 1, 20001, 30000, priceDate),
			},
			want: 20000,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := onlyHolding(t, query.AggregateHoldings(tc.rows), toyotaCode)

			if h.AcquisitionPriceTenths != tc.want {
				t.Errorf("取得価格（0.1円単位）: got %d, want %d", h.AcquisitionPriceTenths, tc.want)
			}
		})
	}
}

// 補助 FR-4・FR-9（SR-4、TC-12）: 7203 を1株＋1株、価格 1,234.5円（Tenths で 12345）
// → 1行・2株・評価額 2,469円。
// 「集約の決まり」2: 評価額は、まとめた後の行で合計した保有数量を使って1回だけ計算する
// （まとめる前の行ごとに切り捨ててから足すと 1,234 + 1,234 = 2,468円 になり、誤り）。
func TestFR4_FR9_SR4_TC12_まとめた行の評価額は合計した保有数量で1回だけ計算する(t *testing.T) {
	t.Parallel()
	rows := []dto.HoldingRow{
		pricedRow(toyotaCode, toyotaName, 1, 20000, 12345, priceDate),
		pricedRow(toyotaCode, toyotaName, 1, 20000, 12345, priceDate),
	}

	h := onlyHolding(t, query.AggregateHoldings(rows), toyotaCode)

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

// 補助 FR-6・FR-9（TC-12）: 7203 を1株、6758 を1株、どちらも価格 1,234.5円（Tenths で 12345）
// → 合計評価額 2,468円。
// 「集約の決まり」3: 合計評価額は、まとめた後の行の評価額（それぞれ 1,234円）を足したもの
// （全体を掛けてから切り捨てると 2,469円 になり、誤り）。
func TestFR6_FR9_TC12_合計評価額は行ごとに切り捨てた評価額の合計(t *testing.T) {
	t.Parallel()
	rows := []dto.HoldingRow{
		pricedRow(toyotaCode, toyotaName, 1, 20000, 12345, priceDate),
		pricedRow(sonyCode, sonyName, 1, 20000, 12345, priceDate),
	}

	list := query.AggregateHoldings(rows)

	if list.TotalValuation != 2468 {
		t.Errorf("合計評価額: got %d, want 2468", list.TotalValuation)
	}
}

// 補助 FR-5（TC-17）: 130A・1301・7203 の行 → 1301、130A、7203 の順。
// 「集約の決まり」4: 並び順は銘柄コードの文字列としての昇順（数字の「1」は英字の「A」より前）。
// 1301・130A は銘柄マスタにない銘柄だが、この関数は DB を使わないので、ここでは仮の銘柄名で渡す。
func TestFR5_TC17_英字を含む銘柄コードも文字列としての昇順に並ぶ(t *testing.T) {
	t.Parallel()
	rows := []dto.HoldingRow{
		unpricedRow("130A", "銘柄130A", 10, 10000),
		unpricedRow("1301", "銘柄1301", 10, 10000),
		pricedRow(toyotaCode, toyotaName, 10, 25000, 30000, priceDate),
	}

	got := codes(query.AggregateHoldings(rows))

	if want := []string{"1301", "130A", toyotaCode}; !slices.Equal(got, want) {
		t.Errorf("並び順: got %v, want %v", got, want)
	}
}

// 補助 FR-7（SR-5）: 保有データが0件 → Holdings は空のスライス（nil ではない）。合計評価額は 0、注記のフラグは false。
// 4.4 の HoldingsList（「0件のときは空のスライス（nil ではない）」）と、API の0件の形
// （{"holdings":[],"totalValuation":0,"totalExcludesUnpriced":false}。nil だと JSON が null になる）。
// 0件は、nil のスライスと長さ0のスライスのどちらで渡されても同じ結果になること。
func TestFR7_SR5_保有データが0件なら一覧は空のスライスになる(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		rows []dto.HoldingRow
	}{
		{"nilのスライス", nil},
		{"長さ0のスライス", []dto.HoldingRow{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			list := query.AggregateHoldings(tc.rows)

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
