package integration

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/write"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: 要件（docs/requirements/price-import.md）の要件1〜9・基準1〜5・基準7〜12 と、
// プラン（docs/plans/price-import/plan.md）の 4.1・4.5・5.1（基準の対応）・5.3（補助のテスト）。
// testdb の DB で pricecsv.Run を直接呼び（task を通すのは prices_import_task_test.go の2件だけ）、stdout・stderr の行、終了コード、
// stock_prices（取得日時を含む）・stocks・trades が変わったかを確かめる。今日は 2026-11-16（日本時間。importToday）。
// 前提は givenPricePool（Aの 7203 の売買履歴と、7203 の 3,000円（2026-10-06））。AAPL（ドル）を使うテストは、テストの中で銘柄マスタに入れる。

// price1116 は、2026-11-16 の終値の期待値。
func price1116(code string, hundredths int64) storedClosePrice {
	return storedClosePrice{stockCode: code, tradingDate: importToday, priceHundredths: hundredths}
}

// 基準1（要件1・要件2・要件3）: 終値は 7203 の 3,000円（2026-10-06）だけ。CSV は「7203,2850」「AAPL,182.35」
// → 終了コード0で「2件の価格を登録しました（取引日 2026-11-16）。」の1行だけが表示され（stderr は空）、
// 7203 の終値は 3,000円（2026-10-06）と 2,850円（2026-11-16）の2件、AAPL の終値は 182.35ドル（2026-11-16）の1件になる。
func Test基準1_CSVの終値を今日の取引日で登録する(t *testing.T) {
	t.Parallel()
	pool := givenPricePool(t)
	insertStock(t, pool, "AAPL", "Apple", currencyUSD)

	out := runPriceImportCSV(t, pool, priceCSV("7203,2850", "AAPL,182.35"))

	assertImportSucceeded(t, out, priceRegisteredMessage(2, importToday))
	assertStockPrices(t, pool, []storedClosePrice{
		price7203Before,
		price1116("7203", 285000),
		price1116("AAPL", 18235),
	})
}

// 基準2（要件2）: 終値は 7203 の 2,800円（2026-11-16）と 6758 の 3,500円（2026-10-06）。前提に Aの 7203 の売買履歴も入れる。
// 「7203,2850」だけの CSV を登録する → 7203 の 2026-11-16 の終値は 2,850円の1件になり（行が増えない）、6758 の終値は変わらない。
// 銘柄マスタと売買履歴も変わらない。
func Test基準2_同じ銘柄と取引日の終値は上書きしほかの終値は変えない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertTrade(t, pool, seedUserA, "7203", "buy", 100, "2500", "2026-01-10")
	insertClosePrice(t, pool, "7203", importToday, "2800")
	insertClosePrice(t, pool, "6758", "2026-10-06", "3500")
	before := snapshotPriceDB(t, pool)

	out := runPriceImportCSV(t, pool, priceCSV("7203,2850"))

	assertImportSucceeded(t, out, priceRegisteredMessage(1, importToday))
	assertStockPrices(t, pool, []storedClosePrice{
		{stockCode: "6758", tradingDate: "2026-10-06", priceHundredths: 350000},
		price1116("7203", 285000),
	})
	after := snapshotPriceDB(t, pool)
	var beforeSony, afterSony []storedPriceRow
	for _, p := range before.prices {
		if p.stockCode == "6758" {
			beforeSony = append(beforeSony, p)
		}
	}
	for _, p := range after.prices {
		if p.stockCode == "6758" {
			afterSony = append(afterSony, p)
		}
	}
	if !slices.Equal(afterSony, beforeSony) {
		t.Errorf("6758 の終値（取得日時を含む）が変わった:\n before %+v\n after  %+v", beforeSony, afterSony)
	}
	if !slices.Equal(after.base.stocks, before.base.stocks) {
		t.Errorf("stocks が変わった:\n before %+v\n after  %+v", before.base.stocks, after.base.stocks)
	}
	if !slices.Equal(after.base.trades, before.base.trades) {
		t.Errorf("trades が変わった:\n before %+v\n after  %+v", before.base.trades, after.base.trades)
	}
}

