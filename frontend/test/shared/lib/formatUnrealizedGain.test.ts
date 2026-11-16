import { describe, expect, test } from 'vitest'
import { formatUnrealizedGain, unrealizedGainTone } from '../../../src/shared/lib/format'

// 仕様: カード表示の要件 docs/requirements/holdings-cards.md の要件4・要件10、基準4・基準13。
// 境界: プラン docs/plans/holdings-cards/plan.md 4.6（PR①）の shared/lib/format.ts。
//   formatUnrealizedGain(currency: Currency, gain: string, rate: string): string
//     含み損益と率を表示する。例 ('JPY', '75000', '20.0') → '+75,000円（+20.0%）'、('USD', '-0.01', '0.0') → '-0.01ドル（-0.0%）'、
//     ('JPY', '0', '0.0') → '0円（0.0%）'。含み損益が0なら、率は rate によらず 0.0（('JPY', '0', '-0.1') → '0円（0.0%）'）。
//     符号は gain から決める（'-' で始まれば '-'、0 なら符号なし、それ以外は '+'）。率は '-' を外した数字に、その符号を付ける。
//     gain の形は円 /^-?[0-9]+$/・ドル /^-?[0-9]+\.[0-9]{2}$/、rate は /^-?[0-9]+\.[0-9]$/。通貨に合わない形は throw する。
//   unrealizedGainTone(gain: string): 'gain' | 'loss' | 'even'
//     プラスは 'gain'、マイナスは 'loss'、0 は 'even'（要件4）。
// テスト計画: プラン 5.1 の基準4（単体の部分）・基準13（TS）と、5.3 の「単体（TS）」の行。

