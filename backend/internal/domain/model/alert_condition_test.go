package model_test

import (
	"errors"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
)

// 仕様: 価格の変動の通知の要件（docs/requirements/price-alerts.md）の要件2（種類・向き・値の単位・選べる対象）・要件4（値の範囲と精度）・基準24と、
// プラン（docs/plans/price-alerts/plan.md）4.8 の PR① の境界（AlertKind・AlertScope・AlertDirection・AlertCondition・NewAlertCondition・ErrInvalidAlertCondition）、
// 5.1 の基準24 の行（範囲の両端の内と外、精度、組み合わせ）。
// 値は 0.01 単位の整数（20% は 2000、100,000円は 10000000、150.25ドルは 15025）。% の種類と全銘柄の条件では currency を見ない。
// 期待値は要件に書かれた範囲をそのまま使う（% は小数第1位まで、円の金額は整数、円の価格は小数第1位まで、ドルは小数第2位まで）。

// alertCase は、NewAlertCondition に渡す値と、期待（誤りなら true）。
type alertCase struct {
	name      string
	kind      model.AlertKind
	scope     model.AlertScope
	stockCode string
	direction model.AlertDirection
	value     int64
	currency  model.Currency
	invalid   bool
}

func runAlertCases(t *testing.T, cases []alertCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := model.NewAlertCondition(tc.kind, tc.scope, tc.stockCode, tc.direction, tc.value, tc.currency)

			if tc.invalid {
				if !errors.Is(err, model.ErrInvalidAlertCondition) {
					t.Errorf("NewAlertCondition: got err %v, want ErrInvalidAlertCondition", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewAlertCondition: got err %v, want nil", err)
			}
			want := model.AlertCondition{Kind: tc.kind, Scope: tc.scope, StockCode: tc.stockCode, Direction: tc.direction, ThresholdHundredths: tc.value}
			if got != want {
				t.Errorf("NewAlertCondition: got %+v, want %+v", got, want)
			}
		})
	}
}

// 基準24（要件4）: % の値。下がった（取得価格から・最高値から・前の取引日から）は 0.1〜99.9 の小数第1位まで。
// 0.1% は 10、99.9% は 9990、100% は 10000、20.05% は 2005（小数第2位がある）、0 と負の値は誤り。
func Test基準24_下がったの率は0_1から99_9まで小数第1位までを受け付ける(t *testing.T) {
	t.Parallel()
	kinds := []struct {
		name  string
		kind  model.AlertKind
		scope model.AlertScope
		code  string
	}{
		{"取得価格から_全銘柄", model.AlertKindCost, model.AlertScopeAll, ""},
		{"取得価格から_1銘柄", model.AlertKindCost, model.AlertScopeStock, "7203"},
		{"最高値から_全銘柄", model.AlertKindPeak, model.AlertScopeAll, ""},
		{"最高値から_1銘柄", model.AlertKindPeak, model.AlertScopeStock, "7203"},
		{"前の取引日から_全銘柄", model.AlertKindPrev, model.AlertScopeAll, ""},
	}
	var cases []alertCase
	for _, k := range kinds {
		for _, v := range []struct {
			name    string
			value   int64
			invalid bool
		}{
			{"0は誤り", 0, true},
			{"負の値は誤り", -2000, true},
			{"0.1は正しい", 10, false},
			{"20は正しい", 2000, false},
			{"99.9は正しい", 9990, false},
			{"100は誤り", 10000, true},
			{"20.05は誤り（小数第2位）", 2005, true},
			{"0.05は誤り（小数第2位）", 5, true},
			{"99.95は誤り（小数第2位）", 9995, true},
		} {
			cases = append(cases, alertCase{
				name: k.name + "_" + v.name, kind: k.kind, scope: k.scope, stockCode: k.code,
				direction: model.AlertDirectionDown, value: v.value, invalid: v.invalid,
			})
		}
	}
	runAlertCases(t, cases)
}