// 基準3（要件1・要件2）: Aが 7203 を「買 2,500円 100株 2026-01-10」で持ち、終値は 3,000円（2026-10-06）だけ。
// 「7203,2850」の CSV を登録してから、Aが一覧を取得する → 7203 の現在の価格は 2,850円、価格の基準日は 2026-11-16。
func Test基準3_登録した終値が一覧の現在の価格になる(t *testing.T) {
	t.Parallel()
	pool := givenPricePool(t)

	out := runPriceImportCSV(t, pool, priceCSV("7203,2850"))

	assertImportSucceeded(t, out, priceRegisteredMessage(1, importToday))
	h := onlyHoldingOf(t, listHoldingsOf(t, pool, seedUserA).Holdings, "7203")
	if h.CurrentPriceHundredths == nil || *h.CurrentPriceHundredths != 285000 {
		t.Errorf("7203 の現在の価格（0.01 単位）: got %v, want 285000（2,850円）", h.CurrentPriceHundredths)
	}
	if h.PriceDate == nil || *h.PriceDate != importToday {
		t.Errorf("7203 の価格の基準日: got %v, want %s", h.PriceDate, importToday)
	}
}

// 基準4（要件6）: 銘柄マスタに XYZ がない。CSV の2行目が「7203,2850」、3行目が「XYZ,10」
// → 終了コード0（stderr は空）で、「1件の価格を登録しました（取引日 2026-11-16）。」「3行目: 銘柄コード XYZ は銘柄マスタにないので飛ばしました」が
// ちょうど2行でこの順に表示され、銘柄マスタに XYZ は足されず、XYZ の終値もない。
func Test基準4_銘柄マスタにない銘柄の行は誤りにせず飛ばす(t *testing.T) {
	t.Parallel()
	pool := givenPricePool(t)
	before := snapshotPriceDB(t, pool)

	out := runPriceImportCSV(t, pool, priceCSV("7203,2850", "XYZ,10"))

	assertImportSucceeded(t, out, priceRegisteredMessage(1, importToday), priceSkippedMessage(3, "XYZ"))
	assertStockPrices(t, pool, []storedClosePrice{price7203Before, price1116("7203", 285000)})
	after := snapshotPriceDB(t, pool)
	if !slices.Equal(after.base.stocks, before.base.stocks) {
		t.Errorf("stocks が変わった（XYZ を銘柄マスタに足さない）:\n before %+v\n after  %+v", before.base.stocks, after.base.stocks)
	}
	if !slices.Equal(after.base.trades, before.base.trades) {
		t.Errorf("trades が変わった:\n before %+v\n after  %+v", before.base.trades, after.base.trades)
	}
}

// 基準5（要件9）: 銘柄マスタに XYZ がない。前提に Aの 7203 の売買履歴と 7203 の 3,000円（2026-10-06）を入れる。「XYZ,10」だけの CSV を登録する
// → 終了コード0（stderr は空）で、「0件の価格を登録しました（取引日 2026-11-16）。」「2行目: 銘柄コード XYZ は銘柄マスタにないので飛ばしました」が
// この順の2行で表示され、DB は何も変わらない。
func Test基準5_すべての行を飛ばしたときは何も変えずに成功する(t *testing.T) {
	t.Parallel()
	pool := givenPricePool(t)
	before := snapshotPriceDB(t, pool)

	out := runPriceImportCSV(t, pool, priceCSV("XYZ,10"))

	assertImportSucceeded(t, out, priceRegisteredMessage(0, importToday), priceSkippedMessage(2, "XYZ"))
	assertPriceDBUnchanged(t, pool, before)
}

// 基準6（要件10）: 見本の CSV（backend/db/seed/prices.example.csv）を登録すると成功する。
// testdb（test.sql に、dev.sql と同じ通貨・名前の 7203・6758 がある）で動かす。dev.sql は流さない（test.sql と銘柄が重なり、主キーの重複で失敗するため）。
// 終了コード0・stderr が空・stdout が「2件の価格を登録しました（取引日 2026-11-16）。」の1行だけ（飛ばした行がない）、
// stock_prices の 2026-11-16 の行が 7203 と 6758 のちょうど2行。
func Test基準6_見本のCSVをそのまま登録できる(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	example := filepath.Join(repositoryRoot(t), "backend", "db", "seed", "prices.example.csv")

	out := runPriceImport(t, pool, example)

	assertImportSucceeded(t, out, priceRegisteredMessage(2, importToday))
	var codes []string
	for _, p := range readStockPrices(t, pool) {
		if p.tradingDate != importToday {
			t.Errorf("取引日が %s でない行がある: %+v", importToday, p)
		}
		codes = append(codes, p.stockCode)
	}
	if want := []string{"6758", "7203"}; !slices.Equal(codes, want) {
		t.Errorf("stock_prices の銘柄コード（%s の行）: got %v, want %v", importToday, codes, want)
	}
}

