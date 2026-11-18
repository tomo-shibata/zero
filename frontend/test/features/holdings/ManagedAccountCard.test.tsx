// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import type { ManagedAccount } from '../../../src/entities/holding/model'
import { ManagedAccountCard } from '../../../src/features/holdings/components/ManagedAccountCard'

// 仕様: おまかせ運用の要件 docs/requirements/managed-account.md の要件7・要件8・要件11・要件12・要件13・要件14・要件22、基準1・基準7・基準8（カード1枚の中身。単体の部分）。
//   基準1 の色と文字の大きさ・基準4・基準5 の操作は E2E（managed-account.spec.ts）。ここは jsdom で確かめられる中身と順番と data-tone。
// 境界: プラン docs/plans/managed-account/plan.md 4.8 の PR③
//   - ManagedAccountCard: props { account: ManagedAccount; onDelete: (account: ManagedAccount) => void }。<li> のカード（単体のテストでは <ul> で包む）。
//   - 上から、名前（同じ行の右端に「⋯」。名前は `${name}の操作`）、文字「おまかせ運用」、見出し「含み損益」と含み損益・率（data-tone の要素が1つ）、
//     見出し付き（dl の dt と次の dd）の評価額・元本・評価額の基準日。
//   - 「⋯」のメニューの項目は「削除」だけ。選ぶと onDelete が account を渡されて呼ばれる。
//   - カード（li）には tabIndex・role・aria-roledescription・aria-label・aria-describedby を付けない。
//   - 文字「おまかせ運用」には aria-hidden を付けない（4.6）。
// 金額と率は、API から受け取った文字列を整えて出すだけ（計算はしない。プラン 4.1）。

afterEach(() => {
  cleanup()
  vi.useRealTimers()
})

/** 基準1 の WealthNavi（評価額 4,000,000円・損益 +1,000,000円。元本 3,000,000円・率 33.3%）。 */
const WEALTHNAVI: ManagedAccount = {
  name: 'WealthNavi',
  valuation: '4000000',
  principal: '3000000',
  unrealizedGain: '1000000',
  unrealizedGainRate: '33.3',
  valuationDate: '2026-11-16',
}

/** 口座の値を、評価額と損益の組から作る（元本と率は、要件12 のとおり、テスト担当が計算し直した値を渡す）。 */
function account(valuation: string, principal: string, gain: string, rate: string): ManagedAccount {
  return { ...WEALTHNAVI, valuation, principal, unrealizedGain: gain, unrealizedGainRate: rate }
}

/** カードを <ul> の中に1枚だけ描画し、その <li> を返す。 */
function renderCard(target: ManagedAccount, onDelete: (account: ManagedAccount) => void = () => {}): HTMLElement {
  render(
    <ul>
      <ManagedAccountCard account={target} onDelete={onDelete} />
    </ul>,
  )
  return screen.getByRole('listitem')
}

/** カードの中の、文字がちょうど text の要素。 */
function textElement(card: HTMLElement, text: string): HTMLElement {
  return within(card).getByText(text, { exact: true })
}

