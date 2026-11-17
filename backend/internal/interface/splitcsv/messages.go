package splitcsv

// 要件定義（docs/requirements/stock-split.md）の「分割の CSV の誤りの文言」の表の文言（「n行目: 」を除いた部分）。

const (
	// ファイルの段階。
	msgMissingArgs = "CSV_FILE を指定してください"
	msgEncoding    = "CSV の文字コードが UTF-8 ではありません"
	msgQuote       = "ダブルクォートの使い方が正しくありません"
	msgHeader      = "1行目の見出しを「銘柄コード,分割の日,分割後の株数」にしてください"
	msgNoRows      = "取り込む行がありません"

	// 各行の段階。
	msgColumns    = "列の数を3にしてください"
	msgStockCode  = "銘柄コードは半角の英大文字・数字・「.」・「-」で1〜10文字にしてください"
	msgSplitDate  = "分割の日は YYYY-MM-DD か YYYY/M/D の実在する日付にしてください"
	msgDateFuture = "未来の分割の日は取り込めません"
	msgRatioSmall = "分割後の株数は2以上の整数にしてください"
	msgRatioLarge = "分割後の株数は1,000以下にしてください"

	// FailedHeadline は、取り込めなかったときの先頭の行。cmd/import-splits の main も、Run を呼ぶ前の失敗で同じ行を出すために使う。
	FailedHeadline = "取り込めませんでした。何も変えていません。"
)

// 見出し（要件15）。この順・この表記に完全に一致しなければならない。
var headerColumns = []string{"銘柄コード", "分割の日", "分割後の株数"}