describe('formatUnrealizedGain（基準13。要件10の表示形式）', () => {
  // 基準13: 含み損益と取得額の組 (a)〜(f) を、Go が返す文字列（例 '297.50'・'19.8'）にして整形する。
  test.each([
    { name: '(a) 円 75,000・取得額 375,000', currency: 'JPY', gain: '75000', rate: '20.0', expected: '+75,000円（+20.0%）' },
    { name: '(b) 円 -30,000・取得額 420,000', currency: 'JPY', gain: '-30000', rate: '-7.1', expected: '-30,000円（-7.1%）' },
    { name: '(c) 円 1,234,567・取得額 1,000,000', currency: 'JPY', gain: '1234567', rate: '123.5', expected: '+1,234,567円（+123.5%）' },
    { name: '(d) ドル 297.5・取得額 1,502.5', currency: 'USD', gain: '297.50', rate: '19.8', expected: '+297.50ドル（+19.8%）' },
    { name: '(e) ドル -0.01・取得額 100', currency: 'USD', gain: '-0.01', rate: '0.0', expected: '-0.01ドル（-0.0%）' },
    { name: '(f) 円 1・取得額 1,000,000', currency: 'JPY', gain: '1', rate: '0.0', expected: '+1円（+0.0%）' },
  ] as const)('基準13: $name は「$expected」になる', ({ currency, gain, rate, expected }) => {
    expect(formatUnrealizedGain(currency, gain, rate)).toBe(expected)
  })

  // 要件10の例: -5,000円（-12.5%）、+1円（+0.0%）。
  test('基準13（要件10の例）: ("JPY", "-5000", "-12.5") は「-5,000円（-12.5%）」になる', () => {
    expect(formatUnrealizedGain('JPY', '-5000', '-12.5')).toBe('-5,000円（-12.5%）')
  })

  // 基準4（単体の部分）: 含み損益が0のときは、符号を付けず「0円（0.0%）」。ドルは「0.00ドル（0.0%）」。
  test('基準4: ("JPY", "0", "0.0") は符号を付けず「0円（0.0%）」になる', () => {
    expect(formatUnrealizedGain('JPY', '0', '0.0')).toBe('0円（0.0%）')
  })

  test('基準4: ("USD", "0.00", "0.0") は符号を付けず「0.00ドル（0.0%）」になる', () => {
    expect(formatUnrealizedGain('USD', '0.00', '0.0')).toBe('0.00ドル（0.0%）')
  })

  // 補助（5.3）: 含み損益が0で率が0でないとき（API の率によらず、画面も 0.0 を出す。二重に守る）。
  test.each([
    { currency: 'JPY', gain: '0', rate: '-0.1', expected: '0円（0.0%）' },
    { currency: 'JPY', gain: '0', rate: '0.1', expected: '0円（0.0%）' },
    { currency: 'USD', gain: '0.00', rate: '0.1', expected: '0.00ドル（0.0%）' },
    { currency: 'USD', gain: '0.00', rate: '-0.1', expected: '0.00ドル（0.0%）' },
  ] as const)(
    '補助: 含み損益が0なら、率が "$rate" でも ($currency, "$gain") は「$expected」になる',
    ({ currency, gain, rate, expected }) => {
      expect(formatUnrealizedGain(currency, gain, rate)).toBe(expected)
    },
  )

  // 補助（判断15）: 画面の符号は含み損益から決める（率の符号によらない）。含み損益 +1円で率が -0.1 でも、率は符号を付け替えて「+0.1%」にする。
  test('補助（判断15）: 含み損益が +1円なら、率が "-0.1" でも ("JPY", "1", "-0.1") は「+1円（+0.1%）」になる', () => {
    expect(formatUnrealizedGain('JPY', '1', '-0.1')).toBe('+1円（+0.1%）')
  })

  // 補助（5.3）: 大きな桁を丸めずに3桁区切りにする（Number の精度 2^53 を超える18桁）。
  test('補助: ("JPY", "123456789012345678", "1.0") は丸めずに「+123,456,789,012,345,678円（+1.0%）」になる', () => {
    expect(formatUnrealizedGain('JPY', '123456789012345678', '1.0')).toBe(
      '+123,456,789,012,345,678円（+1.0%）',
    )
  })

  test('補助: ("JPY", "-123456789012345678", "-1.0") は丸めずに「-123,456,789,012,345,678円（-1.0%）」になる', () => {
    expect(formatUnrealizedGain('JPY', '-123456789012345678', '-1.0')).toBe(
      '-123,456,789,012,345,678円（-1.0%）',
    )
  })

  test('補助: ドルの大きな桁 ("USD", "123456789012345678.99", "5.5") は丸めずに「+123,456,789,012,345,678.99ドル（+5.5%）」になる', () => {
    expect(formatUnrealizedGain('USD', '123456789012345678.99', '5.5')).toBe(
      '+123,456,789,012,345,678.99ドル（+5.5%）',
    )
  })

  // 補助（5.3）: 通貨に合わない形・ありえない値は、誤った表示を出さずに throw する（既存の formatValuation と同じ決まり）。
  test.each([
    { currency: 'JPY', gain: '75000.00', rate: '20.0', reason: '円の含み損益に小数（ドルの形）' },
    { currency: 'JPY', gain: '+75000', rate: '20.0', reason: '含み損益に + が付いている（API は + を付けない）' },
    { currency: 'JPY', gain: '', rate: '20.0', reason: '含み損益が空' },
    { currency: 'JPY', gain: 'abc', rate: '20.0', reason: '含み損益が数でない' },
    { currency: 'USD', gain: '297', rate: '19.8', reason: 'ドルの含み損益に小数がない（円の形）' },
    { currency: 'USD', gain: '297.5', rate: '19.8', reason: 'ドルの含み損益の小数が1桁' },
    { currency: 'USD', gain: '297.505', rate: '19.8', reason: 'ドルの含み損益の小数が3桁' },
    { currency: 'JPY', gain: '75000', rate: '20', reason: '率に小数がない' },
    { currency: 'JPY', gain: '75000', rate: '20.00', reason: '率の小数が2桁' },
    { currency: 'JPY', gain: '75000', rate: '+20.0', reason: '率に + が付いている' },
    { currency: 'JPY', gain: '75000', rate: 'abc', reason: '率が数でない' },
    { currency: 'JPY', gain: '75000', rate: '', reason: '率が空' },
  ] as const)('補助: ($currency, "$gain", "$rate") は $reason ので throw する', ({ currency, gain, rate }) => {
    expect(() => formatUnrealizedGain(currency, gain, rate)).toThrow()
  })
})

describe('unrealizedGainTone（要件4。プラスは gain・マイナスは loss・0 は even）', () => {
  test.each([
    { gain: '75000', expected: 'gain' },
    { gain: '1', expected: 'gain' },
    { gain: '297.50', expected: 'gain' },
    { gain: '0.01', expected: 'gain' },
    { gain: '-501', expected: 'loss' },
    { gain: '-0.01', expected: 'loss' },
    { gain: '-30000', expected: 'loss' },
    { gain: '0', expected: 'even' },
    { gain: '0.00', expected: 'even' },
  ] as const)('基準4: unrealizedGainTone("$gain") は "$expected" になる', ({ gain, expected }) => {
    expect(unrealizedGainTone(gain)).toBe(expected)
  })
})
