package managedcsv

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/interface/csvinput"
)

// CSV の列の位置（見出しの順。要件28）。
const (
	colName = iota
	colDate
	colValuation
	colGain
	columnCount
)

var (
	// valuationFormat は、評価額の書き方（要件29）。半角数字だけ。
	valuationFormat = regexp.MustCompile(`^[0-9]+$`)
	// gainFormat は、損益の書き方（要件29）。半角数字だけで、先頭に + か - を1つだけ付けてもよい。
	gainFormat = regexp.MustCompile(`^[+-]?[0-9]+$`)
	// hyphenDateFormat・slashDateFormat は、日付の書き方（要件29）。ハイフンの形は月と日の0埋めが要り、スラッシュの形は0埋めが任意。
	hyphenDateFormat = regexp.MustCompile(`^([0-9]{4})-([0-9]{2})-([0-9]{2})$`)
	slashDateFormat  = regexp.MustCompile(`^([0-9]{4})/([0-9]{1,2})/([0-9]{1,2})$`)
)

// parseRow は、記録の行 rec を確かめて model.ManagedValuation にする（各行の段階）。
// 列の数が4でなければ、列の数の誤りだけを返す。そうでなければ、すべての列を確かめ、誤りの文言（「n行目: 」なし）を
// 名前・日付・評価額・損益・元本の順に（1つの列では最初に当たった1つだけ）返す。誤りがなければ文言は空で、記録を返す。
// 元本は、評価額と損益がどちらも正しいときだけ確かめる。
// 3つ目の戻り値は、確かめを通ったのに記録を組み立てられないという、起こらないはずの失敗。
func parseRow(rec csvinput.Record, today string) (model.ManagedValuation, []string, error) {
	if len(rec.Fields) != columnCount {
		return model.ManagedValuation{}, []string{msgColumns}, nil
	}
	f := rec.Fields

	var texts []string
	if err := model.CheckManagedAccountName(f[colName]); err != nil {
		texts = append(texts, msgName)
	}
	date, dateText := parseValuationDate(f[colDate], today)
	if dateText != "" {
		texts = append(texts, dateText)
	}
	valuation, valuationText := parseValuation(f[colValuation])
	if valuationText != "" {
		texts = append(texts, valuationText)
	}
	gain, gainText := parseGain(f[colGain])
	if gainText != "" {
		texts = append(texts, gainText)
	}
	if valuationText == "" && gainText == "" {
		if err := model.CheckPrincipal(valuation, gain); err != nil {
			texts = append(texts, msgPrincipalTooLow)
		}
	}
	if len(texts) > 0 {
		return model.ManagedValuation{}, texts, nil
	}

	v, err := model.NewManagedValuation(f[colName], date, valuation, gain, today)
	if err != nil {
		return model.ManagedValuation{}, nil, fmt.Errorf("%d行目の記録を作れません: %w", rec.Line, err)
	}
	return v, nil, nil
}

// parseValuationDate は、日付の列を読んで "YYYY-MM-DD" にそろえる。誤りなら、2つ目の戻り値に文言を返す。
// 書き方と実在する日付かを確かめてから、今日（today）より後でないかを確かめる。
func parseValuationDate(s, today string) (string, string) {
	m := hyphenDateFormat.FindStringSubmatch(s)
	if m == nil {
		m = slashDateFormat.FindStringSubmatch(s)
	}
	if m == nil {
		return "", msgDateFormat
	}
	// 桁は正規表現が4桁・1〜2桁に限っているので、Atoi は失敗しない。
	year, _ := strconv.Atoi(m[1])
	month, _ := strconv.Atoi(m[2])
	day, _ := strconv.Atoi(m[3])
	date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	// time.Date は 2月30日を3月2日などに直すので、直されていない（実在する日付）ことを確かめる。年は1以上（DB の日付の範囲）。
	if year < 1 || date.Year() != year || int(date.Month()) != month || date.Day() != day {
		return "", msgDateFormat
	}
	normalized := date.Format(time.DateOnly)
	if err := model.CheckValuationDate(normalized, today); err != nil {
		return "", msgDateFuture
	}
	return normalized, ""
}

// parseValuation は、評価額の列を読む。誤りなら、2つ目の戻り値に文言を返す。
// 半角数字だけで int64 に収まらない値は、「1兆未満」の誤りにする。
func parseValuation(s string) (int64, string) {
	if !valuationFormat.MatchString(s) {
		return 0, msgValuationFormat
	}
	valuation, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return 0, msgValuationTooBig
		}
		return 0, msgValuationFormat
	}
	if err := model.CheckValuation(valuation); err != nil {
		if errors.Is(err, model.ErrValuationNotPositive) {
			return 0, msgValuationZero
		}
		return 0, msgValuationTooBig
	}
	return valuation, ""
}

// parseGain は、損益の列を読む。誤りなら、2つ目の戻り値に文言を返す。
// 書き方に合っていて int64 に収まらない値は、「-1兆より大きく1兆未満」の誤りにする。
func parseGain(s string) (int64, string) {
	if !gainFormat.MatchString(s) {
		return 0, msgGainFormat
	}
	gain, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return 0, msgGainOutOfRange
		}
		return 0, msgGainFormat
	}
	if err := model.CheckGain(gain); err != nil {
		return 0, msgGainOutOfRange
	}
	return gain, ""
}
