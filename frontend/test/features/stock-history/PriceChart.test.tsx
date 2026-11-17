// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import type { StockHistory } from '../../../src/entities/stock-history/model'
import { PriceChart } from '../../../src/features/stock-history/components/PriceChart'
import { appleHistory, baseline7, close, sameDayHistory, toyotaHistory, trade } from './fixtures'

// 仕様: 要件 docs/requirements/stock-history.md の要件8〜要件13、基準9〜基準11・基準14・基準15（単体）。
// 境界: プラン docs/plans/stock-history/plan.md 4.6「画面」と 4.8「PR② 画面とカードからの移動」の features/stock-history/components/PriceChart.tsx
//   PriceChart({ history })。DOM の約束（テストはこれで探す。見た目の値は約束しない）:
//   - <svg role="img" aria-label="<銘柄コード> <銘柄名>の価格のチャート">（要件13）
//   - 線: 終値が2点以上なら data-chart-part="line" の要素が1つ。1点なら data-chart-part="point" の要素が1つ。0件ならどちらもなく、
//     「この期間の価格はまだありません」の文字を出す（要件12）
//   - 印: 売買1件に1つ、data-chart-part="marker"・data-side="buy|sell"・data-date="YYYY-MM-DD" を持つ要素。並びは markers の順。
//     買いと売りで描く形（points か d の属性）が違う（要件8 の「色だけで区別しない」）
//   - 凡例: 「▲ 買い ▼ 売り」の文字
//   - 縦軸の目盛りの文字: data-chart-part="y-tick" の text 要素（文字は formatAxisPrice）
//   - 受け取る領域: data-chart-part="plot-area" の rect が1つ（描く範囲を覆う）。この要素の pointermove と pointerdown の clientX を、
//     この要素の getBoundingClientRect() の left〜right に対して xRange の from〜to に線形に換えた日付で、nearestClose の点を選ぶ。
//     テストはこの要素の getBoundingClientRect を固定の値に差し替えて座標を決める。pointerleave で消すのは pointerType が touch 以外（mouse・pen）のとき
//     （ステップ6 1ラウンド目のエッジケース5）。
//     タッチの吹き出しはチャートの外の pointerdown で消す
//   - 印の吹き出し: 印の要素の pointerenter（マウス）と pointerdown（タッチ）で、同じ取引日の売買を出す。印の要素の pointerleave でも、
//     pointerType が touch 以外なら消す。チャートの外の pointerdown で消す
//   - 吹き出し: role="tooltip" の要素が1つ。1行が1つの子要素
// テスト計画: プラン 5.1 の基準9・10・11・14・15 の PriceChart.test.tsx の行と、5.2 の PriceChart.test.tsx の行。

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

const TOYOTA_LABEL = '7203 トヨタ自動車の価格のチャート'

function renderChart(history: StockHistory) {
  return render(<PriceChart history={history} />)
}

function parts(container: HTMLElement, name: string): Element[] {
  return Array.from(container.querySelectorAll(`[data-chart-part="${name}"]`))
}

function markers(container: HTMLElement): Element[] {
  return parts(container, 'marker')
}

function plotArea(container: HTMLElement): Element {
  const areas = parts(container, 'plot-area')
  expect(areas, 'plot-area は1つ').toHaveLength(1)
  return areas[0]
}

/** 吹き出しの行（role="tooltip" の子要素の文字）。 */
function tooltipLines(): string[] {
  return Array.from(screen.getByRole('tooltip').children).map((child) => child.textContent ?? '')
}

/** plot-area の getBoundingClientRect を、固定の値（left〜right）に差し替える。 */
function fixRect(area: Element, rect: { left: number; right: number }): void {
  const width = rect.right - rect.left
  vi.spyOn(area, 'getBoundingClientRect').mockReturnValue({
    left: rect.left,
    right: rect.right,
    width,
    x: rect.left,
    y: 0,
    top: 0,
    bottom: 100,
    height: 100,
    toJSON: () => ({}),
  })
}

/** 形を決める属性（points・d・transform）を、印とその子孫から集める。買いと売りの形が違うかを、位置が同じ入力で比べるために使う。 */
function shapeOf(marker: Element): string {
  const targets = [marker, ...Array.from(marker.querySelectorAll('*'))]
  return targets
    .map((element) => ['points', 'd', 'transform'].map((name) => `${name}=${element.getAttribute(name) ?? ''}`).join(';'))
    .join('|')
}

