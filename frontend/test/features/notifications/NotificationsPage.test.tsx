// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, test, vi, type Mock } from 'vitest'
import { NotificationsPage } from '../../../src/features/notifications'
import { NOTIFICATIONS_QUERY_KEY } from '../../../src/features/notifications/hooks/useNotifications'
import { UNSEEN_NOTIFICATION_COUNT_QUERY_KEY } from '../../../src/shared/api/queryKeys'
import { fiveNotifications } from './fixtures'

// 仕様: 価格の変動の通知の要件 docs/requirements/price-alerts.md の要件29（左上の戻るリンクと見出し）・要件31（1件の4行）・要件32・要件33（文と色）・
//   要件34（表示できたら、取得した時点までの見ていない通知を見たにする。新着の印はその表示の間だけ。失敗しても文を出さない）・
//   要件35（1件を押すと銘柄ごとの売買履歴の画面に移る）・要件36（0件の文）・要件37（読み込み中と取得の失敗）と、
//   成功基準の基準35・基準36・基準40・基準41（単体）。
//   「条件を決める」のリンク（要件29 の右側）は価格の変動の通知の PR③ で足すので、このファイルでは確かめない。
// 境界: プラン docs/plans/price-alerts/plan.md 4.6「画面」・4.8 の PR②
//   - NotificationsPage は features/notifications/index.ts から公開され、QueryClientProvider と Router は持たない（app 層で包む）。
//   - 左上に「← 保有株式一覧」のリンク（/holdings）、その下に見出し「通知」。リストは <ul role="list">。1件が1つの <li>で、中は1つのリンク
//     （<a>、/holdings/<銘柄コード>）が4行を全部包む。新着は「新着」の文字（リンクの中）。値の要素に data-tone="gain|loss"（色なしは属性なし）。
//   - 一覧は GET /api/notifications（fetchNotifications）。見たには POST /api/notifications/seen、本文 { "upToId": <latestId> }（markNotificationsSeen）。
//   - features/notifications/hooks/useNotifications.ts: NOTIFICATIONS_QUERY_KEY = ['notifications', 'list']。一覧の取得と、表示できた後に1回だけ見たにする
//     （失敗は黙る）。終わったら見ていない数の key（UNSEEN_NOTIFICATION_COUNT_QUERY_KEY）だけを無効にし、一覧の key は無効にしない。
//     一覧は gcTime: 0（使う画面がなくなったらキャッシュを捨て、開くたびに読み込み中から。古い一覧を一瞬出さない）。
// テスト計画: プラン 5.1 の基準35・基準36・基準40・基準41 の単体の行と、5.2 の NotificationsPage.test.tsx の行
//   （gcTime: 0、読み込み中、useNotifications の見たにした後に一覧を取り直さない）。
// mock は境界の外側（ネットワーク = グローバルの fetch）だけに使う。

const NOTIFICATIONS_URL = '/api/notifications'
const SEEN_URL = '/api/notifications/seen'
const BACK_LINK = '← 保有株式一覧'
const EMPTY_TITLE = '通知はまだありません。'
const EMPTY_HINT = '条件に当てはまる終値が保存されると、ここに並びます。'
const FETCH_ERROR = '通知を取得できませんでした。時間をおいて開き直してください。'

let queryClient: QueryClient
let fetchMock: Mock<typeof fetch>
/** 一覧（GET）の応答。 */
let respondList: () => Promise<Response>
/** 見たにする（POST）の応答。 */
let respondSeen: () => Promise<Response>

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

function requestMethod(init?: RequestInit): string {
  return (init?.method ?? 'GET').toUpperCase()
}

/** 呼ばれた fetch のうち、method と url が一致するもの。 */
function callsTo(method: string, url: string) {
  return fetchMock.mock.calls.filter(([input, init]) => String(input) === url && requestMethod(init) === method)
}

beforeEach(() => {
  queryClient = new QueryClient()
  respondList = () => Promise.resolve(jsonResponse(fiveNotifications()))
  respondSeen = () => Promise.resolve(new Response(null, { status: 204 }))
  fetchMock = vi.fn<typeof fetch>((input, init) => {
    const url = String(input)
    const method = requestMethod(init)
    if (url === NOTIFICATIONS_URL && method === 'GET') return respondList()
    if (url === SEEN_URL && method === 'POST') return respondSeen()
    return Promise.resolve(new Response('not found', { status: 404 }))
  })
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  cleanup()
  queryClient.clear()
  vi.unstubAllGlobals()
})

