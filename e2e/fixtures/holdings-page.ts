import { expect, type Locator, type Page } from '@playwright/test'

// /holdings のカードと合計の欄を読む補助（E2E。holdings.spec.ts・price-update.spec.ts・holdings-cards.spec.ts が使う）。
// 仕様: カード表示の要件 docs/requirements/holdings-cards.md、プラン docs/plans/holdings-cards/plan.md 4.4・4.6（PR①）・5.1 の E2E の共通の決まり。
//   - カードの並びは <ul aria-label="保有株式"> の <li>。カードは、銘柄コード、銘柄名、含み損益と率（data-tone を持つ要素が1つ）、
//     見出し付きの <dl>（評価額・保有数量・取得価格・現在の価格・価格の基準日）の順。
//   - 合計の欄は region「合計」（h2）。通貨ごとに region「円」「ドル」（h3）。各通貨の中は「含み損益」、含み損益と率（data-tone）、「評価額」、評価額、注記の順。
//   - 色は、計算後の color を、テストの中に書いた定数（赤・青）と比べる。ページの CSS 変数から読まない（プラン 5.1）。

/** 益（プラス）の文字の色（プラン 4.4 の判断8: #c62828）。 */
export const GAIN_COLOR = 'rgb(198, 40, 40)'
/** 損（マイナス）の文字の色（#1565c0）。 */
export const LOSS_COLOR = 'rgb(21, 101, 192)'

export const CARD_LIST_LABEL = '保有株式'
export const SUMMARY_LABEL = '合計'
export const UNAVAILABLE = '取得できません'
export const UNPRICED_NOTE = '価格を取得できない銘柄を含みません'
export const REFRESH_BUTTON = '価格を今すぐ更新'

/** カードの並び（<ul aria-label="保有株式">）。 */
export function cardList(page: Page): Locator {
  return page.getByRole('list', { name: CARD_LIST_LABEL, exact: true })
}

/** カードの並びの中のカード（<li>）すべて。DOM の順。 */
export function holdingCards(page: Page): Locator {
  return cardList(page).getByRole('listitem')
}

/** 銘柄コードが code のカード。 */
export function holdingCard(page: Page, code: string): Locator {
  return holdingCards(page).filter({ has: page.getByText(code, { exact: true }) })
}

/** カードの項目（<dl> の見出し label に対応する値）。label は「評価額」「保有数量」「取得価格」「現在の価格」「価格の基準日」。 */
export function cardValue(card: Locator, label: string): Locator {
  return card.locator(`dt:text-is("${label}") + dd`)
}

/** カードの含み損益と率を持つ要素（data-tone を持つ要素。カードに1つ）。 */
export function cardGain(card: Locator): Locator {
  return card.locator('[data-tone]')
}

/** カードの中の、銘柄コードの文字を持つ要素。 */
export function cardCode(card: Locator, code: string): Locator {
  return card.getByText(code, { exact: true })
}

/** 1枚のカードの表示値。 */
export type CardExpectation = {
  code: string
  name: string
  /** 含み損益と率（例 "+50,000円（+20.0%）"）または「取得できません」。 */
  gain: string
  valuation: string
  quantity: string
  acquisitionPrice: string
  currentPrice: string
  priceDate: string
}

/**
 * カードの中身が expected のとおりであることを確かめる（銘柄コード・銘柄名・含み損益と率・評価額・保有数量・取得価格・現在の価格・価格の基準日）。
 * 7つのセルを持つ表の行を、カードに読み替えた確かめ方（holdings-cards.md の「既存の要件の置き換え」。確かめる値は変えない）。
 */
export async function expectCard(card: Locator, expected: CardExpectation): Promise<void> {
  await expect(card).toHaveCount(1)
  await expect(card.getByText(expected.code, { exact: true })).toHaveCount(1)
  await expect(card.getByText(expected.name, { exact: true })).toHaveCount(1)
  await expect(cardGain(card)).toHaveCount(1)
  await expect(cardGain(card)).toHaveText(expected.gain)
  await expect(cardValue(card, '評価額')).toHaveText(expected.valuation)
  await expect(cardValue(card, '保有数量')).toHaveText(expected.quantity)
  await expect(cardValue(card, '取得価格')).toHaveText(expected.acquisitionPrice)
  await expect(cardValue(card, '現在の価格')).toHaveText(expected.currentPrice)
  await expect(cardValue(card, '価格の基準日')).toHaveText(expected.priceDate)
}