describe('チャートの名前（基準15。要件13）', () => {
  test('基準15: 「7203 トヨタ自動車の価格のチャート」という名前の img（svg）がある', () => {
    renderChart(baseline7())

    const chart = screen.getByRole('img', { name: TOYOTA_LABEL })
    expect(chart.tagName.toLowerCase()).toBe('svg')
  })

  test('基準15（通貨・銘柄は history.stock から）: AAPL なら「AAPL Appleの価格のチャート」', () => {
    renderChart(appleHistory([trade('buy', '2026-01-10', 10, '150.25', { currency: 'USD' })], [close('2026-10-06', '180.00')]))

    expect(screen.getByRole('img', { name: 'AAPL Appleの価格のチャート' })).toBeInTheDocument()
  })
})

describe('印と凡例（基準9。要件8）', () => {
  test('基準9: 凡例「▲ 買い ▼ 売り」があり、印（marker）は3つで、data-side は buy 2つ・sell 1つ', () => {
    const { container } = renderChart(sameDayHistory())

    expect(screen.getByText('▲ 買い ▼ 売り')).toBeInTheDocument()
    const all = markers(container)
    expect(all).toHaveLength(3)
    expect(all.filter((marker) => marker.getAttribute('data-side') === 'buy')).toHaveLength(2)
    expect(all.filter((marker) => marker.getAttribute('data-side') === 'sell')).toHaveLength(1)
  })

  test('基準9: 印は markers の順（entries と同じ。2026-03-02 の売、2026-01-10 の買、2026-01-10 の買）に data-date と data-side を持つ', () => {
    const { container } = renderChart(sameDayHistory())

    expect(markers(container).map((marker) => [marker.getAttribute('data-date'), marker.getAttribute('data-side')])).toEqual([
      ['2026-03-02', 'sell'],
      ['2026-01-10', 'buy'],
      ['2026-01-10', 'buy'],
    ])
  })

  test('基準9（要件8: 色だけで区別しない）: 売買の位置が同じ入力で、買いと売りの印は、描く形の属性（points・d・transform）が違う', () => {
    const buy = renderChart(
      toyotaHistory([trade('buy', '2026-01-10', 100, '2500.0')], [close('2026-01-10', '2450.0'), close('2026-10-06', '3000.0')]),
    )
    const buyShape = shapeOf(markers(buy.container)[0])
    cleanup()
    const sell = renderChart(
      toyotaHistory([trade('sell', '2026-01-10', 100, '2500.0')], [close('2026-01-10', '2450.0'), close('2026-10-06', '3000.0')]),
    )
    const sellShape = shapeOf(markers(sell.container)[0])

    expect(buyShape).not.toBe(sellShape)
  })
})

describe('線と点・終値がないとき（基準14。要件7・要件12）', () => {
  test('基準14: 終値が1件もないとき、line と point の要素がなく、印が2つ、「この期間の価格はまだありません」と出る', () => {
    const history = {
      stock: { code: '9984', name: 'ソフトバンクグループ', currency: 'JPY' as const },
      entries: [trade('sell', '2026-02-10', 50, '7000.0'), trade('buy', '2026-01-10', 100, '6000.0')],
      closes: [],
    }
    const { container } = renderChart(history)

    expect(parts(container, 'line')).toHaveLength(0)
    expect(parts(container, 'point')).toHaveLength(0)
    expect(markers(container)).toHaveLength(2)
    expect(screen.getByText('この期間の価格はまだありません')).toBeInTheDocument()
  })

  test('補助 要件7: 終値が2点以上なら line が1つで、point はなく、「この期間の価格はまだありません」は出ない', () => {
    const { container } = renderChart(baseline7())

    expect(parts(container, 'line')).toHaveLength(1)
    expect(parts(container, 'point')).toHaveLength(0)
    expect(screen.queryByText('この期間の価格はまだありません')).not.toBeInTheDocument()
  })

  test('補助 プラン 4.6: 終値が1点だけなら、line ではなく point が1つで、「この期間の価格はまだありません」は出ない', () => {
    const { container } = renderChart(
      toyotaHistory([trade('buy', '2026-01-10', 100, '2500.0')], [close('2026-10-06', '3000.0')]),
    )

    expect(parts(container, 'point')).toHaveLength(1)
    expect(parts(container, 'line')).toHaveLength(0)
    expect(screen.queryByText('この期間の価格はまだありません')).not.toBeInTheDocument()
  })
})

