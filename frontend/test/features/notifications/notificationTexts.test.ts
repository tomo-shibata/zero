import { describe, expect, test } from 'vitest'
import {
  formatSignedRate,
  notificationDetail,
  notificationHeadline,
} from '../../../src/features/notifications/lib/notificationTexts'
import {
  APPLE_PREV,
  SONY_PEAK,
  SONY_PNL,
  SOFTBANK_PRICE,
  TOYOTA_COST,
  notification,
} from './fixtures'

// 仕様: 価格の変動の通知の要件 docs/requirements/price-alerts.md の要件32（「何が起きたか」と「前後の値」の表）・要件33（率の書式と色の向き。金額・価格の書式）と、
//   成功基準の基準35（5件の文字と色）。
// 境界: プラン docs/plans/price-alerts/plan.md 4.8 の PR②「features/notifications/lib/notificationTexts.ts」
//   - formatSignedRate(rate: string): string  "-20.4" → "-20.4%"、"6.1" → "+6.1%"、"0.0" → "0.0%"
//   - notificationHeadline(n): { label: string; value: string; tone: 'gain' | 'loss' | 'none' }
//       例 { label: '取得価格から ', value: '-20.4%', tone: 'loss' }（label は種類の言葉の後ろに空白を付けた形。label + value が「何が起きたか」）
//   - notificationDetail(n): string  「前後の値」（要件32 の表）。例「取得価格 2,500円 → 終値 1,990円」
//   色: 率と含み損益の文字は、プラスなら 'gain'（赤）、マイナスなら 'loss'（青）、0なら 'none'。決めた価格の通知の終値は 'none'（要件33）。
//   書式: 円の価格は FR-12（小数第1位が0なら整数。3桁区切り）、ドルは小数第2位まで、含み損益は符号付き（プラスは「+」）、株数は「200株」。
// テスト計画: プラン 5.1 の基準35（単体）の行（5種類の「何が起きたか」「前後の値」）。

describe('formatSignedRate（要件33: プラスは +、マイナスは -、0は符号なしで小数第1位まで）', () => {
  test.each([
    { rate: '-20.4', expected: '-20.4%' },
    { rate: '6.1', expected: '+6.1%' },
    { rate: '0.0', expected: '0.0%' },
    { rate: '-0.5', expected: '-0.5%' },
    { rate: '20.0', expected: '+20.0%' },
  ])('"$rate" は「$expected」になる', ({ rate, expected }) => {
    expect(formatSignedRate(rate)).toBe(expected)
  })
})

describe('notificationHeadline（要件32 の「何が起きたか」・要件33 の色の向き）', () => {
  test('基準35: 取得価格から -20.4% は、label「取得価格から 」・value「-20.4%」・tone「loss」（プランの例）', () => {
    expect(notificationHeadline(TOYOTA_COST)).toEqual({ label: '取得価格から ', value: '-20.4%', tone: 'loss' })
  })

  test('基準35: 前の取引日から +6.1% は、「前の取引日から +6.1%」で tone「gain」', () => {
    const headline = notificationHeadline(APPLE_PREV)

    expect(headline.label + headline.value).toBe('前の取引日から +6.1%')
    expect(headline.value).toBe('+6.1%')
    expect(headline.tone).toBe('gain')
  })

  test('基準35: 最高値から -15.2% は、「最高値から -15.2%」で tone「loss」', () => {
    const headline = notificationHeadline(SONY_PEAK)

    expect(headline.label + headline.value).toBe('最高値から -15.2%')
    expect(headline.value).toBe('-15.2%')
    expect(headline.tone).toBe('loss')
  })

  test('基準35: 含み損益の金額（円・-104,000円）は、「含み損益 -104,000円」で tone「loss」', () => {
    const headline = notificationHeadline(SONY_PNL)

    expect(headline.label + headline.value).toBe('含み損益 -104,000円')
    expect(headline.value).toBe('-104,000円')
    expect(headline.tone).toBe('loss')
  })

  test('基準35: 決めた価格の通知は、「終値 8,120円」で tone「none」（終値には色を付けない）', () => {
    const headline = notificationHeadline(SOFTBANK_PRICE)

    expect(headline.label + headline.value).toBe('終値 8,120円')
    expect(headline.value).toBe('8,120円')
    expect(headline.tone).toBe('none')
  })

  test('要件33: 含み損益の金額（ドル・含み益 300.00ドル）は、プラスに「+」を付けて「含み損益 +300.00ドル」で tone「gain」', () => {
    const n = notification({
      stockCode: 'AAPL',
      stockName: 'Apple',
      currency: 'USD',
      condition: { kind: 'pnl', scope: 'stock', stockCode: 'AAPL', direction: 'gain', threshold: '300.00' },
      close: '180.00',
      basePrice: '150.00',
      unrealizedGain: '300.00',
      quantity: 10,
    })

    const headline = notificationHeadline(n)

    expect(headline.label + headline.value).toBe('含み損益 +300.00ドル')
    expect(headline.tone).toBe('gain')
  })

  test('要件33: 率が 0.0 のとき（取得価格から）は符号を付けず「取得価格から 0.0%」で tone「none」', () => {
    const n = notification({ rate: '0.0', basePrice: '1990.0' })

    const headline = notificationHeadline(n)

    expect(headline.label + headline.value).toBe('取得価格から 0.0%')
    expect(headline.tone).toBe('none')
  })

  test('要件33: ドルの終値（決めた価格・ドル）は「終値 149.00ドル」で tone「none」', () => {
    const n = notification({
      stockCode: 'AAPL',
      stockName: 'Apple',
      currency: 'USD',
      condition: { kind: 'price', scope: 'stock', stockCode: 'AAPL', direction: 'below', threshold: '150.25' },
      close: '149.00',
    })

    const headline = notificationHeadline(n)

    expect(headline.label + headline.value).toBe('終値 149.00ドル')
    expect(headline.tone).toBe('none')
  })
})