// 基準7（要件3・要件4・要件5）: 7203 の終値は 3,000円（2026-10-06）だけ。次のどれかで登録する
// (a') CSV_FILE なし（Run に空の CSVPath）(b) 存在しないファイル (c) Shift_JIS で書いた CSV (d) 見出しが「終値,銘柄コード」（順番違い）
// (e) 見出しだけの CSV (e') 見出しと CRLF の空の行だけ (f) 2行目の銘柄コードが `"7203` で閉じていない CSV
// → それぞれ、stderr が「登録できませんでした。何も変えていません。」とファイルの段階の文言1つの2行、stdout が空、終了コード1、DB は変わらない。
// (a) は task を通す（prices_import_task_test.go）。
func Test基準7_ファイルの段階の誤りは文言を1つだけ出して何も変えない(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "missing.csv")
	cases := []struct {
		name    string
		path    func(t *testing.T) string
		wantMsg string
	}{
		{"a_CSV_FILEなし", func(*testing.T) string { return "" }, priceMissingArgsMsg},
		{"b_存在しないファイル", func(*testing.T) string { return missing }, fmt.Sprintf("ファイル %s を開けません", missing)},
		{"c_Shift_JIS", func(t *testing.T) string {
			t.Helper()
			return writePriceCSV(t, []byte(priceShiftJISHeader+"7203,2850\r\n"))
		}, priceEncodingMessage},
		{"d_見出しの順番違い", func(t *testing.T) string {
			t.Helper()
			return writePriceCSV(t, []byte("終値,銘柄コード\n2850,7203\n"))
		}, priceHeaderMessage},
		{"e_見出しだけ", func(t *testing.T) string {
			t.Helper()
			return writePriceCSV(t, priceCSV())
		}, priceNoRowsMessage},
		{"e2_見出しと空の行だけ", func(t *testing.T) string {
			t.Helper()
			return writePriceCSV(t, []byte(priceImportHeader+"\r\n\r\n\r\n"))
		}, priceNoRowsMessage},
		{"f_ダブルクォートが閉じていない", func(t *testing.T) string {
			t.Helper()
			return writePriceCSV(t, priceCSV(`"7203,2850`))
		}, "2行目: " + priceQuoteTailMessage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool := givenPricePool(t)
			before := snapshotPriceDB(t, pool)

			out := runPriceImport(t, pool, tc.path(t))

			assertPriceImportFailed(t, out, tc.wantMsg)
			assertPriceDBUnchanged(t, pool, before)
			assertStockPrices(t, pool, []storedClosePrice{price7203Before})
		})
	}
}

// 基準8（要件4）: BOM 付き・改行 CRLF で、空の行を含み、「"7203","2850"」のようにダブルクォートで囲んだ CSV → 成功し、7203 の 2026-11-16 の終値は 2,850円になる。
func Test基準8_BOM付きCRLFで空の行とダブルクォートを含むCSVを登録できる(t *testing.T) {
	t.Parallel()
	pool := givenPricePool(t)
	content := []byte("\xEF\xBB\xBF" + priceImportHeader + "\r\n\r\n\"7203\",\"2850\"\r\n\r\n")

	out := runPriceImportCSV(t, pool, content)

	assertImportSucceeded(t, out, priceRegisteredMessage(1, importToday))
	assertStockPrices(t, pool, []storedClosePrice{price7203Before, price1116("7203", 285000)})
}