/**
 * 古い表の行の7セル（銘柄コード・銘柄名・保有数量・取得価格・現在の価格・価格の基準日・評価額。値は変えない）を、カードの項目として確かめる。
 * 含み損益は確かめない（含み損益は holdings-cards.spec.ts で確かめる）。
 */
export async function expectCardData(
  card: Locator,
  cells: readonly [string, string, string, string, string, string, string],
): Promise<void> {
  const [code, name, quantity, acquisitionPrice, currentPrice, priceDate, valuation] = cells
  await expect(card).toHaveCount(1)
  await expect(card.getByText(code, { exact: true })).toHaveCount(1)
  await expect(card.getByText(name, { exact: true })).toHaveCount(1)
  await expect(cardValue(card, '保有数量')).toHaveText(quantity)
  await expect(cardValue(card, '取得価格')).toHaveText(acquisitionPrice)
  await expect(cardValue(card, '現在の価格')).toHaveText(currentPrice)
  await expect(cardValue(card, '価格の基準日')).toHaveText(priceDate)
  await expect(cardValue(card, '評価額')).toHaveText(valuation)
}

/** 合計の欄（region「合計」）。 */
export function summaryRegion(page: Page): Locator {
  return page.getByRole('region', { name: SUMMARY_LABEL, exact: true })
}

/** 合計の欄の中の、通貨ごとの部分（region「円」「ドル」）。 */
export function currencyPart(page: Page, name: '円' | 'ドル'): Locator {
  return summaryRegion(page).getByRole('region', { name, exact: true })
}

/** 通貨ごとの部分の、含み損益と率を持つ要素（data-tone を持つ要素。1つ）。 */
export function partGain(part: Locator): Locator {
  return part.locator('[data-tone]')
}

/** 「価格を今すぐ更新」のボタン。 */
export function refreshButton(page: Page): Locator {
  return page.getByRole('button', { name: REFRESH_BUTTON, exact: true })
}

