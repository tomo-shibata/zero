package model_test

import (
	"errors"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
)

// 仕様: 要件（docs/requirements/trade-history.md）の要件1（銘柄マスタは銘柄コード・銘柄名・通貨を持つ。銘柄コードと銘柄名は空でない）・
// 要件24（銘柄コードは半角の英大文字・数字・「.」・「-」だけで1〜10文字。銘柄名は空白以外の文字を1つ以上含む）・基準25 と、
// プラン（docs/plans/trade-history/plan.md）4.6（PR③）の model.Stock・NewStock・ErrInvalidStockCode・ErrBlankStockName、
// 4.7 の CheckStockCode・CheckStockName（全角の空白も空白として扱う）。

// 銘柄コードの書き方（要件24・基準25）。誤りは ErrInvalidStockCode。
func Test基準25_CheckStockCodeは半角の英大文字数字とピリオドとハイフンの1から10文字だけを受け付ける(t *testing.T) {
	t.Parallel()
	valid := []string{"7203", "130A", "BRK.B", "BRK-B", "ABCDEFGHIJ", "A", "0123456789", "A.-"}
	for _, code := range valid {
		t.Run("正しい_"+code, func(t *testing.T) {
			t.Parallel()
			if err := model.CheckStockCode(code); err != nil {
				t.Errorf("CheckStockCode(%q): got %v, want nil", code, err)
			}
		})
	}
	invalid := []struct {
		name string
		code string
	}{
		{"空", ""},
		{"小文字", "aapl"},
		{"小文字が混ざる", "Aapl"},
		{"11文字", "ABCDEFGHIJK"},
		{"途中に半角の空白", "72 03"},
		{"前に半角の空白", " 7203"},
		{"後ろに半角の空白", "7203 "},
		{"末尾に改行", "7203\n"},
		{"全角の数字", "７２０３"},
		{"全角の英字", "ＡＡＰＬ"},
		{"アンダースコア", "BRK_B"},
		{"スラッシュ", "BRK/B"},
		{"カンマ", "7203,"},
		{"日本語", "トヨタ"},
	}
	for _, tc := range invalid {
		t.Run("誤り_"+tc.name, func(t *testing.T) {
			t.Parallel()
			if err := model.CheckStockCode(tc.code); !errors.Is(err, model.ErrInvalidStockCode) {
				t.Errorf("CheckStockCode(%q): got %v, want ErrInvalidStockCode", tc.code, err)
			}
		})
	}
}

// 銘柄名の書き方（要件24・基準25）。空白以外の文字が1つもなければ ErrBlankStockName。全角の空白も空白として扱う（4.7）。
func Test基準25_CheckStockNameは空白以外の文字を1つ以上含む名前だけを受け付ける(t *testing.T) {
	t.Parallel()
	valid := []string{"トヨタ自動車", "Apple", "Alphabet Inc., Class A", " Apple ", "　全角空白で囲む　", "A"}
	for _, name := range valid {
		t.Run("正しい_"+name, func(t *testing.T) {
			t.Parallel()
			if err := model.CheckStockName(name); err != nil {
				t.Errorf("CheckStockName(%q): got %v, want nil", name, err)
			}
		})
	}
	invalid := []struct {
		name string
		in   string
	}{
		{"空", ""},
		{"半角の空白だけ", "   "},
		{"全角の空白だけ", "　"},
		{"全角と半角の空白だけ", " 　 　"},
		{"タブだけ", "\t"},
		{"改行だけ", "\r\n"},
	}
	for _, tc := range invalid {
		t.Run("誤り_"+tc.name, func(t *testing.T) {
			t.Parallel()
			if err := model.CheckStockName(tc.in); !errors.Is(err, model.ErrBlankStockName) {
				t.Errorf("CheckStockName(%q): got %v, want ErrBlankStockName", tc.in, err)
			}
		})
	}
}

// NewStock は、正しい銘柄コード・銘柄名・通貨から Stock を作る（要件1）。
func Test要件1_NewStockは正しい値から銘柄を作る(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		code     string
		stock    string
		currency model.Currency
	}{
		{"円の銘柄", "7203", "トヨタ自動車", model.CurrencyJPY},
		{"ドルの銘柄", "AAPL", "Apple", model.CurrencyUSD},
		{"カンマを含む銘柄名", "GOOGL", "Alphabet Inc., Class A", model.CurrencyUSD},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := model.NewStock(tc.code, tc.stock, tc.currency)
			if err != nil {
				t.Fatalf("NewStock(%q, %q, %q): got err %v, want nil", tc.code, tc.stock, tc.currency, err)
			}
			want := model.Stock{Code: tc.code, Name: tc.stock, Currency: tc.currency}
			if got != want {
				t.Errorf("NewStock(%q, %q, %q): got %+v, want %+v", tc.code, tc.stock, tc.currency, got, want)
			}
		})
	}
}

// NewStock は、通貨が円（JPY）・ドル（USD）以外ならエラーにする（要件1・要件24。エラーの値は決めない）。
func Test要件1_NewStockは円とドル以外の通貨をエラーにする(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		currency model.Currency
	}{
		{"EUR", model.Currency("EUR")},
		{"空", model.Currency("")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := model.NewStock("AAPL", "Apple", tc.currency); err == nil {
				t.Errorf("NewStock(通貨 %q): got nil, want エラー", tc.currency)
			}
		})
	}
}

// NewStock は、銘柄コードの書き方が誤りなら ErrInvalidStockCode、銘柄名に空白以外の文字がなければ ErrBlankStockName を返す。
func Test基準25_NewStockは書き方の誤りをエラーにする(t *testing.T) {
	t.Parallel()
	if _, err := model.NewStock("aapl", "Apple", model.CurrencyUSD); !errors.Is(err, model.ErrInvalidStockCode) {
		t.Errorf("NewStock(小文字の銘柄コード): got %v, want ErrInvalidStockCode", err)
	}
	if _, err := model.NewStock("ABCDEFGHIJK", "Apple", model.CurrencyUSD); !errors.Is(err, model.ErrInvalidStockCode) {
		t.Errorf("NewStock(11文字の銘柄コード): got %v, want ErrInvalidStockCode", err)
	}
	if _, err := model.NewStock("AAPL", "　", model.CurrencyUSD); !errors.Is(err, model.ErrBlankStockName) {
		t.Errorf("NewStock(全角の空白だけの銘柄名): got %v, want ErrBlankStockName", err)
	}
	if _, err := model.NewStock("AAPL", "", model.CurrencyUSD); !errors.Is(err, model.ErrBlankStockName) {
		t.Errorf("NewStock(空の銘柄名): got %v, want ErrBlankStockName", err)
	}
}
