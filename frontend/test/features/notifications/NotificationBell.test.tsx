// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, test, vi, type Mock } from 'vitest'
import { NotificationBell } from '../../../src/features/notifications'
import { UNSEEN_NOTIFICATION_COUNT_QUERY_KEY } from '../../../src/shared/api/queryKeys'

// 仕様: 価格の変動の通知の要件 docs/requirements/price-alerts.md の要件25（バッジ。0件はバッジなし、100件以上は「99+」）・要件26（読み上げ名）・
//   要件27（数の取得に失敗したときは、バッジを出さずにベルのリンクだけを出し、失敗の文は出さない）・要件28（押すと /notifications に移る）と、
//   成功基準の基準29・基準30・基準33（単体）。
// 境界: プラン docs/plans/price-alerts/plan.md 4.6「画面」・4.8 の PR②
//   - NotificationBell は features/notifications/index.ts から公開され、QueryClientProvider と Router は持たない（app 層で包む）。
//   - DOM: <a href="/notifications" aria-label="通知（新着 3件）">（react-router の Link）。0件は「通知」。バッジは aria-hidden の要素で、文字は「3」か「99+」。
//     読み込み中・0件・取得の失敗では、バッジの要素がなく、名前は「通知」。SVG は aria-hidden。
//   - 見ていない数は GET /api/notifications/unseen-count（{ "count": n }）。query key は shared/api/queryKeys.ts の UNSEEN_NOTIFICATION_COUNT_QUERY_KEY。
//     取得の設定は gcTime: 0（使う画面がなくなったらキャッシュを捨て、開くたびに読み込み中から。古い数が一瞬出ない）。
// テスト計画: プラン 5.1 の基準29・基準30・基準33 の単体の行と、5.2 の NotificationBell.test.tsx の行（押すと /notifications、gcTime: 0）。
// mock は境界の外側（ネットワーク = グローバルの fetch）だけに使う。「バッジの要素がない」は、リンクの中に文字がないことと、
// リンクの直下に SVG 以外の要素がないことで確かめる（バッジのほかに、リンクの中に文字や要素を持つものはない）。

let queryClient: QueryClient
let fetchMock: Mock<typeof fetch>

beforeEach(() => {
  queryClient = new QueryClient()
  // 応答を返さない（終わらない）fetch。「数の取得が完了していない」（読み込み中）状態を作る。
  fetchMock = vi.fn<typeof fetch>(() => new Promise<Response>(() => {}))
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  cleanup()
  queryClient.clear()
  vi.unstubAllGlobals()
})

function respondWithCount(count: number): void {
  fetchMock.mockImplementation(
    async () => new Response(JSON.stringify({ count }), { status: 200, headers: { 'Content-Type': 'application/json' } }),
  )
}

function renderBell() {
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <NotificationBell />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

/** 今の場所（パス）を文字で出す画面（移った先の確認用）。 */
function LocationProbe() {
  return <p data-testid="location">{useLocation().pathname}</p>
}

/** 取得の結果が画面に反映される（TanStack Query は状態の変化を次のタスクで伝える）のを待つ。 */
async function settle(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 20))
  })
}

/** バッジの文字（リンクの中の文字）。バッジの要素がなければ空。 */
function badgeText(link: HTMLElement): string {
  return link.textContent ?? ''
}

/** 「バッジの要素がない」ことの確かめ。文字が空であることに加え、リンクの直下に SVG（アイコン）以外の要素がない（空のバッジの要素を描く実装を通さない）。 */
function expectNoBadge(link: HTMLElement): void {
  expect(badgeText(link)).toBe('')
  expect(link.querySelectorAll(':scope > :not(svg)')).toHaveLength(0)
}

describe('バッジと読み上げ名（基準29・基準30。要件25・要件26）', () => {
  test('基準29: 見ていない通知が3件のとき、/notifications へのリンクの名前は「通知（新着 3件）」で、バッジに「3」が出る（バッジは読み上げない）', async () => {
    respondWithCount(3)
    renderBell()

    const link = await screen.findByRole('link', { name: '通知（新着 3件）' })

    expect(link).toHaveAttribute('href', '/notifications')
    const badge = screen.getByText('3')
    expect(link).toContainElement(badge)
    expect(badge).toHaveAttribute('aria-hidden', 'true')
    expect(String(fetchMock.mock.calls[0][0])).toBe('/api/notifications/unseen-count')
  })

  test('基準30: 見ていない通知が0件のとき、バッジの要素がなく、名前は「通知」', async () => {
    respondWithCount(0)
    renderBell()
    await waitFor(() => expect(fetchMock).toHaveBeenCalled())
    // 数の応答が画面に届くまで待つ（読み込み中も同じ見た目なので、取得が終わったことを待ってから確かめる）。
    await waitFor(() => expect(queryClient.getQueryState(UNSEEN_NOTIFICATION_COUNT_QUERY_KEY)?.status).toBe('success'))
    await settle()

    const link = screen.getByRole('link', { name: '通知' })

    expectNoBadge(link)
  })

  test('基準30: 見ていない通知が120件のとき、バッジは「99+」で、名前は実際の数の「通知（新着 120件）」', async () => {
    respondWithCount(120)
    renderBell()

    const link = await screen.findByRole('link', { name: '通知（新着 120件）' })

    expect(badgeText(link)).toBe('99+')
  })

  test('基準30（境目）: 99件は「99」、100件は「99+」で名前は「通知（新着 100件）」', async () => {
    respondWithCount(99)
    const first = renderBell()
    const link99 = await screen.findByRole('link', { name: '通知（新着 99件）' })
    expect(badgeText(link99)).toBe('99')
    first.unmount()
    await waitFor(() => expect(queryClient.getQueryState(UNSEEN_NOTIFICATION_COUNT_QUERY_KEY)).toBeUndefined())

    respondWithCount(100)
    renderBell()

    const link100 = await screen.findByRole('link', { name: '通知（新着 100件）' })
    expect(badgeText(link100)).toBe('99+')
  })

  test('基準30（読み込み中）: 数の取得が終わっていない間は、バッジの要素がなく、名前は「通知」', async () => {
    renderBell()
    await waitFor(() => expect(fetchMock).toHaveBeenCalled())

    const link = screen.getByRole('link', { name: '通知' })

    expectNoBadge(link)
  })
})