/** a が b より前（DOM の順）にあること。 */
async function precedes(a: Locator, b: Locator): Promise<boolean> {
  const elementB = await b.elementHandle()
  if (elementB === null) throw new Error('順番を確かめる要素が見つからない')
  return a.evaluate(
    (elementA, other) => (elementA.compareDocumentPosition(other as Node) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0,
    elementB,
  )
}

/** locators が、DOM の順でも画面の上から下の順でも、この順に並んでいること（同じ行の項目は、上端が同じなら前後どちらでもよい）。 */
export async function expectTopToBottom(locators: readonly Locator[], names: readonly string[]): Promise<void> {
  let previousTop = Number.NEGATIVE_INFINITY
  for (let i = 0; i < locators.length; i += 1) {
    await expect(locators[i], `${names[i]} がある`).toHaveCount(1)
    if (i > 0) {
      expect(await precedes(locators[i - 1], locators[i]), `DOM の順で「${names[i - 1]}」の後に「${names[i]}」`).toBe(true)
    }
    const box = await locators[i].boundingBox()
    if (box === null) throw new Error(`${names[i]} の位置を取れない`)
    expect(box.y, `画面で「${names[i - 1] ?? ''}」より下（または同じ行）に「${names[i]}」`).toBeGreaterThanOrEqual(previousTop - 1)
    previousTop = box.y
  }
}

/** 要素とその子孫すべての、計算後の color。 */
export async function computedColors(element: Locator): Promise<string[]> {
  return element.evaluate((root) =>
    [root, ...Array.from(root.querySelectorAll('*'))].map((el) => getComputedStyle(el).color),
  )
}

/** 要素の計算後の color。 */
export async function computedColor(element: Locator): Promise<string> {
  return element.evaluate((el) => getComputedStyle(el).color)
}

/**
 * 含み損益と率を持つ要素（data-tone を持つ要素）の文字の大きさのいちばん小さいものと、
 * container の中のそれ以外の文字（文字を持つ要素）の大きさのいちばん大きいもの（px）。
 * othersTexts は、比べに入れた「それ以外の文字を持つ要素」の文字（何が比べに入ったかを数で確かめるため）。
 */
export async function gainAndOtherFontSizes(
  container: Locator,
): Promise<{ gainMin: number; othersMax: number; othersTexts: string[] }> {
  return container.evaluate((root) => {
    const gainRoots = Array.from(root.querySelectorAll('[data-tone]'))
    const inGain = (el: Element) => gainRoots.some((gain) => gain === el || gain.contains(el))
    const hasOwnText = (el: Element) =>
      Array.from(el.childNodes).some((node) => node.nodeType === Node.TEXT_NODE && (node.textContent ?? '').trim() !== '')
    const sizeOf = (el: Element) => parseFloat(getComputedStyle(el).fontSize)
    const gainSizes = gainRoots.flatMap((gain) => [gain, ...Array.from(gain.querySelectorAll('*'))]).map(sizeOf)
    const others = Array.from(root.querySelectorAll('*')).filter((el) => !inGain(el) && hasOwnText(el))
    return {
      gainMin: Math.min(...gainSizes),
      othersMax: Math.max(...others.map(sizeOf)),
      othersTexts: others.map((el) => (el.textContent ?? '').trim()),
    }
  })
}

/**
 * 要素の文字の色と、その背景の色（透明なら親をたどる。たどっても色がなければ白）の、コントラスト比（WCAG の相対輝度）。
 * 色は不透明（rgb か、alpha が 1 の rgba）の前提。
 */
export async function contrastRatio(element: Locator): Promise<number> {
  const [first] = await contrastRatios(element)
  return first.ratio
}

/**
 * 要素と、その子孫のうち自分の文字を持つすべての要素について、contrastRatio と同じ決め方のコントラスト比を返す
 * （先頭は要素自身。要素自身が文字を持たなくても含める）。
 */
export async function contrastRatios(element: Locator): Promise<Array<{ text: string; ratio: number }>> {
  return element.evaluate((root) => {
    type Rgba = { r: number; g: number; b: number; a: number }
    const parse = (value: string): Rgba => {
      const match = /rgba?\(([^)]+)\)/.exec(value)
      if (match === null) throw new Error(`色を読めない: ${value}`)
      const parts = match[1].split(',').map((part) => parseFloat(part))
      return { r: parts[0], g: parts[1], b: parts[2], a: parts[3] ?? 1 }
    }
    const linear = (channel: number) => {
      const s = channel / 255
      return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
    }
    const luminance = (c: Rgba) => 0.2126 * linear(c.r) + 0.7152 * linear(c.g) + 0.0722 * linear(c.b)
    const ratioOf = (el: Element): number => {
      let background: Rgba = { r: 255, g: 255, b: 255, a: 1 }
      for (let node: Element | null = el; node !== null; node = node.parentElement) {
        const color = parse(getComputedStyle(node).backgroundColor)
        if (color.a > 0) {
          background = color
          break
        }
      }
      const foreground = parse(getComputedStyle(el).color)
      const [lighter, darker] = [luminance(foreground), luminance(background)].sort((x, y) => y - x)
      return (lighter + 0.05) / (darker + 0.05)
    }
    const hasOwnText = (el: Element) =>
      Array.from(el.childNodes).some((node) => node.nodeType === Node.TEXT_NODE && (node.textContent ?? '').trim() !== '')
    const targets = [root, ...Array.from(root.querySelectorAll('*')).filter(hasOwnText)]
    return targets.map((el) => ({ text: (el.textContent ?? '').trim(), ratio: ratioOf(el) }))
  })
}

/** ページ全体が横にスクロールしない（scrollWidth が clientWidth 以下）こと。 */
export async function expectNoHorizontalScroll(page: Page): Promise<void> {
  const widths = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }))
  expect(widths.scrollWidth, 'ページ全体の scrollWidth').toBeLessThanOrEqual(widths.clientWidth)
}
