package managedcsv

// 要件定義（docs/requirements/managed-account.md）の「CSV の誤りの文言」の表の文言（「n行目: 」を除いた部分）。
// 「元本（評価額 − 損益）」の「−」は U+2212（要件定義と同じ）。

const (
	// ファイルの段階。
	msgMissingArgs = "CSV_FILE と USER_ID を指定してください"
	msgEncoding    = "CSV の文字コードが UTF-8 ではありません"
	msgQuote       = "ダブルクォートの使い方が正しくありません"
	msgHeader      = "1行目の見出しを「名前,日付,評価額,損益」にしてください"
	msgNoRows      = "取り込む行がありません"

	// 各行の段階。
	msgColumns         = "列の数を4にしてください"
	msgName            = "名前を入れてください"
	msgDateFormat      = "日付は YYYY-MM-DD か YYYY/M/D の実在する日付にしてください"
	msgDateFuture      = "未来の日付は取り込めません"
	msgValuationFormat = "評価額は半角数字だけの整数で書いてください"
	msgValuationZero   = "評価額は1以上にしてください"
	msgValuationTooBig = "評価額は1兆未満にしてください"
	msgGainFormat      = "損益は半角数字だけの整数で書いてください（マイナスは先頭に -）"
	msgGainOutOfRange  = "損益は -1兆より大きく1兆未満にしてください"
	msgPrincipalTooLow = "元本（評価額 − 損益）が1円未満です"

	// FailedHeadline は、取り込めなかったときの先頭の行。cmd/import-managed の main も、Run を呼ぶ前の失敗で同じ行を出すために使う。
	FailedHeadline = "取り込めませんでした。何も変えていません。"
)

// 見出し（要件28）。この順・この表記に完全に一致しなければならない。
var headerColumns = []string{"名前", "日付", "評価額", "損益"}
