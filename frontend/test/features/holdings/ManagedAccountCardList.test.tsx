// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import type { ManagedAccount } from '../../../src/entities/holding/model'
import { ManagedAccountCardList } from '../../../src/features/holdings/components/ManagedAccountCardList'

// 仕様: おまかせ運用の要件 docs/requirements/managed-account.md の要件6・要件9・要件10・要件11、基準3（口座の並び順の単体の部分）。
//   1行に並べる枚数・「⋯」の位置・掴めるように見えないこと（cursor）は CSS なので E2E（managed-account.spec.ts）。
// 境界: プラン docs/plans/managed-account/plan.md 4.8 の PR③
//   - ManagedAccountCardList: props { accounts: readonly ManagedAccount[]; onDelete: (account: ManagedAccount) => void }。
//     渡された順に並べる（並べ直さない）。空なら何も描かない（見出しも含めて）。
//   - 直前に画面に出さない見出し h2「おまかせ運用」を置き、<ul> に role="list"・aria-labelledby・aria-label="おまかせ運用" を付ける。
//     <ul> には今の holding-card-list を付け、株式の並べ替えのための holding-card-list--reorderable は付けない（4.6）。
//   - 並べ替えのフック・操作の説明・ライブリージョンをつながない。カードに tabIndex を付けない。
//   - React の key は口座の名前（取り直しで並びが変わっても、開いている「⋯」のメニューが別の口座のものにならない）。
// テスト計画: プラン 5.1 の基準3 の PR③ の行と、5.2 の PR③ の ManagedAccountCardList.test.tsx の行。

afterEach(() => {
  cleanup()
})

function account(name: string, overrides: Partial<ManagedAccount> = {}): ManagedAccount {
  return {
    name,
    valuation: '4000000',
    principal: '3000000',
    unrealizedGain: '1000000',
    unrealizedGainRate: '33.3',
    valuationDate: '2026-11-16',
    ...overrides,
  }
}

const ALPHA = account('Alpha', { valuation: '500000', principal: '500000', unrealizedGain: '0', unrealizedGainRate: '0.0' })
const WEALTHNAVI = account('WealthNavi')
const ROBO_B = account('ロボアド B', { valuation: '1000000', principal: '1250000', unrealizedGain: '-250000', unrealizedGainRate: '-20.0' })

/** 並びの中のカード（listitem）。 */
function cards(): HTMLElement[] {
  return within(screen.getByRole('list', { name: 'おまかせ運用' })).getAllByRole('listitem')
}

/** カードの中の「<名前>の操作」のボタンの名前から、カードの名前を読む（並び順の確認用）。 */
function cardNames(): string[] {
  return cards().map((card) => {
    const button = within(card).getByRole('button', { name: /の操作$/ })
    return (button.getAttribute('aria-label') ?? '').replace(/の操作$/, '')
  })
}

describe('ManagedAccountCardList の並び（要件9）', () => {
  // 基準3: Aの口座が「ロボアド B」「WealthNavi」「Alpha」（この順に登録）→ Alpha、WealthNavi、ロボアド B の順に並ぶ。
  // 並べるのは API（名前の文字コードの順）で、画面は並べ直さない。ここでは、API が返す順（Alpha・WealthNavi・ロボアド B）に渡す。
  test('基準3: Alpha・WealthNavi・ロボアド B の順に渡すと、カードがこの順に並ぶ', () => {
    render(<ManagedAccountCardList accounts={[ALPHA, WEALTHNAVI, ROBO_B]} onDelete={() => {}} />)

    expect(cardNames()).toEqual(['Alpha', 'WealthNavi', 'ロボアド B'])
    // 各カードは、その口座の値を持つ（並びと中身が対応している）。
    const [alpha, wealthnavi, robo] = cards()
    expect(within(alpha).getByText('0円（0.0%）')).toBeInTheDocument()
    expect(within(wealthnavi).getByText('+1,000,000円（+33.3%）')).toBeInTheDocument()
    expect(within(robo).getByText('-250,000円（-20.0%）')).toBeInTheDocument()
  })

  // 画面で並べ直すと、API の文字コードの順が崩れる（名前の昇順・降順・登録順に並べ直す誤りを見分ける）。
  test('補助（要件9）: WealthNavi・Alpha の順に渡すと、カードはこの順のまま（画面は並べ直さない）', () => {
    render(<ManagedAccountCardList accounts={[WEALTHNAVI, ALPHA]} onDelete={() => {}} />)

    expect(cardNames()).toEqual(['WealthNavi', 'Alpha'])
  })

  test('補助（要件9）: 口座1つなら、カードは1枚', () => {
    render(<ManagedAccountCardList accounts={[WEALTHNAVI]} onDelete={() => {}} />)

    expect(cards()).toHaveLength(1)
    expect(cardNames()).toEqual(['WealthNavi'])
  })
})