// 基準24（要件4）: 上がった・動いたの率は 0.1〜1,000 の小数第1位まで。1,000% は 100000 で正しく、1000.1% は 100010 で誤り。
func Test基準24_上がったと動いたの率は0_1から1000まで小数第1位までを受け付ける(t *testing.T) {
	t.Parallel()
	kinds := []struct {
		name      string
		kind      model.AlertKind
		direction model.AlertDirection
	}{
		{"取得価格から_上がった", model.AlertKindCost, model.AlertDirectionUp},
		{"前の取引日から_上がった", model.AlertKindPrev, model.AlertDirectionUp},
		{"前の取引日から_動いた", model.AlertKindPrev, model.AlertDirectionEither},
	}
	var cases []alertCase
	for _, k := range kinds {
		for _, v := range []struct {
			name    string
			value   int64
			invalid bool
		}{
			{"0は誤り", 0, true},
			{"0.1は正しい", 10, false},
			{"99.9は正しい", 9990, false},
			{"100は正しい（下がったと違い上限が1,000）", 10000, false},
			{"1000は正しい", 100000, false},
			{"1000.1は誤り", 100010, true},
			{"20.05は誤り（小数第2位）", 2005, true},
		} {
			cases = append(cases, alertCase{
				name: k.name + "_" + v.name, kind: k.kind, scope: model.AlertScopeAll,
				direction: k.direction, value: v.value, invalid: v.invalid,
			})
		}
	}
	runAlertCases(t, cases)
}

// 基準24（要件4）: 円の含み損益の金額は 1〜1,000,000,000 の整数。1円は 100、1.5円は 150（誤り）、100,000円は 10000000、
// 1,000,000,000円は 100000000000（正しい）、1,000,000,001円は 100000000100（誤り）。
func Test基準24_円の含み損益の金額は1から10億までの整数を受け付ける(t *testing.T) {
	t.Parallel()
	var cases []alertCase
	for _, d := range []struct {
		name      string
		direction model.AlertDirection
	}{{"含み損", model.AlertDirectionLoss}, {"含み益", model.AlertDirectionGain}} {
		for _, v := range []struct {
			name    string
			value   int64
			invalid bool
		}{
			{"0は誤り", 0, true},
			{"負の値は誤り", -100, true},
			{"0.5円は誤り", 50, true},
			{"1円は正しい", 100, false},
			{"1.5円は誤り（整数ではない）", 150, true},
			{"100000円は正しい", 10000000, false},
			{"10億円は正しい", 100000000000, false},
			{"10億1円は誤り", 100000000100, true},
		} {
			cases = append(cases, alertCase{
				name: d.name + "_" + v.name, kind: model.AlertKindPnl, scope: model.AlertScopeStock, stockCode: "7203",
				direction: d.direction, value: v.value, currency: model.CurrencyJPY, invalid: v.invalid,
			})
		}
	}
	runAlertCases(t, cases)
}

// 基準24（要件4）: ドルの含み損益の金額は 0.01〜10,000,000 で小数第2位まで。0.01ドルは 1 で正しく、0 は誤り、
// 10,000,000ドルは 1000000000 で正しく、10,000,000.01ドルは 1000000001 で誤り。
func Test基準24_ドルの含み損益の金額は0_01から1000万までを受け付ける(t *testing.T) {
	t.Parallel()
	var cases []alertCase
	for _, d := range []struct {
		name      string
		direction model.AlertDirection
	}{{"含み損", model.AlertDirectionLoss}, {"含み益", model.AlertDirectionGain}} {
		for _, v := range []struct {
			name    string
			value   int64
			invalid bool
		}{
			{"0は誤り", 0, true},
			{"負の値は誤り", -1, true},
			{"0.01ドルは正しい", 1, false},
			{"300.00ドルは正しい", 30000, false},
			{"1000万ドルは正しい", 1000000000, false},
			{"1000万0.01ドルは誤り", 1000000001, true},
		} {
			cases = append(cases, alertCase{
				name: d.name + "_" + v.name, kind: model.AlertKindPnl, scope: model.AlertScopeStock, stockCode: "AAPL",
				direction: d.direction, value: v.value, currency: model.CurrencyUSD, invalid: v.invalid,
			})
		}
	}
	runAlertCases(t, cases)
}