// 基準10（要件3・要件7）: 7203 の終値は 3,000円（2026-10-06）だけ。CSV の2行目は「7203,2850」、3行目は「6758,0」、4行目は「AAPL,182.355,1」
// → 「登録できませんでした。何も変えていません。」「3行目: 終値は0より大きくしてください」「4行目: 列の数を2にしてください」がこの順に表示され、
// 終了コード1で、7203 の終値は 3,000円の1件のまま（2行目の正しい行も登録しない）。
func Test基準10_誤りが1つでもあれば何も登録せず誤りをすべて表示する(t *testing.T) {
	t.Parallel()
	pool := givenPricePool(t)
	before := snapshotPriceDB(t, pool)

	out := runPriceImportCSV(t, pool, priceCSV("7203,2850", "6758,0", "AAPL,182.355,1"))

	assertPriceImportFailed(t, out,
		"3行目: "+priceNotPositiveMsg,
		"4行目: "+priceColumnsMessage,
	)
	assertPriceDBUnchanged(t, pool, before)
	assertStockPrices(t, pool, []storedClosePrice{price7203Before})
}

// 基準11（要件8）: CSV の2行目が「7203,2850」、3行目が別の銘柄の正しい行「6758,3600」、4行目が「7203,2900」。前提に Aの 7203 の売買履歴と 7203 の 3,000円（2026-10-06）を入れる
// → stderr が先頭の行と「4行目: 銘柄コード 7203 は 2行目にもあります」の2行、終了コード1、DB は変わらない。
func Test基準11_同じ銘柄コードの行があれば何も登録しない(t *testing.T) {
	t.Parallel()
	pool := givenPricePool(t)
	before := snapshotPriceDB(t, pool)

	out := runPriceImportCSV(t, pool, priceCSV("7203,2850", "6758,3600", "7203,2900"))

	assertPriceImportFailed(t, out, "4行目: 銘柄コード 7203 は 2行目にもあります")
	assertPriceDBUnchanged(t, pool, before)
}

// 基準12（要件6・要件7）: 銘柄マスタに XYZ がなく、7203 の終値は 3,000円（2026-10-06）だけ。
// (a) 2行目が「7203,2850」、3行目が「XYZ,abc」→ 終了コード1で、stderr が先頭の行と「3行目: 終値は半角数字と小数点で書いてください」の2行、7203 は 3,000円の1件のまま。
// (a2)「XYZ,0」→「3行目: 終値は0より大きくしてください」、(a3)「XYZ,100000000」→「3行目: 終値は1億未満にしてください」（飛ばす行の大きさの確かめ。要件6）。
func Test基準12a_飛ばす行も終値の書き方を確かめ誤りがあれば全体を誤りにする(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		xyzLine string
		wantMsg string
	}{
		{"a_書き方", "XYZ,abc", "3行目: " + priceFormatMessage},
		{"a2_0", "XYZ,0", "3行目: " + priceNotPositiveMsg},
		{"a3_1億", "XYZ,100000000", "3行目: " + priceTooLargeMessage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool := givenPricePool(t)
			before := snapshotPriceDB(t, pool)

			out := runPriceImportCSV(t, pool, priceCSV("7203,2850", tc.xyzLine))

			assertPriceImportFailed(t, out, tc.wantMsg)
			assertPriceDBUnchanged(t, pool, before)
			assertStockPrices(t, pool, []storedClosePrice{price7203Before})
		})
	}
}

// 基準12（b）: 2行目が「7203,2850」、3行目が「XYZ,10.555」→ 成功し（終了コード0）、7203 の 2026-11-16 の終値は 2,850円になり、
// stdout が「1件の価格を登録しました（取引日 2026-11-16）。」「3行目: 銘柄コード XYZ は銘柄マスタにないので飛ばしました」のちょうど2行になる（小数の桁は確かめない）。
func Test基準12b_飛ばす行の終値の小数の桁は確かめない(t *testing.T) {
	t.Parallel()
	pool := givenPricePool(t)

	out := runPriceImportCSV(t, pool, priceCSV("7203,2850", "XYZ,10.555"))

	assertImportSucceeded(t, out, priceRegisteredMessage(1, importToday), priceSkippedMessage(3, "XYZ"))
	assertStockPrices(t, pool, []storedClosePrice{price7203Before, price1116("7203", 285000)})
}