/** elements が、文書の中でこの順に並んでいること（上から下へ）。 */
function expectDocumentOrder(elements: readonly HTMLElement[], names: readonly string[]): void {
  for (let i = 1; i < elements.length; i += 1) {
    const follows = (elements[i - 1].compareDocumentPosition(elements[i]) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0
    expect(follows, `「${names[i - 1]}」の後に「${names[i]}」が来る`).toBe(true)
  }
}

/** 含み損益と率を持つ要素（data-tone を持つ要素。カードに1つ）。 */
function gainElement(card: HTMLElement): HTMLElement {
  const elements = card.querySelectorAll<HTMLElement>('[data-tone]')
  expect(elements).toHaveLength(1)
  return elements[0]
}

/** 見出し label の dt の、次の dd の文字。 */
function valueOf(card: HTMLElement, label: string): string | undefined {
  const term = textElement(card, label)
  expect(term.tagName, `${label} は dt`).toBe('DT')
  expect(term.nextElementSibling?.tagName, `${label} の次は dd`).toBe('DD')
  return term.nextElementSibling?.textContent?.trim()
}

describe('ManagedAccountCard（カード1枚の中身。要件7・要件13）', () => {
  // 基準1 の単体の部分: WealthNavi のカードに「WealthNavi / おまかせ運用 / 含み損益 +1,000,000円（+33.3%）/ 評価額 4,000,000円 / 元本 3,000,000円 /
  // 評価額の基準日 2026-11-16」が上からこの順に表示される。
  test('基準1: WealthNavi のカードは、名前、「おまかせ運用」、含み損益と率、評価額、元本、評価額の基準日の順に表示される', () => {
    const card = renderCard(WEALTHNAVI)

    const parts: [string, HTMLElement][] = [
      ['WealthNavi', textElement(card, 'WealthNavi')],
      ['おまかせ運用', textElement(card, 'おまかせ運用')],
      ['含み損益', textElement(card, '含み損益')],
      ['+1,000,000円（+33.3%）', gainElement(card)],
      ['評価額', textElement(card, '評価額')],
      ['4,000,000円', textElement(card, '4,000,000円')],
      ['元本', textElement(card, '元本')],
      ['3,000,000円', textElement(card, '3,000,000円')],
      ['評価額の基準日', textElement(card, '評価額の基準日')],
      ['2026-11-16', textElement(card, '2026-11-16')],
    ]
    expectDocumentOrder(
      parts.map(([, element]) => element),
      parts.map(([name]) => name),
    )
    expect(gainElement(card).textContent?.trim()).toBe('+1,000,000円（+33.3%）')
    expect(gainElement(card)).toHaveAttribute('data-tone', 'gain')
  })

  test('基準1: カードは <li> で、表の行（tr）やセル（td）ではない', () => {
    const card = renderCard(WEALTHNAVI)

    expect(card.tagName).toBe('LI')
    expect(within(card).queryByRole('row')).not.toBeInTheDocument()
    expect(within(card).queryByRole('cell')).not.toBeInTheDocument()
  })

  // 要件7: 評価額・元本・評価額の基準日には、その名前の見出しを付ける（株式のカードと同じ。dl の dt と dd）。
  test('要件7: 評価額・元本・評価額の基準日の見出しは、それぞれの値と対になっている（dt の次の dd）', () => {
    const card = renderCard(WEALTHNAVI)

    expect(valueOf(card, '評価額')).toBe('4,000,000円')
    expect(valueOf(card, '元本')).toBe('3,000,000円')
    expect(valueOf(card, '評価額の基準日')).toBe('2026-11-16')
  })

  test('要件7: 名前と「⋯」のボタンは、含み損益より前（カードの上）にある。「⋯」の名前は「WealthNaviの操作」', () => {
    const card = renderCard(WEALTHNAVI)

    const menuButton = within(card).getByRole('button', { name: 'WealthNaviの操作' })
    expectDocumentOrder(
      [textElement(card, 'WealthNavi'), menuButton, textElement(card, 'おまかせ運用'), gainElement(card)],
      ['名前', '「⋯」', 'おまかせ運用', '含み損益'],
    )
  })

  // 要件7: 文字「おまかせ運用」は、カードの中にちょうど1つ。aria-hidden を付けない（プラン 4.6。見える文字と読み上げをそろえる）。
  test('要件7（プラン 4.6）: 文字「おまかせ運用」はカードにちょうど1つあり、aria-hidden の中にない', () => {
    const card = renderCard(WEALTHNAVI)

    const labels = within(card).getAllByText('おまかせ運用', { exact: true })
    expect(labels).toHaveLength(1)
    expect(labels[0].closest('[aria-hidden]')).toBeNull()
  })

  // 基準8: 評価額の基準日が今日（2026-11-16）の1年前でも、そのまま出し、要件7 にない文言（注意）は足さない。
  test('基準8（要件14）: 今日が 2026-11-16 で valuationDate が "2025-11-16" のとき、「2025-11-16」がそのまま出て、カードの文字は要件7 の文字だけをつないだものとちょうど同じ', () => {
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(new Date('2026-11-16T03:00:00+09:00'))
    const card = renderCard({ ...WEALTHNAVI, valuationDate: '2025-11-16' })

    expect(valueOf(card, '評価額の基準日')).toBe('2025-11-16')
    expect(card.textContent).toBe(
      'WealthNavi' +
        '⋯' +
        'おまかせ運用' +
        '含み損益' +
        '+1,000,000円（+33.3%）' +
        '評価額' +
        '4,000,000円' +
        '元本' +
        '3,000,000円' +
        '評価額の基準日' +
        '2025-11-16',
    )
  })
})

describe('ManagedAccountCard の含み損益と元本の表示（基準7。要件8・要件12・要件13）', () => {
  // API が返す文字列（元本・損益・率は計算済み）を整えて出すだけ。期待値は要件定義の基準7 の括弧の式から、テスト担当が計算し直した値。
  //   (a) 2,700,000円・-300,000円 → 元本 3,000,000円、率 -10.0%
  //   (b) 3,000,000円・0円 → 元本 3,000,000円、率 0.0%
  //   (c) 1,234,567円・+234,567円 → 元本 1,000,000円、率 23.4567% → 23.5%
  //   (d) 876,543円・-123,457円 → 元本 1,000,000円、率 -12.3457% → -12.3%
  //   (e) 1,000円・+1円 → 元本 999円、率 0.1001…% → 0.1%
  const cases: {
    name: string
    target: ManagedAccount
    valuation: string
    gain: string
    tone: 'gain' | 'loss' | 'even'
    principal: string
  }[] = [
    { name: '(a)', target: account('2700000', '3000000', '-300000', '-10.0'), valuation: '2,700,000円', gain: '-300,000円（-10.0%）', tone: 'loss', principal: '3,000,000円' },
    { name: '(b)', target: account('3000000', '3000000', '0', '0.0'), valuation: '3,000,000円', gain: '0円（0.0%）', tone: 'even', principal: '3,000,000円' },
    { name: '(c)', target: account('1234567', '1000000', '234567', '23.5'), valuation: '1,234,567円', gain: '+234,567円（+23.5%）', tone: 'gain', principal: '1,000,000円' },
    { name: '(d)', target: account('876543', '1000000', '-123457', '-12.3'), valuation: '876,543円', gain: '-123,457円（-12.3%）', tone: 'loss', principal: '1,000,000円' },
    { name: '(e)', target: account('1000', '999', '1', '0.1'), valuation: '1,000円', gain: '+1円（+0.1%）', tone: 'gain', principal: '999円' },
  ]

  test.each(cases)('基準7 $name: 含み損益と率が「$gain」（data-tone が $tone）、評価額が「$valuation」、元本が「$principal」', ({ target, valuation, gain, tone, principal }) => {
    const card = renderCard(target)

    expect(gainElement(card).textContent?.trim()).toBe(gain)
    expect(gainElement(card)).toHaveAttribute('data-tone', tone)
    expect(valueOf(card, '評価額')).toBe(valuation)
    expect(valueOf(card, '元本')).toBe(principal)
  })

  // 要件8: 含み損益が0なら、API の率が 0.0 でなくても「0.0%」と出し、赤・青は付けない（株式のカードと同じ。カード表示の要件4）。
  test('要件8（カード表示の要件4）: 含み損益が "0" なら、率が "-0.1" で来ても「0円（0.0%）」で data-tone が even', () => {
    const card = renderCard(account('3000000', '3000000', '0', '-0.1'))

    expect(gainElement(card).textContent?.trim()).toBe('0円（0.0%）')
    expect(gainElement(card)).toHaveAttribute('data-tone', 'even')
  })
})

describe('ManagedAccountCard の「⋯」のメニュー（要件22）', () => {
  test('要件22: 「WealthNaviの操作」を押すと、メニューの項目は「削除」の1つだけ', async () => {
    const card = renderCard(WEALTHNAVI)

    fireEvent.click(within(card).getByRole('button', { name: 'WealthNaviの操作' }))

    const items = await screen.findAllByRole('menuitem')
    expect(items).toHaveLength(1)
    expect(items[0]).toHaveAccessibleName('削除')
  })

  test('要件22: 「削除」を選ぶと、onDelete がその口座（account）を渡されて1回呼ばれる', async () => {
    const onDelete = vi.fn()
    const card = renderCard(WEALTHNAVI, onDelete)

    fireEvent.click(within(card).getByRole('button', { name: 'WealthNaviの操作' }))
    const deleteItem = await screen.findByRole('menuitem', { name: '削除' })
    await waitFor(() => expect(deleteItem).toHaveFocus())
    fireEvent.click(deleteItem)

    expect(onDelete).toHaveBeenCalledTimes(1)
    expect(onDelete).toHaveBeenCalledWith(WEALTHNAVI)
  })

  test('要件22: 「⋯」を押しただけでは onDelete は呼ばれない', async () => {
    const onDelete = vi.fn()
    const card = renderCard(WEALTHNAVI, onDelete)

    fireEvent.click(within(card).getByRole('button', { name: 'WealthNaviの操作' }))
    await screen.findByRole('menuitem', { name: '削除' })

    expect(onDelete).not.toHaveBeenCalled()
  })
})

describe('ManagedAccountCard はカードそのものにフォーカスを当てない（要件11。並べ替えと移動の属性を付けない）', () => {
  test('要件11: カードの li に tabindex・role・aria-roledescription・aria-label・aria-describedby がない', () => {
    const card = renderCard(WEALTHNAVI)

    for (const attribute of ['tabindex', 'role', 'aria-roledescription', 'aria-label', 'aria-describedby', 'aria-labelledby']) {
      expect(card.hasAttribute(attribute), `li に ${attribute} がない`).toBe(false)
    }
  })

  // jsdom は Tab の移動を持たないので、Tab で止まりうる要素（ボタン・リンク・入力・tabindex を持つ要素）を数える。
  test('要件11: カードの中で Tab で止まる要素は「WealthNaviの操作」のボタンだけで、li 自身も tabindex を持つ子も含まない', () => {
    const card = renderCard(WEALTHNAVI)

    const focusable = [card, ...card.querySelectorAll<HTMLElement>('*')].filter((element) =>
      element.matches('button, a[href], input, select, textarea, summary, [tabindex], [contenteditable]'),
    )
    expect(focusable).toHaveLength(1)
    expect(focusable[0]).toBe(within(card).getByRole('button', { name: 'WealthNaviの操作' }))
    expect(card.querySelectorAll('[tabindex]')).toHaveLength(0)
  })

  test('要件10・要件11: カードの中に並べ替えの説明（hidden の段落）とライブリージョン（role="status"）がない', () => {
    const card = renderCard(WEALTHNAVI)

    expect(within(card).queryByRole('status')).not.toBeInTheDocument()
    expect(card.querySelector('[aria-live]')).toBeNull()
    expect(card.querySelector('[hidden]')).toBeNull()
  })
})