// 基準24（要件4）: 円の価格は 0.1〜99,999,999.9 で小数第1位まで。0.1円は 10 で正しい、0.05円は 5 と 3,000.25円は 300025 は小数第2位があるので誤り、
// 99,999,999.9円は 9999999990 で正しく、100,000,000円は 10000000000 で誤り。
func Test基準24_円の価格は0_1から99999999_9まで小数第1位までを受け付ける(t *testing.T) {
	t.Parallel()
	var cases []alertCase
	for _, d := range []struct {
		name      string
		direction model.AlertDirection
	}{{"以上", model.AlertDirectionAbove}, {"以下", model.AlertDirectionBelow}} {
		for _, v := range []struct {
			name    string
			value   int64
			invalid bool
		}{
			{"0は誤り", 0, true},
			{"負の値は誤り", -10, true},
			{"0.05円は誤り（小数第2位）", 5, true},
			{"0.1円は正しい", 10, false},
			{"3000円は正しい", 300000, false},
			{"3000.2円は正しい", 300020, false},
			{"3000.25円は誤り（小数第2位）", 300025, true},
			{"99999999.9円は正しい", 9999999990, false},
			{"1億円は誤り", 10000000000, true},
		} {
			cases = append(cases, alertCase{
				name: d.name + "_" + v.name, kind: model.AlertKindPrice, scope: model.AlertScopeStock, stockCode: "7203",
				direction: d.direction, value: v.value, currency: model.CurrencyJPY, invalid: v.invalid,
			})
		}
	}
	runAlertCases(t, cases)
}

// 基準24（要件4）: ドルの価格は 0.01〜99,999,999.99 で小数第2位まで。0.01ドルは 1 で正しく、150.25ドルは 15025 で正しく、
// 99,999,999.99ドルは 9999999999 で正しく、1億ドルは 10000000000 で誤り、0 は誤り。
func Test基準24_ドルの価格は0_01から99999999_99までを受け付ける(t *testing.T) {
	t.Parallel()
	var cases []alertCase
	for _, d := range []struct {
		name      string
		direction model.AlertDirection
	}{{"以上", model.AlertDirectionAbove}, {"以下", model.AlertDirectionBelow}} {
		for _, v := range []struct {
			name    string
			value   int64
			invalid bool
		}{
			{"0は誤り", 0, true},
			{"負の値は誤り", -1, true},
			{"0.01ドルは正しい", 1, false},
			{"150.25ドルは正しい", 15025, false},
			{"99999999.99ドルは正しい", 9999999999, false},
			{"1億ドルは誤り", 10000000000, true},
		} {
			cases = append(cases, alertCase{
				name: d.name + "_" + v.name, kind: model.AlertKindPrice, scope: model.AlertScopeStock, stockCode: "AAPL",
				direction: d.direction, value: v.value, currency: model.CurrencyUSD, invalid: v.invalid,
			})
		}
	}
	runAlertCases(t, cases)
}