describe('範囲が1点のとき（プラン 5.2）', () => {
  /** svg の中のどの属性にも、文字にも、NaN・Infinity がない。 */
  function expectNoNonFinite(container: HTMLElement): void {
    const svg = container.querySelector('svg')
    expect(svg).not.toBeNull()
    for (const element of [svg as Element, ...Array.from((svg as Element).querySelectorAll('*'))]) {
      for (const attribute of Array.from(element.attributes)) {
        expect(attribute.value, `${element.tagName} の ${attribute.name}`).not.toMatch(/NaN|Infinity/)
      }
    }
    expect(svg?.textContent ?? '').not.toMatch(/NaN|Infinity/)
  }

  test('補助: 「買 2,500円 100株 2026-11-16」だけで終値なしのとき、印が1つ描かれ、SVG のどの属性にも「NaN」「Infinity」がない', () => {
    const { container } = renderChart(toyotaHistory([trade('buy', '2026-11-16', 100, '2500.0')], []))

    expect(markers(container)).toHaveLength(1)
    expect(parts(container, 'line')).toHaveLength(0)
    expectNoNonFinite(container)
  })

  test('補助: 売買も終値も 2026-11-16 の1点だけのとき、point が1つ・印が1つ描かれ、SVG のどの属性にも「NaN」「Infinity」がない', () => {
    const { container } = renderChart(toyotaHistory([trade('buy', '2026-11-16', 100, '2500.0')], [close('2026-11-16', '2500.0')]))

    expect(parts(container, 'point')).toHaveLength(1)
    expect(markers(container)).toHaveLength(1)
    expectNoNonFinite(container)
  })
})

describe('縦軸の目盛り（基準10。要件6）', () => {
  test('基準10: AAPL（ドル）の縦軸の目盛りの文字（y-tick）が1つ以上あり、どれも「ドル」で小数2桁（/^[0-9,]+\\.[0-9]{2}ドル$/）', () => {
    const { container } = renderChart(
      appleHistory([trade('buy', '2026-01-10', 10, '150.25', { currency: 'USD' })], [close('2026-10-06', '180.00')]),
    )

    const ticks = parts(container, 'y-tick')
    expect(ticks.length).toBeGreaterThanOrEqual(1)
    for (const tick of ticks) {
      expect(tick.textContent).toMatch(/^[0-9,]+\.[0-9]{2}ドル$/)
    }
  })

  test('補助 要件6: 7203（円）の縦軸の目盛りの文字は、どれも「円」で終わる', () => {
    const { container } = renderChart(baseline7())

    const ticks = parts(container, 'y-tick')
    expect(ticks.length).toBeGreaterThanOrEqual(1)
    for (const tick of ticks) {
      expect(tick.textContent).toMatch(/^[0-9,]+(\.[0-9])?円$/)
    }
  })
})