// 5.3「書き方の誤りと桁の誤りが混ざる」: 2行目「AAPL,182.355」（ドルの桁）、3行目「6758,0」（書き方）、4行目「7203,2850.25」（円の桁）
// → stderr は先頭の行、「2行目: ドルの終値は小数第2位までにしてください」、「3行目: 終値は0より大きくしてください」、
// 「4行目: 円の終値は小数第1位までにしてください」の順（Run が2か所の誤りを行の順に並べる）。DB は変わらない。
func Test補助_書き方の誤りと桁の誤りは行の順に並べて表示する(t *testing.T) {
	t.Parallel()
	pool := givenPricePool(t)
	insertStock(t, pool, "AAPL", "Apple", currencyUSD)
	before := snapshotPriceDB(t, pool)

	out := runPriceImportCSV(t, pool, priceCSV("AAPL,182.355", "6758,0", "7203,2850.25"))

	assertPriceImportFailed(t, out,
		"2行目: "+priceDollarFineMsg,
		"3行目: "+priceNotPositiveMsg,
		"4行目: "+priceYenFineMessage,
	)
	assertPriceDBUnchanged(t, pool, before)
}

// 5.3「段階の順」: 「7203,2850」「7203,2900」「6758,abc」→ stderr は先頭の行と「4行目: 終値は半角数字と小数点で書いてください」だけで、
// 同じ銘柄コードの誤りは出さない（各行の段階で止まる）。
func Test補助_各行の段階に誤りがあれば同じ銘柄コードの誤りは出さない_書き方(t *testing.T) {
	t.Parallel()
	pool := givenPricePool(t)
	before := snapshotPriceDB(t, pool)

	out := runPriceImportCSV(t, pool, priceCSV("7203,2850", "7203,2900", "6758,abc"))

	assertPriceImportFailed(t, out, "4行目: "+priceFormatMessage)
	assertPriceDBUnchanged(t, pool, before)
}

// 5.3「段階の順」: 「7203,2850」「7203,2900」「AAPL,182.355」→「4行目: ドルの終値は小数第2位までにしてください」だけ
// （精度の誤りも各行の段階なので、同じ銘柄コードの誤りは出さない）。
func Test補助_各行の段階に誤りがあれば同じ銘柄コードの誤りは出さない_桁(t *testing.T) {
	t.Parallel()
	pool := givenPricePool(t)
	insertStock(t, pool, "AAPL", "Apple", currencyUSD)
	before := snapshotPriceDB(t, pool)

	out := runPriceImportCSV(t, pool, priceCSV("7203,2850", "7203,2900", "AAPL,182.355"))

	assertPriceImportFailed(t, out, "4行目: "+priceDollarFineMsg)
	assertPriceDBUnchanged(t, pool, before)
}

// 5.3「同じ銘柄コードは銘柄マスタにない行も含む」（要件8）: 「XYZ,10」が2行 →「3行目: 銘柄コード XYZ は 2行目にもあります」。
func Test補助_銘柄マスタにない銘柄コードの重複も誤りにする(t *testing.T) {
	t.Parallel()
	pool := givenPricePool(t)
	before := snapshotPriceDB(t, pool)

	out := runPriceImportCSV(t, pool, priceCSV("XYZ,10", "XYZ,10"))

	assertPriceImportFailed(t, out, "3行目: 銘柄コード XYZ は 2行目にもあります")
	assertPriceDBUnchanged(t, pool, before)
}

// 5.3「行番号は空の行も数える」（統合）: 見出し、空の行、「XYZ,10」→ stdout の2行目が「3行目: 銘柄コード XYZ は銘柄マスタにないので飛ばしました」。
func Test補助_行番号は空の行も数える(t *testing.T) {
	t.Parallel()
	pool := givenPricePool(t)

	out := runPriceImportCSV(t, pool, []byte(priceImportHeader+"\n\nXYZ,10\n"))

	assertImportSucceeded(t, out, priceRegisteredMessage(0, importToday), priceSkippedMessage(3, "XYZ"))
}

// 5.3「飛ばした行の並び」（要件6）: 「ZZZ,5」「7203,2850」「XYZ,10」→ stdout が「1件の価格を登録しました（取引日 2026-11-16）。」
// 「2行目: 銘柄コード ZZZ は銘柄マスタにないので飛ばしました」「4行目: 銘柄コード XYZ は銘柄マスタにないので飛ばしました」の順（行番号の順。銘柄コードの順と違う）。
func Test補助_飛ばした行は行番号の昇順に表示する(t *testing.T) {
	t.Parallel()
	pool := givenPricePool(t)

	out := runPriceImportCSV(t, pool, priceCSV("ZZZ,5", "7203,2850", "XYZ,10"))

	assertImportSucceeded(t, out,
		priceRegisteredMessage(1, importToday),
		priceSkippedMessage(2, "ZZZ"),
		priceSkippedMessage(4, "XYZ"),
	)
}

