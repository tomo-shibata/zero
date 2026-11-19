// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeAll, beforeEach, describe, expect, test, vi, type Mock } from 'vitest'
import { HoldingsPage } from '../../../src/features/holdings'

// 仕様: 価格の変動の通知の要件 docs/requirements/price-alerts.md の要件24（/holdings の見出し「保有株式一覧」と同じ行の右端にベルのリンクを置く。
//   見出しを出すときは、保有が0件のときも出す）。ベルそのものは features/notifications の NotificationBell で、E2E（基準29・基準31）と NotificationBell.test.tsx で確かめる。
// 境界: プラン docs/plans/price-alerts/plan.md 4.6・4.7・4.8 の PR②
//   - features/holdings の HoldingsPage は props.headerActions?: ReactNode を受け取り、見出しの行の右端に出す（見出しを行に包む）。読み込み中は今どおり出さない。
//   - app/App.tsx が <HoldingsPage headerActions={<NotificationBell />} /> にする。features/holdings と features/notifications は互いを import しない。
//   - headerActions を渡さなければ、今と同じ（HoldingsPage の今のテストは変えない）。
// このファイルは、HoldingsPage が headerActions を見出しと同じ行に出すことだけを、任意の要素（ここではリンク）で確かめる。
// mock は境界の外側（ネットワーク = グローバルの fetch）だけに使う。

let queryClient: QueryClient
let fetchMock: Mock<typeof fetch>

beforeAll(() => {
  // jsdom の HTMLDialogElement は showModal・close を持たない。取得失敗のダイアログ（AlertDialog）が呼ぶため、代用を足す。
  HTMLDialogElement.prototype.showModal = function showModal(this: HTMLDialogElement) {
    this.setAttribute('open', '')
  }
  HTMLDialogElement.prototype.close = function close(this: HTMLDialogElement) {
    this.removeAttribute('open')
    this.dispatchEvent(new Event('close'))
  }
})

beforeEach(() => {
  queryClient = new QueryClient()
  fetchMock = vi.fn<typeof fetch>(() => new Promise<Response>(() => {}))
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  cleanup()
  queryClient.clear()
  vi.unstubAllGlobals()
})

function renderWithAction() {
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <HoldingsPage headerActions={<a href="/notifications">通知の印</a>} />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

const EMPTY_BODY = { holdings: [], totals: [], combinedTotal: null }
const HEADING = '保有株式一覧'

/** 見出し（h1）と同じ行の中（見出しの親）にあるか。 */
function expectInHeadingRow(action: HTMLElement): void {
  const heading = screen.getByRole('heading', { name: HEADING })
  const row = heading.parentElement
  expect(row).not.toBeNull()
  expect(row).toContainElement(action)
  // 見出しの後（DOM の順で右側）に置く。
  expect(heading.compareDocumentPosition(action) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
}

describe('HoldingsPage の headerActions（要件24）', () => {
  test('要件24: 保有が1件以上あるとき、headerActions は見出し「保有株式一覧」と同じ行の、見出しより後ろに出る', async () => {
    fetchMock.mockImplementation(async () =>
      Response.json({
        holdings: [
          {
            code: '7203', name: 'トヨタ自動車', currency: 'JPY', quantity: 100, acquisitionPrice: '2500.0',
            currentPrice: '3000.0', priceDate: '2026-10-06', valuation: '300000', unrealizedGain: '50000', unrealizedGainRate: '20.0',
          },
        ],
        totals: [{ currency: 'JPY', valuation: '300000', excludesUnpriced: false, unrealizedGain: '50000', unrealizedGainRate: '20.0' }],
        combinedTotal: null,
      }),
    )
    renderWithAction()

    expectInHeadingRow(await screen.findByRole('link', { name: '通知の印' }))
  })

  test('要件24: 保有が0件のとき（「保有株式はありません」の画面）も、headerActions は見出しと同じ行に出る', async () => {
    fetchMock.mockImplementation(async () => Response.json(EMPTY_BODY))
    renderWithAction()

    expect(await screen.findByText('保有株式はありません')).toBeInTheDocument()
    expectInHeadingRow(screen.getByRole('link', { name: '通知の印' }))
  })

  test('要件24: 読み込み中は、今どおり見出しを出さないので、headerActions も出さない', async () => {
    renderWithAction()
    await waitFor(() => expect(fetchMock).toHaveBeenCalled())

    expect(screen.getByRole('status', { name: '読み込み中' })).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: '通知の印' })).not.toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: HEADING })).not.toBeInTheDocument()
  })

  test('補助 要件24: 一覧の取得に失敗して見出しだけ出ているときも（見出しを出すとき）、headerActions は見出しと同じ行に出る', async () => {
    fetchMock.mockImplementation(async () => new Response(JSON.stringify({ error: 'internal_error' }), { status: 500 }))
    renderWithAction()

    expect(await screen.findByRole('alertdialog')).toBeInTheDocument()
    expectInHeadingRow(screen.getByRole('link', { hidden: true, name: '通知の印' }))
  })

  test('headerActions を渡さなければ、見出しの行にほかの要素は出ない（今と同じ）', async () => {
    fetchMock.mockImplementation(async () => Response.json(EMPTY_BODY))
    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <HoldingsPage />
        </MemoryRouter>
      </QueryClientProvider>,
    )

    await screen.findByText('保有株式はありません')

    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })
})