// 要件2: 種類・対象・向きの組み合わせ。表にない組み合わせは ErrInvalidAlertCondition。値は、その種類で正しい値（20% など）を使い、組み合わせだけを誤りにする。
func Test基準24_要件2_種類と対象と向きの組み合わせを見分ける(t *testing.T) {
	t.Parallel()
	cases := []alertCase{
		// 正しい組み合わせ
		{name: "取得価格から_全銘柄_上がった", kind: model.AlertKindCost, scope: model.AlertScopeAll, direction: model.AlertDirectionUp, value: 2000},
		{name: "取得価格から_全銘柄_下がった", kind: model.AlertKindCost, scope: model.AlertScopeAll, direction: model.AlertDirectionDown, value: 2000},
		{name: "取得価格から_1銘柄_下がった", kind: model.AlertKindCost, scope: model.AlertScopeStock, stockCode: "7203", direction: model.AlertDirectionDown, value: 2000},
		{name: "最高値から_全銘柄_下がった", kind: model.AlertKindPeak, scope: model.AlertScopeAll, direction: model.AlertDirectionDown, value: 1500},
		{name: "前の取引日から_全銘柄_動いた", kind: model.AlertKindPrev, scope: model.AlertScopeAll, direction: model.AlertDirectionEither, value: 500},
		{name: "含み損益の金額_1銘柄_含み損_円", kind: model.AlertKindPnl, scope: model.AlertScopeStock, stockCode: "7203", direction: model.AlertDirectionLoss, value: 10000000, currency: model.CurrencyJPY},
		{name: "決めた価格_1銘柄_以下_ドル", kind: model.AlertKindPrice, scope: model.AlertScopeStock, stockCode: "AAPL", direction: model.AlertDirectionBelow, value: 15025, currency: model.CurrencyUSD},

		// 含み損益の金額と決めた価格は、全銘柄にできない
		{name: "含み損益の金額_全銘柄は誤り", kind: model.AlertKindPnl, scope: model.AlertScopeAll, direction: model.AlertDirectionLoss, value: 10000000, currency: model.CurrencyJPY, invalid: true},
		{name: "決めた価格_全銘柄は誤り", kind: model.AlertKindPrice, scope: model.AlertScopeAll, direction: model.AlertDirectionAbove, value: 300000, currency: model.CurrencyJPY, invalid: true},

		// 向きが種類に合わない
		{name: "最高値から_上がったは誤り", kind: model.AlertKindPeak, scope: model.AlertScopeAll, direction: model.AlertDirectionUp, value: 1500, invalid: true},
		{name: "最高値から_動いたは誤り", kind: model.AlertKindPeak, scope: model.AlertScopeAll, direction: model.AlertDirectionEither, value: 1500, invalid: true},
		{name: "取得価格から_動いたは誤り", kind: model.AlertKindCost, scope: model.AlertScopeAll, direction: model.AlertDirectionEither, value: 2000, invalid: true},
		{name: "取得価格から_含み損は誤り", kind: model.AlertKindCost, scope: model.AlertScopeAll, direction: model.AlertDirectionLoss, value: 2000, invalid: true},
		{name: "前の取引日から_以上は誤り", kind: model.AlertKindPrev, scope: model.AlertScopeAll, direction: model.AlertDirectionAbove, value: 500, invalid: true},
		{name: "含み損益の金額_上がったは誤り", kind: model.AlertKindPnl, scope: model.AlertScopeStock, stockCode: "7203", direction: model.AlertDirectionUp, value: 10000000, currency: model.CurrencyJPY, invalid: true},
		{name: "含み損益の金額_以上は誤り", kind: model.AlertKindPnl, scope: model.AlertScopeStock, stockCode: "7203", direction: model.AlertDirectionAbove, value: 10000000, currency: model.CurrencyJPY, invalid: true},
		{name: "決めた価格_含み益は誤り", kind: model.AlertKindPrice, scope: model.AlertScopeStock, stockCode: "7203", direction: model.AlertDirectionGain, value: 300000, currency: model.CurrencyJPY, invalid: true},
		{name: "決めた価格_下がったは誤り", kind: model.AlertKindPrice, scope: model.AlertScopeStock, stockCode: "7203", direction: model.AlertDirectionDown, value: 300000, currency: model.CurrencyJPY, invalid: true},

		// 対象と銘柄コード: 全銘柄は銘柄コードが空、1銘柄は銘柄コードが空でない
		{name: "全銘柄なのに銘柄コードがあるのは誤り", kind: model.AlertKindCost, scope: model.AlertScopeAll, stockCode: "7203", direction: model.AlertDirectionDown, value: 2000, invalid: true},
		{name: "1銘柄なのに銘柄コードが空なのは誤り", kind: model.AlertKindCost, scope: model.AlertScopeStock, stockCode: "", direction: model.AlertDirectionDown, value: 2000, invalid: true},

		// 知らない値
		{name: "知らない種類は誤り", kind: model.AlertKind("volume"), scope: model.AlertScopeAll, direction: model.AlertDirectionDown, value: 2000, invalid: true},
		{name: "知らない対象は誤り", kind: model.AlertKindCost, scope: model.AlertScope("group"), direction: model.AlertDirectionDown, value: 2000, invalid: true},
		{name: "知らない向きは誤り", kind: model.AlertKindCost, scope: model.AlertScopeAll, direction: model.AlertDirection("sideways"), value: 2000, invalid: true},
	}
	runAlertCases(t, cases)
}