describe('notificationDetail（要件32 の「前後の値」）', () => {
  test('基準35: 取得価格から は「取得価格 2,500円 → 終値 1,990円」', () => {
    expect(notificationDetail(TOYOTA_COST)).toBe('取得価格 2,500円 → 終値 1,990円')
  })

  test('基準35: 最高値から は「最高値 4,000円（2026-09-30）→ 終値 3,392円」', () => {
    expect(notificationDetail(SONY_PEAK)).toBe('最高値 4,000円（2026-09-30）→ 終値 3,392円')
  })

  test('基準35: 前の取引日から（ドル）は「前の終値 180.25ドル（2026-11-13）→ 終値 191.24ドル」', () => {
    expect(notificationDetail(APPLE_PREV)).toBe('前の終値 180.25ドル（2026-11-13）→ 終値 191.24ドル')
  })

  test('基準35: 含み損益の金額は「取得価格 4,000円 → 終値 3,480円（200株）」', () => {
    expect(notificationDetail(SONY_PNL)).toBe('取得価格 4,000円 → 終値 3,480円（200株）')
  })

  test('基準35: 決めた価格（以上）は「決めた価格 8,000円 以上」', () => {
    expect(notificationDetail(SOFTBANK_PRICE)).toBe('決めた価格 8,000円 以上')
  })

  test('要件32: 決めた価格（以下・ドル）は「決めた価格 150.25ドル 以下」', () => {
    const n = notification({
      stockCode: 'AAPL',
      stockName: 'Apple',
      currency: 'USD',
      condition: { kind: 'price', scope: 'stock', stockCode: 'AAPL', direction: 'below', threshold: '150.25' },
      close: '149.00',
    })

    expect(notificationDetail(n)).toBe('決めた価格 150.25ドル 以下')
  })

  test('要件32: 決めた価格を分割で割ったときは「決めた価格 4,000円 以上（分割に合わせた値。足したときは 8,000円）」', () => {
    const n = notification({
      stockCode: '7203',
      condition: { kind: 'price', scope: 'stock', stockCode: '7203', direction: 'above', threshold: '8000.0' },
      close: '4100.0',
      adjustedThreshold: '4000.0',
    })

    expect(notificationDetail(n)).toBe('決めた価格 4,000円 以上（分割に合わせた値。足したときは 8,000円）')
  })

  test('要件32・要件33: 含み損益の金額（ドル）は「取得価格 150.00ドル → 終値 180.00ドル（10株）」。株数は3桁区切り（1,000株）', () => {
    const usd = notification({
      stockCode: 'AAPL',
      currency: 'USD',
      condition: { kind: 'pnl', scope: 'stock', stockCode: 'AAPL', direction: 'gain', threshold: '300.00' },
      close: '180.00',
      basePrice: '150.00',
      unrealizedGain: '300.00',
      quantity: 10,
    })
    const many = notification({
      condition: { kind: 'pnl', scope: 'stock', stockCode: '7203', direction: 'loss', threshold: '100000' },
      close: '3480.0',
      basePrice: '4000.0',
      unrealizedGain: '-520000',
      quantity: 1000,
    })

    expect(notificationDetail(usd)).toBe('取得価格 150.00ドル → 終値 180.00ドル（10株）')
    expect(notificationDetail(many)).toBe('取得価格 4,000円 → 終値 3,480円（1,000株）')
  })

  test('要件33: 取得価格・最高値・前の終値の円は小数第1位が0でないとき小数第1位まで出す（「取得価格 2,500.5円 → 終値 1,990円」）', () => {
    const n = notification({ rate: '-20.4', basePrice: '2500.5' })

    expect(notificationDetail(n)).toBe('取得価格 2,500.5円 → 終値 1,990円')
  })
})