/** 今の場所（パス）を文字で出す画面（移った先の確認用）。 */
function LocationProbe() {
  return <p data-testid="location">{useLocation().pathname}</p>
}

function renderPage() {
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/notifications']}>
        <Routes>
          <Route path="/notifications" element={<NotificationsPage />} />
          <Route path="/holdings" element={<LocationProbe />} />
          <Route path="/holdings/:code" element={<LocationProbe />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

/** 一覧が表示されるのを待ち、通知の <li> を返す。 */
async function findItems(): Promise<HTMLElement[]> {
  const list = await screen.findByRole('list')
  return within(list).getAllByRole('listitem')
}

/** 見たにする（POST）の1回目の要求が出るのを待つ。 */
async function waitForSeenRequest(): Promise<void> {
  await waitFor(() => expect(callsTo('POST', SEEN_URL)).toHaveLength(1))
}

/** 取得の結果が画面に反映される（TanStack Query は状態の変化を次のタスクで伝える）のを待つ。 */
async function settle(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 20))
  })
}

describe('読み込み中（要件37。5.2）', () => {
  test('補助 要件37: 取得が終わっていない間は、「← 保有株式一覧」のリンクと「読み込み中」のローディングのアイコンだけが出る（見出しはない）', async () => {
    respondList = () => new Promise<Response>(() => {})
    const { container } = renderPage()
    await waitFor(() => expect(callsTo('GET', NOTIFICATIONS_URL)).toHaveLength(1))

    expect(screen.getByRole('link', { name: BACK_LINK })).toHaveAttribute('href', '/holdings')
    expect(screen.getByRole('status', { name: '読み込み中' })).toBeInTheDocument()
    expect(screen.queryAllByRole('heading')).toHaveLength(0)
    expect(screen.queryByRole('list')).not.toBeInTheDocument()
    expect(container.textContent).toBe(BACK_LINK)
  })
})