describe('線の上の吹き出し（基準10。要件10）', () => {
  test('基準10: AAPL の 2026-10-06 の終値 180.00ドルの位置（plot-area の右端）に pointermove を送ると、role="tooltip" に「2026-10-06 180.00ドル」が出る。送る前は吹き出しがない', () => {
    const { container } = renderChart(
      appleHistory([trade('buy', '2026-01-10', 10, '150.25', { currency: 'USD' })], [close('2026-10-06', '180.00')]),
    )
    const area = plotArea(container)
    // 横軸は 2026-01-10 から 2026-10-06 の 269日。1日 1px の幅 269 にする。
    fixRect(area, { left: 300, right: 569 })
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()

    fireEvent.pointerMove(area, { clientX: 569, clientY: 50, pointerType: 'mouse' })

    expect(tooltipLines()).toEqual(['2026-10-06 180.00ドル'])
  })

  test('補助 5.2（pointermove の位置の換え方）: 基準7 のデータで plot-area を { left: 300, right: 569 }（1日 1px）にし、clientX: 310 の pointermove で「2026-01-10 2,450円」。続けて pointerleave（mouse）で吹き出しが消える', () => {
    const { container } = renderChart(baseline7())
    const area = plotArea(container)
    fixRect(area, { left: 300, right: 569 })

    // 10日目（2026-01-20）。2026-01-10 まで10日、2026-03-02 まで41日なので、いちばん近いのは 2026-01-10。
    // left を引き忘れる・左右を逆にする誤りなら 2026-10-06 になる。
    fireEvent.pointerMove(area, { clientX: 310, clientY: 50, pointerType: 'mouse' })
    expect(tooltipLines()).toEqual(['2026-01-10 2,450円'])

    fireEvent.pointerLeave(area, { pointerType: 'mouse' })
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
  })

  // エッジケース5: 消すのは pointerType が touch 以外のとき（pen もマウスと同じで、外へ出たら消す）。touch は指を離した直後にも pointerleave が来るので消さない。
  for (const pointerType of ['pen', 'mouse'] as const) {
    test(`補助 エッジケース5: pointerType が ${pointerType} の pointermove で吹き出しが出て、pointerleave（${pointerType}）で消える`, () => {
      const { container } = renderChart(baseline7())
      const area = plotArea(container)
      fixRect(area, { left: 300, right: 569 })

      fireEvent.pointerMove(area, { clientX: 310, clientY: 50, pointerType })
      expect(tooltipLines()).toEqual(['2026-01-10 2,450円'])

      fireEvent.pointerLeave(area, { pointerType })
      expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
    })

    test(`補助 エッジケース5: 印の pointerdown（${pointerType}）で出した吹き出しは、印の pointerleave（${pointerType}）で消える`, () => {
      const { container } = renderChart(sameDayHistory())
      const marker = markers(container)[1]

      fireEvent.pointerDown(marker, { pointerType })
      expect(screen.getByRole('tooltip')).toBeInTheDocument()

      fireEvent.pointerLeave(marker, { pointerType })
      expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
    })
  }

  test('補助 5.2（線のタップ）: pointermove を送らずに、同じ位置（clientX: 310）に pointerdown（touch）を送ると「2026-01-10 2,450円」が出る。pointerleave（touch）を送っても残り、チャートの外（document.body）に pointerdown を送ると消える', () => {
    const { container } = renderChart(baseline7())
    const area = plotArea(container)
    fixRect(area, { left: 300, right: 569 })

    fireEvent.pointerDown(area, { clientX: 310, clientY: 50, pointerType: 'touch' })
    expect(tooltipLines()).toEqual(['2026-01-10 2,450円'])

    fireEvent.pointerLeave(area, { pointerType: 'touch' })
    expect(tooltipLines()).toEqual(['2026-01-10 2,450円'])

    fireEvent.pointerDown(document.body, { pointerType: 'touch' })
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
  })

  test('補助 5.2（右端の換え方）: 同じ設定で clientX: 569（右端）の pointermove は「2026-10-06 3,000円」（左右を取り違えない）', () => {
    const { container } = renderChart(baseline7())
    const area = plotArea(container)
    fixRect(area, { left: 300, right: 569 })

    fireEvent.pointerMove(area, { clientX: 569, clientY: 50, pointerType: 'mouse' })

    expect(tooltipLines()).toEqual(['2026-10-06 3,000円'])
  })
})

