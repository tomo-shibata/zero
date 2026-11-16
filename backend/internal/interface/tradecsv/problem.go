package tradecsv

import "fmt"

// Problem は、利用者に見せる誤り1件。Line はファイルの段階の誤りなら0。
// Message は要件定義の「CSV の誤りの文言」のとおり（行の誤りは「n行目: 」を含む）。
type Problem struct {
	Line    int
	Message string
}

// ParseError は、段階1（ファイル全体）か段階2（各行の書き方）の誤り。Problems は並べる順（要件定義のとおり）。
type ParseError struct{ Problems []Problem }

// Error は、誤りの件数を返す。利用者に見せる文言は Problems の Message。
func (e *ParseError) Error() string {
	return fmt.Sprintf("CSV に誤りが %d 件あります", len(e.Problems))
}

// lineProblem は、n行目の誤り（文言は「n行目: <text>」）を作る。
func lineProblem(line int, text string) Problem {
	return Problem{Line: line, Message: fmt.Sprintf("%d行目: %s", line, text)}
}

// fileProblem は、行番号のないファイルの段階の誤りを作る。
func fileProblem(text string) Problem {
	return Problem{Message: text}
}