describe('通知のリスト（基準35。要件29・要件31〜要件33）', () => {
  test('基準35: 左上に「← 保有株式一覧」のリンク（/holdings）、その下に見出し「通知」があり、通知が取得した順（取引日の新しい順）に5件並ぶ', async () => {
    renderPage()

    const items = await findItems()

    expect(screen.getByRole('link', { name: BACK_LINK })).toHaveAttribute('href', '/holdings')
    expect(screen.getByRole('heading', { name: '通知' })).toBeInTheDocument()
    expect(String(fetchMock.mock.calls[0][0])).toBe(NOTIFICATIONS_URL)
    expect(items).toHaveLength(5)
    // 1件が1つの <li>で、中は1つのリンク（その銘柄の画面）が全部を包む。
    const hrefs = items.map((item) => {
      const links = within(item).getAllByRole('link')
      expect(links).toHaveLength(1)
      return links[0].getAttribute('href')
    })
    expect(hrefs).toEqual(['/holdings/7203', '/holdings/AAPL', '/holdings/9984', '/holdings/6758', '/holdings/6758'])
  })

  test('基準35: 7203 は「7203 トヨタ自動車」（1つの文字として）「2026-11-14」「取得価格から -20.4%」（-20.4% は loss）「取得価格 2,500円 → 終値 1,990円」「条件: 全銘柄・取得価格から 20% 以上 下がったら」', async () => {
    renderPage()
    const [toyota] = await findItems()

    const link = within(toyota).getByRole('link')
    for (const text of [
      '7203 トヨタ自動車',
      '2026-11-14',
      '取得価格から -20.4%',
      '取得価格 2,500円 → 終値 1,990円',
      '条件: 全銘柄・取得価格から 20% 以上 下がったら',
    ]) {
      expect(link).toHaveTextContent(text)
    }
    const tones = toyota.querySelectorAll('[data-tone]')
    expect(tones).toHaveLength(1)
    expect(tones[0]).toHaveAttribute('data-tone', 'loss')
    expect(tones[0]).toHaveTextContent('-20.4%')
  })

  test('基準35: AAPL は「AAPL Apple」（1つの文字として）「前の取引日から +6.1%」（+6.1% は gain）「前の終値 180.25ドル（2026-11-13）→ 終値 191.24ドル」「条件: 全銘柄・前の取引日から 5% 以上 動いたら（上がっても下がっても）」', async () => {
    renderPage()
    const items = await findItems()

    const apple = items[1]

    for (const text of [
      'AAPL Apple',
      '2026-11-14',
      '前の取引日から +6.1%',
      '前の終値 180.25ドル（2026-11-13）→ 終値 191.24ドル',
      '条件: 全銘柄・前の取引日から 5% 以上 動いたら（上がっても下がっても）',
    ]) {
      expect(apple).toHaveTextContent(text)
    }
    const tones = apple.querySelectorAll('[data-tone]')
    expect(tones).toHaveLength(1)
    expect(tones[0]).toHaveAttribute('data-tone', 'gain')
    expect(tones[0]).toHaveTextContent('+6.1%')
  })

  test('基準35: 9984 は「9984 ソフトバンクグループ」（1つの文字として）「終値 8,120円」（色なし。data-tone の要素がない）「決めた価格 8,000円 以上」「条件: 9984・8,000円 以上になったら」', async () => {
    renderPage()
    const items = await findItems()

    const softbank = items[2]

    for (const text of ['9984 ソフトバンクグループ', '2026-11-13', '終値 8,120円', '決めた価格 8,000円 以上', '条件: 9984・8,000円 以上になったら']) {
      expect(softbank).toHaveTextContent(text)
    }
    expect(softbank.querySelector('[data-tone]')).toBeNull()
  })

  test('基準35: 6758（11-10）は「6758 ソニーグループ」（1つの文字として）「最高値から -15.2%」（loss）「最高値 4,000円（2026-09-30）→ 終値 3,392円」「条件: 全銘柄・最高値から 15% 以上 下がったら」', async () => {
    renderPage()
    const items = await findItems()

    const sony = items[3]

    for (const text of [
      '6758 ソニーグループ',
      '2026-11-10',
      '最高値から -15.2%',
      '最高値 4,000円（2026-09-30）→ 終値 3,392円',
      '条件: 全銘柄・最高値から 15% 以上 下がったら',
    ]) {
      expect(sony).toHaveTextContent(text)
    }
    expect(sony.querySelector('[data-tone]')).toHaveAttribute('data-tone', 'loss')
  })

  test('基準35: 6758（11-04）は「6758 ソニーグループ」（1つの文字として）「含み損益 -104,000円」（loss）「取得価格 4,000円 → 終値 3,480円（200株）」「条件: 6758・含み損が 100,000円 以上になったら」', async () => {
    renderPage()
    const items = await findItems()

    const sony = items[4]

    for (const text of [
      '6758 ソニーグループ',
      '2026-11-04',
      '含み損益 -104,000円',
      '取得価格 4,000円 → 終値 3,480円（200株）',
      '条件: 6758・含み損が 100,000円 以上になったら',
    ]) {
      expect(sony).toHaveTextContent(text)
    }
    const tones = sony.querySelectorAll('[data-tone]')
    expect(tones).toHaveLength(1)
    expect(tones[0]).toHaveAttribute('data-tone', 'loss')
    expect(tones[0]).toHaveTextContent('-104,000円')
  })
})

describe('新着の印（基準36。要件34）', () => {
  test('基準36: 取得した時点で見ていなかった通知（isNew）の3件に「新着」の印（リンクの中）が出て、見た2件には出ない。見たにした後も、その表示の間は出たまま', async () => {
    renderPage()
    const items = await findItems()
    await waitForSeenRequest()
    await settle()

    items.slice(0, 3).forEach((item) => {
      expect(within(within(item).getByRole('link')).getByText('新着')).toBeInTheDocument()
    })
    items.slice(3).forEach((item) => {
      expect(within(item).queryByText('新着')).not.toBeInTheDocument()
    })
    expect(screen.getAllByText('新着')).toHaveLength(3)
  })
})

