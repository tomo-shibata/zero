package model_test

import (
	"errors"
	"math"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
)

// 仕様: おまかせ運用の要件（docs/requirements/managed-account.md）の要件1（名前は空白以外の文字を1つ以上含む。書いたとおりに持つ）・
// 要件2（記録は日付・評価額・損益）・要件3（評価額は1以上1兆未満、損益は絶対値が1兆未満、元本（評価額 − 損益）は1円以上、
// 日付は取り込んだ日（日本時間）以前）と、プラン（docs/plans/managed-account/plan.md）4.8 の PR① の境界
// （ManagedValuation・CheckManagedAccountName・CheckValuationDate・CheckValuation・CheckGain・CheckPrincipal・NewManagedValuation と各エラー）、
// 5.2 の「domain」。「今日」は 2026-11-16（日本時間。trade_test.go の today）。

const managedToday = "2026-11-16"

// 要件3: 評価額は 1以上 999,999,999,999 以下。0 以下は ErrValuationNotPositive、1兆以上は ErrValuationTooLarge。
func Test要件3_CheckValuationは1以上1兆未満なら通し範囲外はエラーを返す(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		valuation int64
		want      error
	}{
		{"負の値", -1, model.ErrValuationNotPositive},
		{"0", 0, model.ErrValuationNotPositive},
		{"1は通る", 1, nil},
		{"999999999999は通る", 999_999_999_999, nil},
		{"1兆は大きすぎる", 1_000_000_000_000, model.ErrValuationTooLarge},
		{"int64の最大は大きすぎる", math.MaxInt64, model.ErrValuationTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := model.CheckValuation(tc.valuation); !errors.Is(got, tc.want) {
				t.Errorf("CheckValuation(%d): got %v, want %v", tc.valuation, got, tc.want)
			}
		})
	}
}

// 要件3: 損益は 0 とマイナスを含み、絶対値が1兆未満（-999,999,999,999 以上 999,999,999,999 以下）。範囲外は ErrGainOutOfRange。
func Test要件3_CheckGainは絶対値が1兆未満なら通し範囲外はエラーを返す(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		gain int64
		want error
	}{
		{"0は通る", 0, nil},
		{"999999999999は通る", 999_999_999_999, nil},
		{"-999999999999は通る", -999_999_999_999, nil},
		{"1兆は範囲外", 1_000_000_000_000, model.ErrGainOutOfRange},
		{"-1兆は範囲外", -1_000_000_000_000, model.ErrGainOutOfRange},
		{"int64の最大は範囲外", math.MaxInt64, model.ErrGainOutOfRange},
		{"int64の最小は範囲外", math.MinInt64, model.ErrGainOutOfRange},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := model.CheckGain(tc.gain); !errors.Is(got, tc.want) {
				t.Errorf("CheckGain(%d): got %v, want %v", tc.gain, got, tc.want)
			}
		})
	}
}

// 要件3: 元本（評価額 − 損益）は1円以上。1円未満は ErrPrincipalNotPositive。どの int64 を渡しても、あふれずに比べる。
func Test要件3_CheckPrincipalは元本が1円以上なら通しあふれずに比べる(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		valuation int64
		gain      int64
		want      error
	}{
		{"元本1（1000と999）", 1000, 999, nil},
		{"元本1（評価額1と損益-999999999999は元本が大きい）", 1, -999_999_999_999, nil},
		{"元本0（1000と1000）", 1000, 1000, model.ErrPrincipalNotPositive},
		{"元本が負（1000と1500）", 1000, 1500, model.ErrPrincipalNotPositive},
		{"損益が0なら元本は評価額", 5, 0, nil},
		{"MaxInt64とMinInt64はあふれずに正しく通る", math.MaxInt64, math.MinInt64, nil},
		{"MinInt64とMaxInt64はあふれずに誤り", math.MinInt64, math.MaxInt64, model.ErrPrincipalNotPositive},
		{"MaxInt64とMaxInt64は元本0で誤り", math.MaxInt64, math.MaxInt64, model.ErrPrincipalNotPositive},
		{"MinInt64とMinInt64は元本0で誤り", math.MinInt64, math.MinInt64, model.ErrPrincipalNotPositive},
		{"MaxInt64と0は通る", math.MaxInt64, 0, nil},
		{"0とMinInt64は元本が正で通る", 0, math.MinInt64, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := model.CheckPrincipal(tc.valuation, tc.gain); !errors.Is(got, tc.want) {
				t.Errorf("CheckPrincipal(%d, %d): got %v, want %v", tc.valuation, tc.gain, got, tc.want)
			}
		})
	}
}