describe('ManagedAccountCardList が空のとき（要件19・要件20に沿って、口座がなければ何も描かない）', () => {
  test('補助: 空の配列なら、list「おまかせ運用」も見出し h2「おまかせ運用」も描かず、描画結果は空', () => {
    const { container } = render(<ManagedAccountCardList accounts={[]} onDelete={() => {}} />)

    expect(screen.queryByRole('list', { name: 'おまかせ運用' })).not.toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'おまかせ運用' })).not.toBeInTheDocument()
    expect(screen.queryAllByRole('heading')).toHaveLength(0)
    expect(screen.queryAllByRole('list')).toHaveLength(0)
    expect(container).toBeEmptyDOMElement()
  })
})

describe('ManagedAccountCardList の見出しと並びの名前（プラン 4.8 PR③ の「画面の要素」）', () => {
  test('補助: 画面に出さない見出し h2「おまかせ運用」が、list「おまかせ運用」の直前にある', () => {
    render(<ManagedAccountCardList accounts={[WEALTHNAVI]} onDelete={() => {}} />)

    const heading = screen.getByRole('heading', { level: 2, name: 'おまかせ運用' })
    const list = screen.getByRole('list', { name: 'おまかせ運用' })
    expect(heading.compareDocumentPosition(list) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(heading.nextElementSibling).toBe(list)
    // 画面には出さない（visually-hidden。株式の並びの見出しと同じ）。
    expect(heading).toHaveClass('visually-hidden')
  })

  test('補助: <ul> に role="list"・aria-label="おまかせ運用"・aria-labelledby（見出しの id）があり、列の数と間隔を株式と共用する holding-card-list を持つ。並べ替えの class は持たない', () => {
    render(<ManagedAccountCardList accounts={[WEALTHNAVI]} onDelete={() => {}} />)

    const list = screen.getByRole('list', { name: 'おまかせ運用' })
    const heading = screen.getByRole('heading', { level: 2, name: 'おまかせ運用' })
    expect(list.tagName).toBe('UL')
    expect(list).toHaveAttribute('role', 'list')
    expect(list).toHaveAttribute('aria-label', 'おまかせ運用')
    expect(list).toHaveAttribute('aria-labelledby', heading.id)
    expect(heading.id).not.toBe('')
    expect(list).toHaveClass('holding-card-list')
    expect(list).not.toHaveClass('holding-card-list--reorderable')
    // E2E の基準27 は ul[aria-label="おまかせ運用"] > li で数える（<ul> の子は <li> だけ）。
    expect(list.children).toHaveLength(1)
    expect(list.children[0].tagName).toBe('LI')
  })
})

describe('ManagedAccountCardList は並べ替え・移動をつながない（要件10・要件11）', () => {
  test('要件10・要件11: カードの li に tabindex・aria-roledescription・aria-label・aria-describedby がなく、並びにも並べ替えの属性がない', () => {
    render(<ManagedAccountCardList accounts={[ALPHA, WEALTHNAVI]} onDelete={() => {}} />)

    for (const card of cards()) {
      for (const attribute of ['tabindex', 'role', 'aria-roledescription', 'aria-label', 'aria-describedby']) {
        expect(card.hasAttribute(attribute), `li に ${attribute} がない`).toBe(false)
      }
    }
    const list = screen.getByRole('list', { name: 'おまかせ運用' })
    expect(list.hasAttribute('aria-roledescription')).toBe(false)
    expect(list.hasAttribute('aria-describedby')).toBe(false)
  })

  test('要件10: 並べ替えの操作の説明（hidden の段落）とライブリージョン（role="status"・aria-live）を置かない', () => {
    const { container } = render(<ManagedAccountCardList accounts={[ALPHA, WEALTHNAVI]} onDelete={() => {}} />)

    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    expect(container.querySelector('[aria-live]')).toBeNull()
    expect(container.querySelector('[hidden]')).toBeNull()
  })

  test('要件11: 画面全体で Tab で止まる要素は、口座ごとの「⋯」のボタンだけ（li や tabindex を持つ要素はない）', () => {
    const { container } = render(<ManagedAccountCardList accounts={[ALPHA, WEALTHNAVI, ROBO_B]} onDelete={() => {}} />)

    const focusable = [...container.querySelectorAll<HTMLElement>('*')].filter((element) =>
      element.matches('button, a[href], input, select, textarea, summary, [tabindex], [contenteditable]'),
    )
    expect(focusable.map((element) => element.getAttribute('aria-label'))).toEqual(['Alphaの操作', 'WealthNaviの操作', 'ロボアド Bの操作'])
    expect(container.querySelectorAll('[tabindex]')).toHaveLength(0)
  })
})

describe('ManagedAccountCardList の「削除」（要件22）', () => {
  test('要件22: 2つ目のカードの「⋯」から「削除」を選ぶと、onDelete に2つ目の口座（WealthNavi）が渡される', async () => {
    const onDelete = vi.fn()
    render(<ManagedAccountCardList accounts={[ALPHA, WEALTHNAVI]} onDelete={onDelete} />)

    fireEvent.click(screen.getByRole('button', { name: 'WealthNaviの操作' }))
    const deleteItem = await screen.findByRole('menuitem', { name: '削除' })
    await waitFor(() => expect(deleteItem).toHaveFocus())
    fireEvent.click(deleteItem)

    expect(onDelete).toHaveBeenCalledTimes(1)
    expect(onDelete).toHaveBeenCalledWith(WEALTHNAVI)
  })
})

describe('ManagedAccountCardList の key は口座の名前（プラン 4.8 PR③）', () => {
  // 取り直しで並びが変わっても、開いている「⋯」のメニューが別の口座のものにならない。位置（index）が key だと、メニューは位置に残り、別の口座のものになる。
  test('補助: WealthNavi の「⋯」のメニューを開いたまま、並びが [Alpha, WealthNavi] から [WealthNavi, Alpha] に変わっても、メニューは WealthNavi のまま', async () => {
    const { rerender } = render(<ManagedAccountCardList accounts={[ALPHA, WEALTHNAVI]} onDelete={() => {}} />)
    fireEvent.click(screen.getByRole('button', { name: 'WealthNaviの操作' }))
    await screen.findByRole('menu', { name: 'WealthNaviの操作' })

    rerender(<ManagedAccountCardList accounts={[WEALTHNAVI, ALPHA]} onDelete={() => {}} />)

    expect(cardNames()).toEqual(['WealthNavi', 'Alpha'])
    expect(screen.getByRole('menu', { name: 'WealthNaviの操作' })).toBeInTheDocument()
    expect(screen.queryByRole('menu', { name: 'Alphaの操作' })).not.toBeInTheDocument()
    expect(within(cards()[0]).getByRole('menu', { name: 'WealthNaviの操作' })).toBeInTheDocument()
  })
})
