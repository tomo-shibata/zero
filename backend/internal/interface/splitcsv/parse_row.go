package splitcsv

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/interface/csvinput"
)

// CSV の列の位置（見出しの順。要件15）。
const (
	colStockCode = iota
	colSplitDate
	colRatio
	columnCount
)

var (
	// ratioFormat は、分割後の株数の書き方（要件16）。半角数字だけ。
	ratioFormat = regexp.MustCompile(`^[0-9]+$`)
	// hyphenDateFormat・slashDateFormat は、分割の日の書き方（要件16）。ハイフンの形は月と日の0埋めが要り、スラッシュの形は0埋めが任意。
	hyphenDateFormat = regexp.MustCompile(`^([0-9]{4})-([0-9]{2})-([0-9]{2})$`)
	slashDateFormat  = regexp.MustCompile(`^([0-9]{4})/([0-9]{1,2})/([0-9]{1,2})$`)
)

// parseRow は、分割の行 rec を確かめて model.StockSplit にする（各行の段階）。
// 列の数が3でなければ、列の数の誤りだけを返す。そうでなければ、すべての列を確かめ、誤りの文言（「n行目: 」なし）を
// 銘柄コード・分割の日・分割後の株数の順に（1つの列では最初に当たった1つだけ）返す。誤りがなければ文言は空で、分割を返す。
// 3つ目の戻り値は、確かめを通ったのに分割を組み立てられないという、起こらないはずの失敗。
func parseRow(rec csvinput.Record, today string) (model.StockSplit, []string, error) {
	if len(rec.Fields) != columnCount {
		return model.StockSplit{}, []string{msgColumns}, nil
	}
	f := rec.Fields

	var texts []string
	if err := model.CheckStockCode(f[colStockCode]); err != nil {
		texts = append(texts, msgStockCode)
	}
	date, dateText := parseSplitDate(f[colSplitDate], today)
	if dateText != "" {
		texts = append(texts, dateText)
	}
	ratio, ratioText := parseRatio(f[colRatio])
	if ratioText != "" {
		texts = append(texts, ratioText)
	}
	if len(texts) > 0 {
		return model.StockSplit{}, texts, nil
	}

	split, err := model.NewStockSplit(f[colStockCode], date, ratio, today)
	if err != nil {
		return model.StockSplit{}, nil, fmt.Errorf("%d行目の分割を作れません: %w", rec.Line, err)
	}
	return split, nil, nil
}

// parseSplitDate は、分割の日の列を読んで "YYYY-MM-DD" にそろえる。誤りなら、2つ目の戻り値に文言を返す。
// 書き方と実在する日付かを確かめてから、今日（today）より後でないかを確かめる。
func parseSplitDate(s, today string) (string, string) {
	m := hyphenDateFormat.FindStringSubmatch(s)
	if m == nil {
		m = slashDateFormat.FindStringSubmatch(s)
	}
	if m == nil {
		return "", msgSplitDate
	}
	// 桁は正規表現が4桁・1〜2桁に限っているので、Atoi は失敗しない。
	year, _ := strconv.Atoi(m[1])
	month, _ := strconv.Atoi(m[2])
	day, _ := strconv.Atoi(m[3])
	date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	// time.Date は 2月30日を3月2日などに直すので、直されていない（実在する日付）ことを確かめる。年は1以上（DB の分割の日の範囲）。
	if year < 1 || date.Year() != year || int(date.Month()) != month || date.Day() != day {
		return "", msgSplitDate
	}
	normalized := date.Format(time.DateOnly)
	if err := model.CheckSplitDate(normalized, today); err != nil {
		return "", msgDateFuture
	}
	return normalized, ""
}

// parseRatio は、分割後の株数の列を読む。誤りなら、2つ目の戻り値に文言を返す。
// 半角数字だけで int64 に収まらない値は、「1,000以下」の誤りにする。
func parseRatio(s string) (int64, string) {
	if !ratioFormat.MatchString(s) {
		return 0, msgRatioSmall
	}
	ratio, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return 0, msgRatioLarge
		}
		return 0, msgRatioSmall
	}
	if err := model.CheckSplitRatio(ratio); err != nil {
		if errors.Is(err, model.ErrSplitRatioTooSmall) {
			return 0, msgRatioSmall
		}
		return 0, msgRatioLarge
	}
	return ratio, ""
}
