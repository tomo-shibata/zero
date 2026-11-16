package csvinput_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/interface/csvinput"
)

// 仕様: 価格の CSV の登録のプラン（docs/plans/price-import/plan.md）4.3・4.5 の csvinput.ReadRecords と、5.2 の ReadRecords の単体の箇条書き。
//   - UTF-8 の確かめ・先頭の BOM の除去・見出しを含むすべてのレコードを返す
//   - 空の行は読み飛ばす（行番号には数える）。Record.Line はレコードが始まるファイルの行番号（見出しが1行目。値の中の改行も行に数える）
//   - UTF-8 でなければ ErrNotUTF8、ダブルクォートの誤りは最初の1か所を *QuoteError（Line はそのレコードが始まる行）で返す
//   - 列の数は確かめない

func mustRead(t *testing.T, content string) []csvinput.Record {
	t.Helper()
	got, err := csvinput.ReadRecords([]byte(content))
	if err != nil {
		t.Fatalf("ReadRecords(%q): 想定外のエラー: %v", content, err)
	}
	return got
}

func TestReadRecords_BOMありなしで同じレコードになる(t *testing.T) {
	t.Parallel()
	const body = "code,name\n7203,Toyota\n6758,Sony\n"
	want := []csvinput.Record{
		{Line: 1, Fields: []string{"code", "name"}},
		{Line: 2, Fields: []string{"7203", "Toyota"}},
		{Line: 3, Fields: []string{"6758", "Sony"}},
	}
	without := mustRead(t, body)
	with := mustRead(t, "\xEF\xBB\xBF"+body)
	if !reflect.DeepEqual(without, want) {
		t.Errorf("BOM なし: got %+v, want %+v", without, want)
	}
	if !reflect.DeepEqual(with, want) {
		t.Errorf("BOM あり: got %+v, want %+v（見出しの先頭に BOM が残ってはいけない）", with, want)
	}
}

func TestReadRecords_BOMだけのファイルはレコード0件(t *testing.T) {
	t.Parallel()
	if got := mustRead(t, "\xEF\xBB\xBF"); len(got) != 0 {
		t.Errorf("BOM だけ: got %+v, want 0件", got)
	}
}

func TestReadRecords_UTF8でないとErrNotUTF8(t *testing.T) {
	t.Parallel()
	// Shift_JIS の「銘柄」（0x96 0xBC 0x95 0xC2）
	content := []byte("code,\x96\xBC\x95\xC2\n7203,Toyota\n")
	got, err := csvinput.ReadRecords(content)
	if !errors.Is(err, csvinput.ErrNotUTF8) {
		t.Fatalf("Shift_JIS: err = %v, want ErrNotUTF8", err)
	}
	if len(got) != 0 {
		t.Errorf("エラーのときはレコードを返さない: got %+v", got)
	}
}

func TestReadRecords_空の行は飛ばすが行番号には数える(t *testing.T) {
	t.Parallel()
	// 1行目が見出し、2行目が空、3行目のレコードの Line は 3。
	got := mustRead(t, "code,name\n\n7203,Toyota\n")
	want := []csvinput.Record{
		{Line: 1, Fields: []string{"code", "name"}},
		{Line: 3, Fields: []string{"7203", "Toyota"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestReadRecords_CRLFの空の行も飛ばして行番号に数える(t *testing.T) {
	t.Parallel()
	got := mustRead(t, "code,name\r\n\r\n7203,Toyota\r\n\r\n6758,Sony\r\n")
	want := []csvinput.Record{
		{Line: 1, Fields: []string{"code", "name"}},
		{Line: 3, Fields: []string{"7203", "Toyota"}},
		{Line: 5, Fields: []string{"6758", "Sony"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestReadRecords_値の中の改行の分だけ次のレコードの行番号が進む(t *testing.T) {
	t.Parallel()
	// 2行目から始まるレコードの値が2〜3行目にまたがる。次のレコードは4行目。
	got := mustRead(t, "code,name\n7203,\"Toyota\nMotor\"\n6758,Sony\n")
	want := []csvinput.Record{
		{Line: 1, Fields: []string{"code", "name"}},
		{Line: 2, Fields: []string{"7203", "Toyota\nMotor"}},
		{Line: 4, Fields: []string{"6758", "Sony"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestReadRecords_CRLFとLFで同じレコードになる(t *testing.T) {
	t.Parallel()
	lf := mustRead(t, "code,name\n7203,Toyota\n6758,Sony\n")
	crlf := mustRead(t, "code,name\r\n7203,Toyota\r\n6758,Sony\r\n")
	want := []csvinput.Record{
		{Line: 1, Fields: []string{"code", "name"}},
		{Line: 2, Fields: []string{"7203", "Toyota"}},
		{Line: 3, Fields: []string{"6758", "Sony"}},
	}
	if !reflect.DeepEqual(lf, want) {
		t.Errorf("LF: got %+v, want %+v", lf, want)
	}
	if !reflect.DeepEqual(crlf, want) {
		t.Errorf("CRLF: got %+v, want %+v", crlf, want)
	}
}

func TestReadRecords_ダブルクォートの値を読める(t *testing.T) {
	t.Parallel()
	got := mustRead(t, "code,name\n\"7203\",\"Toyota, Motor\"\n")
	want := []csvinput.Record{
		{Line: 1, Fields: []string{"code", "name"}},
		{Line: 2, Fields: []string{"7203", "Toyota, Motor"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestReadRecords_ダブルクォートの誤りはQuoteErrorでレコードが始まる行を返す(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		content  string
		wantLine int
	}{
		{"閉じていないダブルクォート（3行目から始まる）", "code,name\n7203,Toyota\n6758,\"Apple\n", 3},
		{"囲んでいない値の中のダブルクォート", "code,name\n72\"03,Toyota\n", 2},
		{"囲んだ値の後ろに文字がある", "code,name\n7203,\"Toyota\"x\n", 2},
		{"誤りが2か所あるときは最初の1か所", "code,name\n72\"03,Toyota\n6758,\"Apple\n", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := csvinput.ReadRecords([]byte(tc.content))
			var qe *csvinput.QuoteError
			if !errors.As(err, &qe) {
				t.Fatalf("err = %v, want *QuoteError", err)
			}
			if qe.Line != tc.wantLine {
				t.Errorf("QuoteError.Line = %d, want %d", qe.Line, tc.wantLine)
			}
			if errors.Is(err, csvinput.ErrNotUTF8) {
				t.Errorf("ダブルクォートの誤りが ErrNotUTF8 になっている")
			}
			if len(got) != 0 {
				t.Errorf("エラーのときはレコードを返さない: got %+v", got)
			}
		})
	}
}

func TestReadRecords_列の数が違う行が混ざっていてもそのまま返す(t *testing.T) {
	t.Parallel()
	got := mustRead(t, "code,name\n7203\n6758,Sony,extra\n")
	want := []csvinput.Record{
		{Line: 1, Fields: []string{"code", "name"}},
		{Line: 2, Fields: []string{"7203"}},
		{Line: 3, Fields: []string{"6758", "Sony", "extra"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