describe('見たにする（基準36。要件34）', () => {
  test('基準36: 一覧を表示した後に POST /api/notifications/seen が1回だけ呼ばれ、本文の upToId は一覧に出ている通知の最大の ID ではなく取得した latestId（"15"）', async () => {
    renderPage()
    await findItems()

    await waitForSeenRequest()
    await settle()

    expect(callsTo('POST', SEEN_URL)).toHaveLength(1)
    const [, init] = callsTo('POST', SEEN_URL)[0]
    expect(JSON.parse(String(init?.body))).toEqual({ upToId: '15' })
    // 一覧を取得してから見たにする。
    const urls = fetchMock.mock.calls.map(([input, requestInit]) => `${requestMethod(requestInit)} ${String(input)}`)
    expect(urls).toEqual([`GET ${NOTIFICATIONS_URL}`, `POST ${SEEN_URL}`])
  })

  test('基準36: 見たにする（POST）が 500 でも、失敗の文もダイアログも出さず、一覧は表示したまま', async () => {
    respondSeen = () => Promise.resolve(jsonResponse({ error: 'internal_error' }, 500))
    renderPage()
    await findItems()

    await waitForSeenRequest()
    await settle()

    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    expect(screen.queryByText(FETCH_ERROR)).not.toBeInTheDocument()
    expect(screen.getAllByRole('listitem')).toHaveLength(5)
    expect(screen.getAllByText('新着')).toHaveLength(3)
  })

  test('基準36（通信エラー）: 見たにする（POST）の通信が失敗しても、文を出さず、一覧は表示したまま', async () => {
    respondSeen = () => Promise.reject(new TypeError('Failed to fetch'))
    renderPage()
    await findItems()

    await waitForSeenRequest()
    await settle()

    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    expect(screen.getAllByRole('listitem')).toHaveLength(5)
  })

  test('補助 要件34: 通知が0件（latestId が null）のときは、見たにする（POST）を呼ばない（送る ID がない）', async () => {
    respondList = () => Promise.resolve(jsonResponse({ items: [], latestId: null }))
    renderPage()
    await screen.findByText(EMPTY_TITLE)

    await settle()

    expect(callsTo('POST', SEEN_URL)).toHaveLength(0)
  })

  test('補助 5.2: 見たにした後に、一覧を取り直さない（visibilitychange の後も GET は1回のまま）。見ていない数の key だけが無効になり、一覧の key は無効にならない', async () => {
    // ベルを描かないので、見ていない数の取り直しは起きない。無効になったことだけを確かめる（取り直しの回数では確かめない）。
    queryClient.setQueryData(UNSEEN_NOTIFICATION_COUNT_QUERY_KEY, 3)
    renderPage()
    await findItems()
    await waitForSeenRequest()
    await waitFor(() => expect(queryClient.getQueryState(UNSEEN_NOTIFICATION_COUNT_QUERY_KEY)?.isInvalidated).toBe(true))

    // TanStack Query v5 が聞くのは focus ではなく visibilitychange。取り直しを判定するのは resumePausedMutations を await した後なので、タスクを1つ待ってから数える。
    await act(async () => {
      window.dispatchEvent(new Event('visibilitychange'))
      await new Promise((resolve) => setTimeout(resolve, 0))
    })

    expect(callsTo('GET', NOTIFICATIONS_URL)).toHaveLength(1)
    const listState = queryClient.getQueryState(NOTIFICATIONS_QUERY_KEY)
    expect(listState).toBeDefined()
    expect(listState?.isInvalidated).toBe(false)
  })
})

describe('一覧のキャッシュ（5.2。gcTime: 0）', () => {
  test('新着3件の一覧を出し、見たにする POST が返った後にアンマウントして、キャッシュから消えるのを待ってから、応答を返さない fetch でもう一度描くと、戻るリンクと「読み込み中」だけで「新着」がない', async () => {
    queryClient.setQueryData(UNSEEN_NOTIFICATION_COUNT_QUERY_KEY, 3)
    const first = renderPage()
    await findItems()
    await waitForSeenRequest()
    // 見たにする POST が返った（見ていない数の key が無効になった）後にアンマウントする。
    await waitFor(() => expect(queryClient.getQueryState(UNSEEN_NOTIFICATION_COUNT_QUERY_KEY)?.isInvalidated).toBe(true))
    first.unmount()
    // gcTime: 0 でも、キャッシュを消すのは次のタスク。消えるのを待つ（clear() は呼ばない）。
    await waitFor(() => expect(queryClient.getQueryState(NOTIFICATIONS_QUERY_KEY)).toBeUndefined())

    respondList = () => new Promise<Response>(() => {})
    const { container } = renderPage()

    expect(await screen.findByRole('status', { name: '読み込み中' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: BACK_LINK })).toBeInTheDocument()
    expect(screen.queryByText('新着')).not.toBeInTheDocument()
    expect(container.textContent).toBe(BACK_LINK)
  })
})