// 5.3「Run は受け取った今日を使う」（要件1・要件3）: Today に "2026-11-13" を渡して「7203,2850」を登録
// → stdout が「1件の価格を登録しました（取引日 2026-11-13）。」、7203 の終値が 2026-11-13 の 2,850円（実際の今日の日付を使う誤りを見つけるため）。
func Test補助_Runは受け取った今日を取引日にする(t *testing.T) {
	t.Parallel()
	pool := givenPricePool(t)
	const otherDay = "2026-11-13"

	out := runPriceImportOn(t, pool, writePriceCSV(t, priceCSV("7203,2850")), otherDay)

	assertImportSucceeded(t, out, priceRegisteredMessage(1, otherDay))
	assertStockPrices(t, pool, []storedClosePrice{
		price7203Before,
		{stockCode: "7203", tradingDate: otherDay, priceHundredths: 285000},
	})
}

// 5.3「取得日時」（要件2）: 7203 の 11-16 の終値を取得日時 2026-01-01 00:00（日本時間）で入れておく。Run の直前に同じ DB で `SELECT now()` した時刻を起点にし、
// CSV の「7203,2850」「6758,3600」を登録すると、上書きした 7203 の行も、足した 6758 の行も、取得日時が起点以後になる（テストのプロセスの時計とは比べない）。
func Test補助_登録した行の取得日時は登録した日時になる(t *testing.T) {
	t.Parallel()
	pool := givenPricePool(t)
	insertClosePrice(t, pool, "7203", importToday, "2800")
	if _, err := pool.Exec(t.Context(),
		"UPDATE stock_prices SET fetched_at = '2026-01-01 00:00:00+09'::timestamptz WHERE stock_code = '7203' AND trading_date = $1::text::date", importToday); err != nil {
		t.Fatalf("取得日時を入れられない: %v", err)
	}
	var start time.Time
	if err := pool.QueryRow(t.Context(), "SELECT now()").Scan(&start); err != nil {
		t.Fatalf("DB の時刻を読めない: %v", err)
	}

	out := runPriceImportCSV(t, pool, priceCSV("7203,2850", "6758,3600"))

	assertImportSucceeded(t, out, priceRegisteredMessage(2, importToday))
	rows, err := pool.Query(t.Context(),
		"SELECT stock_code, fetched_at FROM stock_prices WHERE trading_date = $1::text::date ORDER BY stock_code", importToday)
	if err != nil {
		t.Fatalf("stock_prices を読めない: %v", err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var code string
		var fetchedAt time.Time
		if err := rows.Scan(&code, &fetchedAt); err != nil {
			t.Fatalf("stock_prices を読めない: %v", err)
		}
		seen[code] = true
		if fetchedAt.Before(start) {
			t.Errorf("%s の取得日時: got %v, want %v 以後（登録した日時にする）", code, fetchedAt, start)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("stock_prices を読めない: %v", err)
	}
	if !seen["7203"] || !seen["6758"] {
		t.Errorf("%s の終値がある銘柄: got %v, want 7203 と 6758", importToday, seen)
	}
}

// 5.3「1つのトランザクション」（要件2）: SaveAll を直接呼ぶ。1件目は正しい終値、2件目は銘柄マスタにない銘柄の終値
// → エラーになり、1件目も保存されない（stock_prices は呼ぶ前のまま）。
func Test補助_SaveAllは途中で失敗したら何も保存しない(t *testing.T) {
	t.Parallel()
	pool := givenPricePool(t)
	before := snapshotPriceDB(t, pool)
	repo := write.NewClosingPriceImportRepository(pool)

	err := repo.SaveAll(t.Context(), []model.ClosingPrice{
		{StockCode: "7203", TradingDate: importToday, PriceHundredths: 285000},
		{StockCode: "XYZ", TradingDate: importToday, PriceHundredths: 1000},
	})

	if err == nil {
		t.Error("SaveAll（2件目が銘柄マスタにない銘柄）: got err nil, want エラー")
	}
	assertPriceDBUnchanged(t, pool, before)
}
