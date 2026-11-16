package tradecsv

// 要件定義の「CSV の誤りの文言」の表の文言（「n行目: 」を除いた部分）。

const (
	// ファイルの段階。
	msgMissingArgs = "CSV_FILE と USER_ID を指定してください"
	msgEncoding    = "CSV の文字コードが UTF-8 ではありません"
	msgQuote       = "ダブルクォートの使い方が正しくありません"
	msgHeader      = "1行目の見出しを「銘柄コード,銘柄名,通貨,売買,価格,株数,取引日」にしてください"
	msgNoRows      = "取り込む行がありません"

	// 各行の段階。
	msgColumns       = "列の数を7にしてください"
	msgStockCode     = "銘柄コードは半角の英大文字・数字・「.」・「-」で1〜10文字にしてください"
	msgStockName     = "銘柄名を入れてください"
	msgCurrency      = "通貨は JPY か USD にしてください"
	msgSide          = "売買は 買 か 売 にしてください"
	msgPriceFormat   = "価格は半角数字と小数点で書いてください"
	msgPriceNotPos   = "価格は0より大きくしてください"
	msgPriceTooLarge = "価格は1億未満にしてください"
	msgPriceYen      = "円の価格は小数第1位までにしてください"
	msgPriceDollar   = "ドルの価格は小数第2位までにしてください"
	msgQtyFormat     = "株数は1以上の整数にしてください"
	msgQtyTooLarge   = "株数は1,000万株未満にしてください"
	msgDateFormat    = "取引日は YYYY-MM-DD か YYYY/M/D の実在する日付にしてください"
	msgDateFuture    = "未来の取引日は取り込めません"

	// 取り込めなかったときの先頭の行。
	failedHeadline = "取り込めませんでした。何も変えていません。"
)

// 見出し（要件23）。この順・この表記に完全に一致しなければならない。
var headerColumns = []string{"銘柄コード", "銘柄名", "通貨", "売買", "価格", "株数", "取引日"}

// 売買の列の値（要件24）。
const (
	sideBuyText  = "買"
	sideSellText = "売"
)