// 要件1: 名前は空白以外の文字を1つ以上含む。空・半角の空白・全角の空白・タブだけは ErrBlankManagedAccountName。
// 前に空白があっても、空白以外の文字があれば通る。
func Test要件1_CheckManagedAccountNameは空白以外の文字が1つ以上あれば通す(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want error
	}{
		{"空", "", model.ErrBlankManagedAccountName},
		{"半角の空白だけ", "   ", model.ErrBlankManagedAccountName},
		{"全角の空白だけ", "　", model.ErrBlankManagedAccountName},
		{"タブだけ", "\t", model.ErrBlankManagedAccountName},
		{"前に空白がある名前は通る", " a", nil},
		{"空白を含む名前は通る", "おまかせ NISA", nil},
		{"普通の名前は通る", "WealthNavi", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := model.CheckManagedAccountName(tc.in); !errors.Is(got, tc.want) {
				t.Errorf("CheckManagedAccountName(%q): got %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// 要件3: 日付は取り込んだ日（日本時間）以前。今日 2026-11-16 と昨日は通り、翌日 2026-11-17 は ErrFutureValuationDate。
func Test要件3_CheckValuationDateは今日以前なら通し後なら未来の日付の誤りを返す(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		date string
		want error
	}{
		{"今日は通る", "2026-11-16", nil},
		{"昨日は通る", "2026-11-15", nil},
		{"去年は通る", "2025-11-16", nil},
		{"翌日は未来", "2026-11-17", model.ErrFutureValuationDate},
		{"来年は未来", "2027-01-01", model.ErrFutureValuationDate},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := model.CheckValuationDate(tc.date, managedToday); !errors.Is(got, tc.want) {
				t.Errorf("CheckValuationDate(%q, %q): got %v, want %v", tc.date, managedToday, got, tc.want)
			}
		})
	}
}

// NewManagedValuation: 正しい値から記録を作る。名前は書いたとおりに持ち、前後の空白も取り除かない（要件1）。
func Test要件1_NewManagedValuationは名前の前後の空白を取り除かずに記録を作る(t *testing.T) {
	t.Parallel()
	got, err := model.NewManagedValuation(" WealthNavi ", "2026-11-16", 4_000_000, 1_000_000, managedToday)
	if err != nil {
		t.Fatalf("NewManagedValuation: got err %v, want nil", err)
	}
	want := model.ManagedValuation{AccountName: " WealthNavi ", ValuationDate: "2026-11-16", Valuation: 4_000_000, Gain: 1_000_000}
	if got != want {
		t.Errorf("NewManagedValuation:\n got %+v\nwant %+v", got, want)
	}
}

// NewManagedValuation: 損益がマイナスでも 0 でも作れる（要件3）。
func Test要件3_NewManagedValuationは損益がマイナスや0でも記録を作る(t *testing.T) {
	t.Parallel()
	for _, gain := range []int64{-20_000, 0} {
		got, err := model.NewManagedValuation("Alpha", "2026-11-01", 500_000, gain, managedToday)
		if err != nil {
			t.Fatalf("NewManagedValuation(損益 %d): got err %v, want nil", gain, err)
		}
		want := model.ManagedValuation{AccountName: "Alpha", ValuationDate: "2026-11-01", Valuation: 500_000, Gain: gain}
		if got != want {
			t.Errorf("NewManagedValuation(損益 %d):\n got %+v\nwant %+v", gain, got, want)
		}
	}
}

// NewManagedValuation: 名前 → 日付 → 評価額 → 損益 → 元本 の順（CSV の列の順）に確かめ、最初に当たった誤りを返す。
func Test要件29_NewManagedValuationは列の順で最初の誤りを返す(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		account   string
		date      string
		valuation int64
		gain      int64
		want      error
	}{
		{"名前が空白で評価額も0なら名前の誤り", "  ", "2026-11-16", 0, 0, model.ErrBlankManagedAccountName},
		{"日付が未来で評価額も0なら日付の誤り", "WealthNavi", "2026-11-17", 0, 0, model.ErrFutureValuationDate},
		{"評価額が0で損益が1兆なら評価額の誤り", "WealthNavi", "2026-11-16", 0, 1_000_000_000_000, model.ErrValuationNotPositive},
		{"評価額が1兆以上なら評価額の誤り", "WealthNavi", "2026-11-16", 1_000_000_000_000, 0, model.ErrValuationTooLarge},
		{"損益が範囲外で元本も1円未満なら損益の誤り", "WealthNavi", "2026-11-16", 1000, 1_000_000_000_000, model.ErrGainOutOfRange},
		{"元本が0なら元本の誤り", "WealthNavi", "2026-11-16", 1000, 1000, model.ErrPrincipalNotPositive},
		{"名前が空白で元本も0なら名前の誤り", "", "2026-11-16", 1000, 1000, model.ErrBlankManagedAccountName},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := model.NewManagedValuation(tc.account, tc.date, tc.valuation, tc.gain, managedToday)
			if !errors.Is(err, tc.want) {
				t.Errorf("NewManagedValuation(%q, %q, %d, %d): got %v, want %v", tc.account, tc.date, tc.valuation, tc.gain, err, tc.want)
			}
		})
	}
}
