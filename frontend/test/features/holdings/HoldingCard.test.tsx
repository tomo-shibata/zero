// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import type { Holding } from '../../../src/entities/holding/model'
import { HoldingCard } from '../../../src/features/holdings/components/HoldingCard'

// 仕様: カード表示の要件 docs/requirements/holdings-cards.md の要件2〜要件6、基準1・基準2・基準4・基準5（カード1枚の中身。単体の部分）。
//   基準1〜3・基準5・基準6・基準36 の見た目（色・文字の大きさ）は E2E（holdings-cards.spec.ts）。ここは jsdom で確かめられる中身と順番と data-tone。
// 境界: プラン docs/plans/holdings-cards/plan.md 4.4（PR①）・4.6（PR①）
//   - HoldingCard: props { holding: Holding; onDelete: (holding: Holding) => void }。<li> のカード（<ul> の中に置く）。
//   - 上から、銘柄コード（同じ行の右端に「⋯」。名前は `${name}の操作`）、銘柄名、「含み損益」の見出しと含み損益・率、
//     見出し付きの評価額・保有数量・取得価格・現在の価格・価格の基準日。見出しは「含み損益」「評価額」「保有数量」「取得価格」「現在の価格」「価格の基準日」。
//   - 含み損益と率を持つ要素（1つ）に data-tone="gain" | "loss" | "even"（価格なしは even で、文言は「取得できません」）。
//   - 価格なしの行は、含み損益・現在の価格・価格の基準日・評価額が「取得できません」（率は出さない）。
//   - 「⋯」のメニューの「削除」を選ぶと onDelete が holding を渡されて呼ばれる（振る舞いは今のまま。要件6）。

afterEach(() => {
  cleanup()
})

const TOYOTA: Holding = {
  code: '7203',
  name: 'トヨタ自動車',
  currency: 'JPY',
  quantity: 100,
  acquisitionPrice: '2500.0',
  currentPrice: '3000.0',
  priceDate: '2026-10-06',
  valuation: '300000',
  unrealizedGain: '50000',
  unrealizedGainRate: '20.0',
}

const SONY_LOSS: Holding = {
  code: '6758',
  name: 'ソニーグループ',
  currency: 'JPY',
  quantity: 10,
  acquisitionPrice: '4000.0',
  currentPrice: '3500.0',
  priceDate: '2026-10-06',
  valuation: '35000',
  unrealizedGain: '-5000',
  unrealizedGainRate: '-12.5',
}

const APPLE: Holding = {
  code: 'AAPL',
  name: 'Apple',
  currency: 'USD',
  quantity: 10,
  acquisitionPrice: '150.25',
  currentPrice: '180.00',
  priceDate: '2026-10-06',
  valuation: '1800.00',
  unrealizedGain: '297.50',
  unrealizedGainRate: '19.8',
}

const SBG_UNPRICED: Holding = {
  code: '9984',
  name: 'ソフトバンクグループ',
  currency: 'JPY',
  quantity: 100,
  acquisitionPrice: '4500.0',
  currentPrice: null,
  priceDate: null,
  valuation: null,
  unrealizedGain: null,
  unrealizedGainRate: null,
}

/** 3,000円で3,000円を買い、現在の価格も3,000円（含み損益が0）。 */
const TOYOTA_EVEN: Holding = {
  code: '7203',
  name: 'トヨタ自動車',
  currency: 'JPY',
  quantity: 10,
  acquisitionPrice: '3000.0',
  currentPrice: '3000.0',
  priceDate: '2026-10-06',
  valuation: '30000',
  unrealizedGain: '0',
  unrealizedGainRate: '0.0',
}

/** カードを <ul> の中に1枚だけ描画し、その <li> を返す。 */
function renderCard(holding: Holding, onDelete: (holding: Holding) => void = () => {}): HTMLElement {
  render(
    <ul>
      <HoldingCard holding={holding} onDelete={onDelete} />
    </ul>,
  )
  return screen.getByRole('listitem')
}

/** 要素のうち、文字がちょうど text の最初のもの（カードの中の見出しや値）。 */
function textElement(card: HTMLElement, text: string): HTMLElement {
  return within(card).getByText(text)
}