describe('数の取得に失敗したとき（基準33。要件27）', () => {
  test('基準33: 数の取得が 500 のとき、リンクはあり（名前は「通知」）、バッジの要素も失敗の文もない', async () => {
    fetchMock.mockImplementation(async () => new Response(JSON.stringify({ error: 'internal_error' }), { status: 500 }))
    const { container } = renderBell()
    await waitFor(() => expect(queryClient.getQueryState(UNSEEN_NOTIFICATION_COUNT_QUERY_KEY)?.status).toBe('error'))
    await settle()

    const link = screen.getByRole('link', { name: '通知' })

    expectNoBadge(link)
    // リンクのほかに文字はなく、失敗を伝えるダイアログや alert もない。
    expect(container.textContent).toBe('')
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  })

  test('基準33（通信エラー）: fetch が reject しても、リンクはあり、バッジの要素も失敗の文もない', async () => {
    fetchMock.mockImplementation(() => Promise.reject(new TypeError('Failed to fetch')))
    const { container } = renderBell()
    await waitFor(() => expect(queryClient.getQueryState(UNSEEN_NOTIFICATION_COUNT_QUERY_KEY)?.status).toBe('error'))
    await settle()

    const link = screen.getByRole('link', { name: '通知' })

    expectNoBadge(link)
    expect(container.textContent).toBe('')
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  })

  // 数が出た後の取り直し（今すぐ更新の後・見たにした後）の失敗。TanStack Query は失敗しても前の data を残すので、前の数のバッジを残さない。
  test('基準33（取り直しの失敗）: 数3を出した後、取り直しが 500 で失敗すると、バッジの要素がなく、名前は「通知」', async () => {
    respondWithCount(3)
    renderBell()
    await screen.findByRole('link', { name: '通知（新着 3件）' })

    fetchMock.mockImplementation(async () => new Response(JSON.stringify({ error: 'internal_error' }), { status: 500 }))
    await act(async () => {
      await queryClient.invalidateQueries({ queryKey: UNSEEN_NOTIFICATION_COUNT_QUERY_KEY })
    })
    await waitFor(() => expect(queryClient.getQueryState(UNSEEN_NOTIFICATION_COUNT_QUERY_KEY)?.status).toBe('error'))
    await settle()

    const link = screen.getByRole('link', { name: '通知' })

    expectNoBadge(link)
  })

  test('基準33（取り直しの通信エラー）: 数3を出した後、取り直しの fetch が reject すると、バッジの要素がなく、名前は「通知」', async () => {
    respondWithCount(3)
    renderBell()
    await screen.findByRole('link', { name: '通知（新着 3件）' })

    fetchMock.mockImplementation(() => Promise.reject(new TypeError('Failed to fetch')))
    await act(async () => {
      await queryClient.invalidateQueries({ queryKey: UNSEEN_NOTIFICATION_COUNT_QUERY_KEY })
    })
    await waitFor(() => expect(queryClient.getQueryState(UNSEEN_NOTIFICATION_COUNT_QUERY_KEY)?.status).toBe('error'))
    await settle()

    const link = screen.getByRole('link', { name: '通知' })

    expectNoBadge(link)
  })
})

describe('押すと /notifications に移る（要件28）', () => {
  test('要件28: ベルのリンクを押すと、場所が /notifications になる', async () => {
    respondWithCount(3)
    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={['/holdings']}>
          <Routes>
            <Route path="/holdings" element={<NotificationBell />} />
            <Route path="/notifications" element={<LocationProbe />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    )

    fireEvent.click(await screen.findByRole('link', { name: '通知（新着 3件）' }))

    expect(await screen.findByTestId('location')).toHaveTextContent('/notifications')
  })
})

describe('取得の設定（プラン 4.6「取得の設定」。gcTime: 0）', () => {
  test('同じ QueryClient で数3を出してアンマウントし、キャッシュから消えるのを待ってから、応答を返さない fetch でもう一度描くと、バッジの要素がなく名前は「通知」', async () => {
    respondWithCount(3)
    const first = renderBell()
    await screen.findByRole('link', { name: '通知（新着 3件）' })
    first.unmount()
    // gcTime: 0 でも、キャッシュを消すのは次のタスク。消えるのを待つ（待たずに描き直すと、正しい実装でも「3」が出る。clear() は呼ばない）。
    await waitFor(() => expect(queryClient.getQueryState(UNSEEN_NOTIFICATION_COUNT_QUERY_KEY)).toBeUndefined())

    fetchMock.mockImplementation(() => new Promise<Response>(() => {}))
    renderBell()

    const link = screen.getByRole('link', { name: '通知' })
    expectNoBadge(link)
  })
})
