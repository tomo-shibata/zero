import { describe, expect, test } from 'vitest'
import { closeTooltip, tradeTooltipLines } from '../../../src/features/stock-history/lib/tooltipLines'
import { close, trade } from './fixtures'

// 仕様: 要件 docs/requirements/stock-history.md の要件10・要件11、基準10・基準11・基準12（単体）。
//   要件10: 線の吹き出しは「<取引日> <終値>」。要件9 で割った終値は、売買履歴の要件7 の取得価格と同じ丸め方（API が割った価格を返す）で出す。
//   要件11: 印の吹き出しは「<取引日> <買 か 売> <株数> <価格>」。株数と価格は表と同じ実際の値。割った印は、続けて「分割を反映した価格 <割った価格>」。
// 境界: プラン docs/plans/stock-history/plan.md 4.8「PR② 画面とカードからの移動」の features/stock-history/lib/tooltipLines.ts
//   closeTooltip(currency, close): string  … 「2026-10-06 3,000円」（adjustedPrice を使う）
//   tradeTooltipLines(currency, trades): string[]  … 渡した順に「<取引日> 買 100株 2,500円」。割った売買は続けて「分割を反映した価格 833.3円」
// テスト計画: プラン 5.1 の基準10・基準11・基準12 の ② の行。

describe('closeTooltip（線の吹き出し。基準10・基準12。要件10）', () => {
  test('基準10: AAPL の 2026-10-06 の終値 180.00ドルは「2026-10-06 180.00ドル」', () => {
    expect(closeTooltip('USD', close('2026-10-06', '180.00'))).toBe('2026-10-06 180.00ドル')
  })

  test('基準10（要件10 の例）: 7203 の 2026-10-06 の終値 3,000円は「2026-10-06 3,000円」', () => {
    expect(closeTooltip('JPY', close('2026-10-06', '3000.0'))).toBe('2026-10-06 3,000円')
  })

  test('基準12: 分割で割った終値（3100.0 を 3 で割った 1033.3）は、割った価格を使って「2026-03-30 1,033.3円」', () => {
    const splitClose = close('2026-03-30', '3100.0', { splitDivisor: '3', adjustedPrice: '1033.3' })

    expect(closeTooltip('JPY', splitClose)).toBe('2026-03-30 1,033.3円')
  })

  test('補助 要件10: ドルの割った終値は小数2桁のまま。150.00 を 10 で割った 15.00 は「2026-01-10 15.00ドル」', () => {
    const splitClose = close('2026-01-10', '150.00', { splitDivisor: '10', adjustedPrice: '15.00' })

    expect(closeTooltip('USD', splitClose)).toBe('2026-01-10 15.00ドル')
  })
})

describe('tradeTooltipLines（印の吹き出し。基準11・基準12。要件11）', () => {
  test('基準11: 2026-01-10 の印は、渡した順（後に登録した 10株 2,520円が先）に「2026-01-10 買 10株 2,520円」「2026-01-10 買 100株 2,500円」', () => {
    const lines = tradeTooltipLines('JPY', [
      trade('buy', '2026-01-10', 10, '2520.0'),
      trade('buy', '2026-01-10', 100, '2500.0'),
    ])

    expect(lines).toEqual(['2026-01-10 買 10株 2,520円', '2026-01-10 買 100株 2,500円'])
  })

  test('基準11: 渡した順のまま出す（順を並べ替えない）。100株を先に渡せば、100株が先', () => {
    const lines = tradeTooltipLines('JPY', [
      trade('buy', '2026-01-10', 100, '2500.0'),
      trade('buy', '2026-01-10', 10, '2520.0'),
    ])

    expect(lines).toEqual(['2026-01-10 買 100株 2,500円', '2026-01-10 買 10株 2,520円'])
  })

  test('基準12: 分割で割った印（2,500円・割る数 3）は「2026-01-10 買 100株 2,500円」に続けて「分割を反映した価格 833.3円」', () => {
    const lines = tradeTooltipLines('JPY', [
      trade('buy', '2026-01-10', 100, '2500.0', { splitDivisor: '3', adjustedPrice: '833.3' }),
    ])

    expect(lines).toEqual(['2026-01-10 買 100株 2,500円', '分割を反映した価格 833.3円'])
  })

  test('基準12: 分割の日と同じ日の印（割る数 1）は「2026-03-31 買 30株 1,000円」の1行だけ', () => {
    const lines = tradeTooltipLines('JPY', [trade('buy', '2026-03-31', 30, '1000.0')])

    expect(lines).toEqual(['2026-03-31 買 30株 1,000円'])
  })

  test('要件11: 売りは「売」。株数は3桁区切り。例 売 1,000株 3,050円', () => {
    const lines = tradeTooltipLines('JPY', [trade('sell', '2026-03-02', 1000, '3050.0')])

    expect(lines).toEqual(['2026-03-02 売 1,000株 3,050円'])
  })

  test('要件11: ドルは小数2桁。買 10株 150.25ドルは「2026-01-10 買 10株 150.25ドル」。割った印は「分割を反映した価格 15.03ドル」が続く', () => {
    const lines = tradeTooltipLines('USD', [
      trade('buy', '2026-01-10', 10, '150.25', { currency: 'USD', splitDivisor: '10', adjustedPrice: '15.03' }),
    ])

    expect(lines).toEqual(['2026-01-10 買 10株 150.25ドル', '分割を反映した価格 15.03ドル'])
  })

  test('補助 要件11: 割った売買と割らない売買が混ざるとき、割った売買の行の直後にだけ「分割を反映した価格」が付く', () => {
    const lines = tradeTooltipLines('JPY', [
      trade('buy', '2026-01-10', 10, '2520.0', { splitDivisor: '3', adjustedPrice: '840.0' }),
      trade('buy', '2026-01-10', 100, '2500.0'),
    ])

    expect(lines).toEqual(['2026-01-10 買 10株 2,520円', '分割を反映した価格 840円', '2026-01-10 買 100株 2,500円'])
  })

  test('補助: 売買がなければ行もない', () => {
    expect(tradeTooltipLines('JPY', [])).toEqual([])
  })
})