describe('印の吹き出し（基準11。要件11）', () => {
  test('基準11: 重なった 2026-01-10 の2つの印のどちらに pointerenter（mouse）しても、同じ2行が同じ順（10株 2,520円、100株 2,500円）で出る', () => {
    const { container } = renderChart(sameDayHistory())
    const sameDay = markers(container).filter((marker) => marker.getAttribute('data-date') === '2026-01-10')
    expect(sameDay).toHaveLength(2)
    const expected = ['2026-01-10 買 10株 2,520円', '2026-01-10 買 100株 2,500円']

    fireEvent.pointerEnter(sameDay[0], { pointerType: 'mouse' })
    expect(tooltipLines()).toEqual(expected)
    fireEvent.pointerLeave(sameDay[0], { pointerType: 'mouse' })
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()

    fireEvent.pointerEnter(sameDay[1], { pointerType: 'mouse' })
    expect(tooltipLines()).toEqual(expected)
  })

  test('補助 5.2（印のタップ）: 2026-01-10 の印に pointerdown（touch）を送ると2行の吹き出しが出て、チャートの外（document.body）に pointerdown を送ると消える', () => {
    const { container } = renderChart(sameDayHistory())
    const marker = markers(container).find((candidate) => candidate.getAttribute('data-date') === '2026-01-10')
    expect(marker).toBeDefined()

    fireEvent.pointerDown(marker as Element, { pointerType: 'touch' })
    expect(tooltipLines()).toEqual(['2026-01-10 買 10株 2,520円', '2026-01-10 買 100株 2,500円'])

    fireEvent.pointerDown(document.body, { pointerType: 'touch' })
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
  })

  test('補助 プラン 4.6: 印の吹き出しは1つだけ（role="tooltip" が1つ）で、売り 1件の印（2026-03-02）の吹き出しは1行「2026-03-02 売 50株 3,050円」', () => {
    const { container } = renderChart(sameDayHistory())
    const sell = markers(container).find((candidate) => candidate.getAttribute('data-side') === 'sell')

    fireEvent.pointerEnter(sell as Element, { pointerType: 'mouse' })

    expect(screen.getAllByRole('tooltip')).toHaveLength(1)
    expect(tooltipLines()).toEqual(['2026-03-02 売 50株 3,050円'])
  })

  test('補助 要件11（割った印）: 分割 1→3 の前の売買の印の吹き出しは「2026-01-10 買 100株 2,500円」「分割を反映した価格 833.3円」。分割の日の売買の印は1行だけ', () => {
    const history = toyotaHistory(
      [
        trade('buy', '2026-03-31', 30, '1000.0'),
        trade('buy', '2026-01-10', 100, '2500.0', { splitDivisor: '3', adjustedPrice: '833.3' }),
      ],
      [close('2026-03-30', '3100.0', { splitDivisor: '3', adjustedPrice: '1033.3' }), close('2026-03-31', '1050.0')],
    )
    const { container } = renderChart(history)
    const byDate = (date: string) => markers(container).find((marker) => marker.getAttribute('data-date') === date) as Element

    fireEvent.pointerEnter(byDate('2026-01-10'), { pointerType: 'mouse' })
    expect(tooltipLines()).toEqual(['2026-01-10 買 100株 2,500円', '分割を反映した価格 833.3円'])
    fireEvent.pointerLeave(byDate('2026-01-10'), { pointerType: 'mouse' })

    fireEvent.pointerEnter(byDate('2026-03-31'), { pointerType: 'mouse' })
    expect(tooltipLines()).toEqual(['2026-03-31 買 30株 1,000円'])
  })

  // エッジケース2-2: ペンを浮かせて印の上に動かしたとき（pointerenter、pointerType が pen）もマウスと同じで、吹き出しが出る。
  // タッチは、指が触れた pointerdown で出すので、pointerenter では出さない。
  test('エッジケース2-2: 印の pointerenter（pen）で、その取引日の吹き出しが出る', () => {
    const { container } = renderChart(sameDayHistory())
    const marker = markers(container).find((candidate) => candidate.getAttribute('data-date') === '2026-01-10') as Element

    fireEvent.pointerEnter(marker, { pointerType: 'pen' })

    expect(tooltipLines()).toEqual(['2026-01-10 買 10株 2,520円', '2026-01-10 買 100株 2,500円'])
  })

  test('エッジケース2-2: 印の pointerenter（touch）では、吹き出しは出ない', () => {
    const { container } = renderChart(sameDayHistory())
    const marker = markers(container)[1]

    fireEvent.pointerEnter(marker, { pointerType: 'touch' })

    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
  })

  test('補助 プラン 4.8: 印の pointerleave（touch）では消えない。マウスなら消える', () => {
    const { container } = renderChart(sameDayHistory())
    const marker = markers(container)[1]

    fireEvent.pointerDown(marker, { pointerType: 'touch' })
    fireEvent.pointerLeave(marker, { pointerType: 'touch' })
    expect(screen.getByRole('tooltip')).toBeInTheDocument()

    fireEvent.pointerDown(document.body, { pointerType: 'touch' })
    fireEvent.pointerEnter(marker, { pointerType: 'mouse' })
    fireEvent.pointerLeave(marker, { pointerType: 'mouse' })
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
  })
})

describe('箱の大きさが変わったとき（ステップ6 1ラウンド目のエッジケース。プラン 4.6）', () => {
  // 吹き出しの位置は、指す点を決めたときの箱の大きさで決める。箱の大きさが変わる（画面の向きの変更など）と位置が合わなくなるので、消す。
  // jsdom には ResizeObserver がないので、差し替えて通知を送る。箱（.price-chart__canvas）の大きさは getBoundingClientRect の値（描き方は 640×320 から始まる）。
  test('補助: 線の吹き出しが出ているときに、箱の大きさが 640×320 から 390×300 に変わる通知が届くと、吹き出し（role="tooltip"）が消える', () => {
    let notify: (() => void) | undefined
    vi.stubGlobal(
      'ResizeObserver',
      class {
        constructor(callback: () => void) {
          notify = callback
        }
        observe(): void {}
        unobserve(): void {}
        disconnect(): void {}
      },
    )
    let canvasSize = { width: 640, height: 320 }
    vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(function (this: Element) {
      const { width, height } = this.classList.contains('price-chart__canvas') ? canvasSize : { width: 0, height: 0 }
      return { left: 0, top: 0, right: width, bottom: height, x: 0, y: 0, width, height, toJSON: () => ({}) }
    })
    const { container } = renderChart(baseline7())
    const area = plotArea(container)
    fixRect(area, { left: 300, right: 569 })

    fireEvent.pointerMove(area, { clientX: 310, clientY: 50, pointerType: 'mouse' })
    expect(tooltipLines()).toEqual(['2026-01-10 2,450円'])

    canvasSize = { width: 390, height: 300 }
    act(() => {
      notify?.()
    })

    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
  })
})