describe('1件を押す（基準37。要件35）', () => {
  test('基準37: 7203 の通知（のリンク）を押すと、場所が /holdings/7203 になる', async () => {
    renderPage()
    const [toyota] = await findItems()

    fireEvent.click(within(toyota).getByRole('link'))

    expect(await screen.findByTestId('location')).toHaveTextContent('/holdings/7203')
  })

  test('要件35: 戻るリンク「← 保有株式一覧」を押すと、場所が /holdings になる', async () => {
    renderPage()
    await findItems()

    fireEvent.click(screen.getByRole('link', { name: BACK_LINK }))

    expect(await screen.findByTestId('location')).toHaveTextContent('/holdings')
  })
})

describe('0件（基準40。要件36）', () => {
  test('基準40: 通知が0件のとき、「通知はまだありません。」と「条件に当てはまる終値が保存されると、ここに並びます。」が出る（戻るリンクと見出しもある）', async () => {
    respondList = () => Promise.resolve(jsonResponse({ items: [], latestId: null }))
    renderPage()

    expect(await screen.findByText(EMPTY_TITLE)).toBeInTheDocument()
    expect(screen.getByText(EMPTY_HINT)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: BACK_LINK })).toHaveAttribute('href', '/holdings')
    expect(screen.getByRole('heading', { name: '通知' })).toBeInTheDocument()
    expect(screen.queryAllByRole('listitem')).toHaveLength(0)
  })
})

describe('取得の失敗（基準41。要件37）', () => {
  test('基準41: 一覧が 500 のとき、リストの代わりに「通知を取得できませんでした。時間をおいて開き直してください。」が出て、「← 保有株式一覧」と見出し「通知」もある', async () => {
    respondList = () => Promise.resolve(jsonResponse({ error: 'internal_error' }, 500))
    renderPage()

    // 失敗の文は role="alert"（読み上げに伝える）。
    expect(await screen.findByRole('alert')).toHaveTextContent(FETCH_ERROR)
    expect(screen.getByRole('link', { name: BACK_LINK })).toHaveAttribute('href', '/holdings')
    expect(screen.getByRole('heading', { name: '通知' })).toBeInTheDocument()
    expect(screen.queryByRole('list')).not.toBeInTheDocument()
    expect(screen.queryByText(EMPTY_TITLE)).not.toBeInTheDocument()
    // 取得に失敗したので、見たにはしない。
    await settle()
    expect(callsTo('POST', SEEN_URL)).toHaveLength(0)
  })

  test('基準41（通信エラー）: fetch が reject しても、失敗の文と「← 保有株式一覧」・見出しが出る', async () => {
    respondList = () => Promise.reject(new TypeError('Failed to fetch'))
    renderPage()

    expect(await screen.findByRole('alert')).toHaveTextContent(FETCH_ERROR)
    expect(screen.getByRole('link', { name: BACK_LINK })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '通知' })).toBeInTheDocument()
  })

  test('補助 要件37（形の誤り）: 応答の形が誤っている（200 だが items が null）ときも、失敗の文が出る（0件の表示にはならない）', async () => {
    respondList = () => Promise.resolve(jsonResponse({ items: null, latestId: null }))
    renderPage()

    expect(await screen.findByRole('alert')).toHaveTextContent(FETCH_ERROR)
    expect(screen.queryByText(EMPTY_TITLE)).not.toBeInTheDocument()
  })

  // プラン 6章の判断14: 1件でも形が誤り（種類に必要な値がない）なら、一覧の全体を失敗にする（保有株式一覧と同じ）。見たにもしない。
  test('補助 判断14: 一覧の5件のうち1件だけ、取得価格から（cost）の通知の率が null のとき、一覧は出ず失敗の文が出て、見たにする POST は呼ばれない', async () => {
    const body = fiveNotifications()
    body.items[0] = { ...body.items[0], rate: null }
    expect(body.items[0].condition.kind).toBe('cost')
    respondList = () => Promise.resolve(jsonResponse(body))
    renderPage()

    expect(await screen.findByRole('alert')).toHaveTextContent(FETCH_ERROR)
    expect(screen.queryByRole('list')).not.toBeInTheDocument()
    expect(screen.queryAllByRole('listitem')).toHaveLength(0)
    await settle()
    expect(callsTo('POST', SEEN_URL)).toHaveLength(0)
  })
})