/** elements が、文書の中でこの順に並んでいること（上から下へ）。 */
function expectDocumentOrder(elements: readonly HTMLElement[], names: readonly string[]): void {
  for (let i = 1; i < elements.length; i += 1) {
    const before = elements[i - 1]
    const after = elements[i]
    const follows = (before.compareDocumentPosition(after) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0
    expect(follows, `「${names[i - 1]}」の後に「${names[i]}」が来る`).toBe(true)
  }
}

/** 含み損益と率を持つ要素（data-tone を持つ要素。カードに1つ）。 */
function gainElement(card: HTMLElement): HTMLElement {
  const elements = card.querySelectorAll<HTMLElement>('[data-tone]')
  expect(elements).toHaveLength(1)
  return elements[0]
}

describe('HoldingCard（カード1枚の中身。要件2）', () => {
  // 基準1: 7203 のカードに「7203 / トヨタ自動車 / +50,000円（+20.0%）/ 評価額 300,000円 / 保有数量 100株 / 取得価格 2,500円 / 現在の価格 3,000円 /
  // 価格の基準日 2026-10-06」が上からこの順に表示される。
  test('基準1: 7203 のカードは、銘柄コード、銘柄名、含み損益と率、評価額、保有数量、取得価格、現在の価格、価格の基準日の順に表示される', () => {
    const card = renderCard(TOYOTA)

    const parts: [string, HTMLElement][] = [
      ['7203', textElement(card, '7203')],
      ['トヨタ自動車', textElement(card, 'トヨタ自動車')],
      ['含み損益', textElement(card, '含み損益')],
      ['+50,000円（+20.0%）', gainElement(card)],
      ['評価額', textElement(card, '評価額')],
      ['300,000円', textElement(card, '300,000円')],
      ['保有数量', textElement(card, '保有数量')],
      ['100株', textElement(card, '100株')],
      ['取得価格', textElement(card, '取得価格')],
      ['2,500円', textElement(card, '2,500円')],
      ['現在の価格', textElement(card, '現在の価格')],
      ['3,000円', textElement(card, '3,000円')],
      ['価格の基準日', textElement(card, '価格の基準日')],
      ['2026-10-06', textElement(card, '2026-10-06')],
    ]
    expectDocumentOrder(
      parts.map(([, element]) => element),
      parts.map(([name]) => name),
    )
    expect(gainElement(card).textContent?.trim()).toBe('+50,000円（+20.0%）')
  })

  test('基準1: カードは <li> で、表の行（tr）やセル（td）ではない', () => {
    const card = renderCard(TOYOTA)

    expect(card.tagName).toBe('LI')
    expect(within(card).queryByRole('row')).not.toBeInTheDocument()
    expect(within(card).queryByRole('cell')).not.toBeInTheDocument()
  })

  // 基準2: AAPL のカードに「+297.50ドル（+19.8%）/ 評価額 1,800.00ドル / 保有数量 10株 / 取得価格 150.25ドル / 現在の価格 180.00ドル」が表示される。
  test('基準2: AAPL のカードは「+297.50ドル（+19.8%）」「1,800.00ドル」「10株」「150.25ドル」「180.00ドル」を表示する', () => {
    const card = renderCard(APPLE)

    expect(gainElement(card).textContent?.trim()).toBe('+297.50ドル（+19.8%）')
    expect(textElement(card, '1,800.00ドル')).toBeInTheDocument()
    expect(textElement(card, '10株')).toBeInTheDocument()
    expect(textElement(card, '150.25ドル')).toBeInTheDocument()
    expect(textElement(card, '180.00ドル')).toBeInTheDocument()
    expect(textElement(card, '2026-10-06')).toBeInTheDocument()
  })

  // 基準1: 項目の見出しと値は組になっている（定義リスト dl の dt と dd）。含み損益より下の項目に見出しが付く（要件2）。
  test('基準1: 評価額・保有数量・取得価格・現在の価格・価格の基準日の見出しは、それぞれの値と対になっている', () => {
    const card = renderCard(TOYOTA)

    const pairs: [string, string][] = [
      ['評価額', '300,000円'],
      ['保有数量', '100株'],
      ['取得価格', '2,500円'],
      ['現在の価格', '3,000円'],
      ['価格の基準日', '2026-10-06'],
    ]
    for (const [label, value] of pairs) {
      const term = textElement(card, label)
      expect(term.tagName, `${label} は dt`).toBe('DT')
      // dt の次の dd が値。
      expect(term.nextElementSibling?.tagName, `${label} の次は dd`).toBe('DD')
      expect(term.nextElementSibling?.textContent?.trim(), `${label} の値`).toBe(value)
    }
  })
})

describe('HoldingCard の含み損益の色の種類（data-tone。要件4・要件5）', () => {
  // 基準3 の単体の部分（色そのものは E2E）: プラスは gain、マイナスは loss。
  test('基準3: 含み損益がプラスなら data-tone が gain、マイナスなら loss になる', () => {
    const gain = renderCard(TOYOTA)
    expect(gainElement(gain)).toHaveAttribute('data-tone', 'gain')
    cleanup()

    const loss = renderCard(SONY_LOSS)
    expect(gainElement(loss)).toHaveAttribute('data-tone', 'loss')
    expect(gainElement(loss).textContent?.trim()).toBe('-5,000円（-12.5%）')
  })

  // 基準4: 7203 の売買履歴が「買 3,000円 10株」で現在の価格が 3,000円 → 「0円（0.0%）」で、赤・青は付かない（data-tone が even）。
  test('基準4: 含み損益が0のカードは「0円（0.0%）」で、data-tone が even になる', () => {
    const card = renderCard(TOYOTA_EVEN)

    expect(gainElement(card).textContent?.trim()).toBe('0円（0.0%）')
    expect(gainElement(card)).toHaveAttribute('data-tone', 'even')
  })

  // 基準4: API の率が 0.0 でなくても（-0.1）、含み損益が0なら率は 0.0 を出す（二重に守る。プラン 4.2）。
  test('基準4: 含み損益が0なら、API の率が "-0.1" でも「0円（0.0%）」と表示する', () => {
    const card = renderCard({
      code: '7203',
      name: 'トヨタ自動車',
      currency: 'JPY',
      quantity: 10,
      acquisitionPrice: '3000.0',
      currentPrice: '3000.0',
      priceDate: '2026-10-06',
      valuation: '30000',
      unrealizedGain: '0',
      unrealizedGainRate: '-0.1',
    })

    expect(gainElement(card).textContent?.trim()).toBe('0円（0.0%）')
    expect(gainElement(card)).toHaveAttribute('data-tone', 'even')
  })

  // 基準5: 9984 のカードの含み損益は「取得できません」だけで、率は表示されず、赤・青は付かない（data-tone が even）。
  // 現在の価格・価格の基準日・評価額も「取得できません」と表示される。
  test('基準5: 価格のないカードは、含み損益・評価額・現在の価格・価格の基準日が「取得できません」で、率は出ず、data-tone が even', () => {
    const card = renderCard(SBG_UNPRICED)

    const gain = gainElement(card)
    expect(gain.textContent?.trim()).toBe('取得できません')
    expect(gain).toHaveAttribute('data-tone', 'even')
    expect(card).not.toHaveTextContent('%')
    expect(card).not.toHaveTextContent('+')
    expect(within(card).getAllByText('取得できません')).toHaveLength(4)
    // 評価額・現在の価格・価格の基準日の値が「取得できません」（見出しと対になっている）。
    for (const label of ['評価額', '現在の価格', '価格の基準日']) {
      expect(textElement(card, label).nextElementSibling?.textContent?.trim(), `${label} の値`).toBe(
        '取得できません',
      )
    }
    // 保有数量と取得価格は、価格がなくても表示する。
    expect(textElement(card, '100株').previousElementSibling?.textContent).toBe('保有数量')
    expect(textElement(card, '4,500円').previousElementSibling?.textContent).toBe('取得価格')
  })
})

describe('HoldingCard の「⋯」のメニュー（要件6。振る舞いは今のまま）', () => {
  test('補助（要件6）: 「⋯」のボタンの名前は「<銘柄名>の操作」で、カードの中にある', () => {
    const card = renderCard(TOYOTA)

    expect(within(card).getByRole('button', { name: 'トヨタ自動車の操作' })).toBeInTheDocument()
  })

  test('補助（要件6）: 銘柄コードと「⋯」のボタンは、含み損益より前（カードの上）にある', () => {
    const card = renderCard(TOYOTA)

    expectDocumentOrder(
      [
        textElement(card, '7203'),
        within(card).getByRole('button', { name: 'トヨタ自動車の操作' }),
        textElement(card, 'トヨタ自動車'),
        gainElement(card),
      ],
      ['銘柄コード', '「⋯」', '銘柄名', '含み損益'],
    )
  })

  test('補助（要件6）: 「⋯」を押して「削除」を選ぶと、onDelete がその銘柄の holding で呼ばれる', async () => {
    const onDelete = vi.fn()
    const card = renderCard(TOYOTA, onDelete)

    fireEvent.click(within(card).getByRole('button', { name: 'トヨタ自動車の操作' }))
    const deleteItem = await screen.findByRole('menuitem', { name: '削除' })
    await waitFor(() => expect(deleteItem).toHaveFocus())
    fireEvent.click(deleteItem)

    expect(onDelete).toHaveBeenCalledTimes(1)
    expect(onDelete).toHaveBeenCalledWith(TOYOTA)
  })
})
