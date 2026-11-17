// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import type { Holding } from '../../../src/entities/holding/model'
import { HoldingCardList } from '../../../src/features/holdings/components/HoldingCardList'

// 仕様: カード表示の要件 docs/requirements/holdings-cards.md の要件1・要件2・要件6、基準1（表は表示されない）・基準8（PR① の部分: 「⋯」でメニューが開く）。
//   1行に並べる枚数（基準7）は CSS なので E2E（holdings-cards.spec.ts）。
// 境界: プラン docs/plans/holdings-cards/plan.md 4.4（PR①）・4.6（PR①）
//   - HoldingCardList: props { holdings: readonly Holding[]; onDelete: (holding: Holding) => void }。
//     <ul aria-label="保有株式"> の中に <li> のカード（渡された順）。カードの中の「⋯」の名前は `${name}の操作`。
// PR② で props に onMove: (activeCode: string, overCode: string | null) => void が増える（4.6 PR②）。このファイルでは使わないので、何もしない関数を渡す。
//   並べ替えのテストは HoldingCardList.reorder.test.tsx。
// 要件 FR-3 の表（table）は、カードに置き換わって表示されない。

afterEach(() => {
  cleanup()
})

const SONY: Holding = {
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

const SBG: Holding = {
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

const FOUR: readonly Holding[] = [SONY, TOYOTA, SBG, APPLE]

/** カードの並び（<ul aria-label="保有株式">）。 */
function cardList(): HTMLElement {
  return screen.getByRole('list', { name: '保有株式' })
}

describe('HoldingCardList（カードの並び。要件1）', () => {
  test('基準1: 名前「保有株式」のリストの中に、渡した順にカードが1銘柄1枚ずつ表示され、表（table）は表示されない', () => {
    render(<HoldingCardList holdings={FOUR} onDelete={() => {}} onMove={() => {}} />)

    const cards = within(cardList()).getAllByRole('listitem')
    expect(cards).toHaveLength(4)
    expect(cards.map((card) => within(card).getByText(/^(6758|7203|9984|AAPL)$/).textContent)).toEqual([
      '6758',
      '7203',
      '9984',
      'AAPL',
    ])
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
    expect(screen.queryByRole('row')).not.toBeInTheDocument()
    expect(screen.queryByRole('columnheader')).not.toBeInTheDocument()
  })

  test('基準1: リストは <ul> で、カードは直下の <li>', () => {
    render(<HoldingCardList holdings={FOUR} onDelete={() => {}} onMove={() => {}} />)

    const list = cardList()
    expect(list.tagName).toBe('UL')
    expect(Array.from(list.children).map((child) => child.tagName)).toEqual(['LI', 'LI', 'LI', 'LI'])
  })

  // 基準1・基準2・基準5: 渡した holding をそのまま、カードごとの中身にする（円・ドル・価格なしが混ざっても、銘柄ごとに正しい行）。
  test('基準1・基準2・基準5: 円・ドル・価格なしのカードは、それぞれ自分の含み損益を表示する', () => {
    render(<HoldingCardList holdings={FOUR} onDelete={() => {}} onMove={() => {}} />)

    const [sony, toyota, sbg, apple] = within(cardList()).getAllByRole('listitem')
    expect(sony).toHaveTextContent('-5,000円（-12.5%）')
    expect(toyota).toHaveTextContent('+50,000円（+20.0%）')
    expect(sbg).toHaveTextContent('取得できません')
    expect(sbg).not.toHaveTextContent('%')
    expect(apple).toHaveTextContent('+297.50ドル（+19.8%）')
  })

  test('補助（要件6）: カードごとに「<銘柄名>の操作」の「⋯」のボタンが1つずつある', () => {
    render(<HoldingCardList holdings={FOUR} onDelete={() => {}} onMove={() => {}} />)

    for (const holding of FOUR) {
      expect(screen.getByRole('button', { name: `${holding.name}の操作` })).toBeInTheDocument()
    }
    expect(screen.getAllByRole('button')).toHaveLength(4)
  })

  // 基準8（PR① の部分）: 7203 のカードの「⋯」（「トヨタ自動車の操作」）を押すと「削除」のメニューが開く。
  test('基準8: 「トヨタ自動車の操作」を押すと「削除」のメニューが開き、並びは変わらない', async () => {
    render(<HoldingCardList holdings={FOUR} onDelete={() => {}} onMove={() => {}} />)

    fireEvent.click(screen.getByRole('button', { name: 'トヨタ自動車の操作' }))

    const deleteItem = await screen.findByRole('menuitem', { name: '削除' })
    expect(deleteItem).toBeInTheDocument()
    expect(screen.getByRole('menu')).toBeInTheDocument()
    const codes = within(cardList())
      .getAllByRole('listitem')
      .map((card) => within(card).getByText(/^(6758|7203|9984|AAPL)$/).textContent)
    expect(codes).toEqual(['6758', '7203', '9984', 'AAPL'])
  })

  test('補助（要件6）: 2枚目のカードの「⋯」から「削除」を選ぶと、onDelete が2枚目の holding で呼ばれる', async () => {
    const onDelete = vi.fn()
    render(<HoldingCardList holdings={FOUR} onDelete={onDelete} onMove={() => {}} />)

    fireEvent.click(screen.getByRole('button', { name: 'トヨタ自動車の操作' }))
    const deleteItem = await screen.findByRole('menuitem', { name: '削除' })
    await waitFor(() => expect(deleteItem).toHaveFocus())
    fireEvent.click(deleteItem)

    expect(onDelete).toHaveBeenCalledTimes(1)
    expect(onDelete).toHaveBeenCalledWith(TOYOTA)
  })

  test('補助: holdings が空ならカードは1枚もない', () => {
    render(<HoldingCardList holdings={[]} onDelete={() => {}} onMove={() => {}} />)

    expect(screen.queryAllByRole('listitem')).toHaveLength(0)
  })
})
