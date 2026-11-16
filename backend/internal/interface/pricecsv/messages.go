package pricecsv

// 要件定義（docs/requirements/price-import.md）の「CSV の誤りの文言」の表の文言（「n行目: 」を除いた部分）。

const (
	// ファイルの段階。
	msgMissingArgs = "CSV_FILE を指定してください"
	msgEncoding    = "CSV の文字コードが UTF-8 ではありません"
	msgQuote       = "ダブルクォートの使い方が正しくありません"
	msgHeader      = "1行目の見出しを「銘柄コード,終値」にしてください"
	msgNoRows      = "登録する行がありません"

	// 各行の段階。
	msgColumns       = "列の数を2にしてください"
	msgStockCode     = "銘柄コードは半角の英大文字・数字・「.」・「-」で1〜10文字にしてください"
	msgPriceFormat   = "終値は半角数字と小数点で書いてください"
	msgPriceNotPos   = "終値は0より大きくしてください"
	msgPriceTooLarge = "終値は1億未満にしてください"
	msgPriceYen      = "円の終値は小数第1位までにしてください"
	msgPriceDollar   = "ドルの終値は小数第2位までにしてください"

	// FailedHeadline は、登録できなかったときの先頭の行。cmd/import-prices の main も、Run を呼ぶ前の失敗で同じ行を出すために使う。
	FailedHeadline = "登録できませんでした。何も変えていません。"
)

// 見出し（要件5）。この順・この表記に完全に一致しなければならない。
var headerColumns = []string{"銘柄コード", "終値"}
