import pg from 'pg'

// E2E から DB（zero_e2e）のデータを入れ替えるヘルパー。
// プラン docs/plans/stock-holdings/plan.md 4.5「E2E」:
//   - zero_e2e は、API の webServer（task e2e:api）が起動時に cmd/migrate -recreate -seed db/seed/test.sql で作り直す。
//   - E2E から DB を操作する pg の接続先は、zero_e2e に固定した定数にする。DB 名が zero_e2e でなければ失敗させる。
//   - 各テストの beforeEach で TRUNCATE trades, stock_prices と、test.sql にない銘柄マスタの削除を行ってから、
//     そのテストのデータを入れる。
// 売買履歴は利用者A のものだけを入れる（E2E の API は DEV_USER_ID=利用者A で起動する。プラン 4.5）。
// 売買履歴への作り変え（docs/plans/trade-history/plan.md の 4.6・4.7 の PR①）で、保有データ（holdings）を売買履歴（trades）に替えた。
// PR② で、銘柄マスタに通貨を足した（StockSeed.currency は必須）。test.sql の銘柄（7203・6758・9984）はどれも JPY。AAPL・MSFT・GOOGL などのドルの銘柄は、各テストで stocks に入れる。

/** E2E 専用の DB 名（プラン 4.5）。開発用 DB（zero）やテスト用 DB（zero_test_…）を誤って書き換えないよう、これ以外には接続しない。 */
export const E2E_DATABASE_NAME = 'zero_e2e'

/**
 * 接続先（固定）。docker-compose.yml の PostgreSQL（127.0.0.1:5432、利用者 zero）。
 * 環境変数（DATABASE_URL・PGDATABASE など）からは決めない（接続先を差し替えられないようにするため）。
 * ホストを localhost にしないのは、Windows で先に IPv6 の ::1 に解決されるため（プラン 4.5）。
 */
const E2E_DATABASE_CONFIG: pg.ClientConfig = {
  host: '127.0.0.1',
  port: 5432,
  user: 'zero',
  password: 'zero',
  database: E2E_DATABASE_NAME,
}

/** 利用者A（プラン 4.4「テストデータ」、db/seed/test.sql）。 */
export const USER_A_ID = '00000000-0000-0000-0000-00000000000a'

/**
 * 利用者B（db/seed/test.sql）。E2E の API は利用者A として動くので、Bの売買履歴は画面に出ない。
 * 「他の利用者だけが売買している銘柄」を作るために使う（銘柄ごとの売買履歴の画面の基準23）。
 */
export const USER_B_ID = '00000000-0000-0000-0000-00000000000b'

/** db/seed/test.sql が入れる銘柄マスタ（プラン 4.4「テストデータ」）。これ以外の銘柄マスタは resetHoldingsData で消す。 */
export const SEED_STOCK_CODES: readonly string[] = ['7203', '6758', '9984']

/** 銘柄の通貨（stocks.currency の値。プラン 4.2）。画面の Currency 型と同じ値だが、E2E は別のパッケージなのでここで持つ。 */
export type Currency = 'JPY' | 'USD'

/** 銘柄マスタに足す銘柄（test.sql にない銘柄。基準31 の100銘柄や、ドルの AAPL・MSFT など）。通貨は必須（プラン 4.7）。 */
export type StockSeed = { code: string; name: string; currency: Currency }

/**
 * 利用者A の売買履歴1件（プラン 4.6 の PR①）。価格（1株あたり）は浮動小数点を通さないよう文字列で渡す（例 "2500.0"）。
 * 取引日は "YYYY-MM-DD"。trades は配列の順に入れ、その順が登録した順（trades.id の昇順）になる。
 */
export type TradeSeed = { code: string; side: 'buy' | 'sell'; quantity: number; price: string; tradeDate: string }

/** 保存済みの終値1件。取引日は "YYYY-MM-DD"、終値は円が小数第1位まで・ドルが小数第2位までの文字列（例 "3000.0"、"180.00"）。 */
export type StockPriceSeed = { code: string; tradingDate: string; closePrice: string }

/**
 * 株式分割1件（株式分割のプラン 4.8 PR①）。分割は全利用者で共有する（利用者は持たない）。
 * 分割の日は "YYYY-MM-DD"、ratio は分割後の株数（2〜1,000。1→3 なら 3）。銘柄マスタにない銘柄も入れられる（外部キーなし）。
 */
export type SplitSeed = { code: string; splitDate: string; ratio: number }

/**
 * ドル円のレート1件（ドルを円にまとめる合計のプラン 4.7）。レートは全利用者で共有する（usd_jpy_rates）。
 * 日付は "YYYY-MM-DD"、レートは 1ドルあたりの円で、小数第2位までの文字列（例 "150.00"）。
 */
export type UsdJpyRateSeed = { date: string; rate: string }

/**
 * 価格の変動の通知の条件1件（価格の変動の通知のプラン 4.8 の PR②）。alert_conditions に入れる。
 * userId の既定は利用者A、createdAt の既定は '2026-11-01T00:00:00+09:00'（足した日を 2026-11-01 の日本時間の0時にする）。
 * kind・scope・direction は 4.2 の値（"cost"・"all"・"down" など）、stockCode は全銘柄なら省く（''）、threshold は 20（%）・100000（円）・150.25（ドル）のような値そのまま。
 */
export type AlertConditionSeed = {
  userId?: string
  kind: string
  scope: 'all' | 'stock'
  stockCode?: string
  direction: string
  threshold: string
  createdAt?: string
}

/**
 * 価格の変動の通知1件（判定を通さず直接入れる。要件定義の「通知（直接）」）。notifications に入れる。
 * userId の既定は利用者A。conditionId の既定は 0（条件は消えていてもよい。notifications.condition_id は外部キーにしない）。
 * 一意制約（condition_id, stock_code, trading_date）があるので、同じ銘柄・同じ取引日を2件以上入れるときは conditionId を変える。
 * conditionStockCode は全銘柄の条件なら省く（''）。threshold・closePrice は文字列の値（例 "20"・"1990"・"191.24"）。details は 4.2 の jsonb（例 { rateTenths: -204, basePriceHundredths: 250000 }）。
 * seen は見たかどうか。配列の順が id の順（古い順）になる。
 */
export type NotificationSeed = {
  userId?: string
  conditionId?: number
  stockCode: string
  tradingDate: string
  kind: string
  scope: 'all' | 'stock'
  conditionStockCode?: string
  direction: string
  threshold: string
  closePrice: string
  details: Record<string, unknown>
  seen: boolean
}

/** 条件の足した日の既定（日本時間の 2026-11-01 の0時）。 */
const DEFAULT_ALERT_CONDITION_CREATED_AT = '2026-11-01T00:00:00+09:00'

/** 1つのテストで入れるデータ。銘柄マスタ → 売買履歴 → 終値の順に入れる（FK のため）。 */
export type HoldingsTestData = {
  stocks?: StockSeed[]
  trades?: TradeSeed[]
  /** 利用者B の売買履歴（省略できる。銘柄ごとの売買履歴の画面の基準23。Aの画面には出ない）。配列の順に、Aの売買履歴の後に入れる。 */
  otherUserTrades?: TradeSeed[]
  prices?: StockPriceSeed[]
  /** 株式分割（省略できる。株式分割のプラン 4.8 PR①）。stock_splits に入れる。 */
  splits?: SplitSeed[]
  /**
   * 利用者A の保存済みの並び（銘柄コードを位置の順に。カード表示のプラン 4.6 PR②）。holding_orders に位置 0, 1, 2, … で入れる。
   * 銘柄マスタに銘柄があること（FK）。保有しているかは確かめない。なければ並びは保存しない（銘柄コードの昇順で表示）。
   */
  order?: string[]
  /** ドル円のレート（省略できる。ドルを円にまとめる合計のプラン 4.7）。usd_jpy_rates に入れる。日付ごとに1件。 */
  usdJpyRates?: UsdJpyRateSeed[]
  /** 価格の変動の通知の条件（省略できる。価格の変動の通知のプラン 4.8 の PR②）。alert_conditions に入れる。 */
  alertConditions?: AlertConditionSeed[]
  /** 価格の変動の通知（省略できる。同じ PR②）。notifications に、配列の順（古い順）で入れる。 */
  notifications?: NotificationSeed[]
}

/**
 * zero_e2e に接続する。接続した DB の名前が zero_e2e でなければ、何もせずに失敗する。
 */
async function connectToE2EDatabase(): Promise<pg.Client> {
  const client = new pg.Client(E2E_DATABASE_CONFIG)
  await client.connect()
  try {
    const result = await client.query<{ name: string }>(
      'SELECT current_database() AS name',
    )
    const name = result.rows[0]?.name
    if (name !== E2E_DATABASE_NAME) {
      throw new Error(
        `E2E の DB 操作は ${E2E_DATABASE_NAME} にだけ行います（接続先: ${String(name)}）`,
      )
    }
    return client
  } catch (error) {
    await client.end()
    throw error
  }
}

/**
 * zero_e2e に接続し、work を1つのトランザクションで実行する。失敗したらロールバックして、エラーを投げ直す。
 */
async function inE2ETransaction(
  work: (client: pg.Client) => Promise<void>,
): Promise<void> {
  const client = await connectToE2EDatabase()
  try {
    await client.query('BEGIN')
    await work(client)
    await client.query('COMMIT')
  } catch (error) {
    await client.query('ROLLBACK').catch(() => {
      // ロールバックの失敗より、元のエラーを優先して投げる。
    })
    throw error
  } finally {
    await client.end()
  }
}

/**
 * 売買履歴と終値と並びをすべて消し、test.sql にない銘柄マスタを消す（プラン 4.5 の beforeEach。並びはカード表示のプラン 4.6 PR②）。
 * 利用者と test.sql の銘柄マスタ（7203・6758・9984）は残す。
 * 並び（holding_orders）は銘柄マスタを参照するので、銘柄マスタを消す前に空にする。
 */
export async function resetHoldingsData(): Promise<void> {
  await inE2ETransaction(async (client) => {
    // レートは全利用者で共有なので、前のテストのレートを残さない（ドルを円にまとめる合計のプラン 4.6）。
    // 価格の変動の通知の3つの表は、銘柄マスタの行を消すときに外部キーで邪魔をしないよう、前のテストの分を空にする（価格の変動の通知のプラン 4.7）。
    await client.query(
      'TRUNCATE trades, stock_prices, holding_orders, stock_splits, usd_jpy_rates, alert_conditions, alert_judgments, notifications',
    )
    await client.query('DELETE FROM stocks WHERE code <> ALL($1::text[])', [
      SEED_STOCK_CODES,
    ])
  })
}

/**
 * そのテストのデータを入れる。売買履歴はすべて利用者A のものとして、配列の順に入れる（その順が登録した順になる）。
 */
export async function insertHoldingsData(
  data: HoldingsTestData,
): Promise<void> {
  const stocks = data.stocks ?? []
  const trades = data.trades ?? []
  const otherUserTrades = data.otherUserTrades ?? []
  const prices = data.prices ?? []
  const splits = data.splits ?? []
  const order = data.order ?? []
  const usdJpyRates = data.usdJpyRates ?? []
  const alertConditions = data.alertConditions ?? []
  const notifications = data.notifications ?? []

  await inE2ETransaction(async (client) => {
    if (stocks.length > 0) {
      await client.query(
        `INSERT INTO stocks (code, name, currency)
         SELECT * FROM unnest($1::text[], $2::text[], $3::text[])`,
        [stocks.map((s) => s.code), stocks.map((s) => s.name), stocks.map((s) => s.currency)],
      )
    }
    if (trades.length > 0) {
      // unnest の WITH ORDINALITY の順（ORDER BY ord）で入れ、配列の順が trades.id の順になるようにする（プラン 4.7）。
      await client.query(
        `INSERT INTO trades (user_id, stock_code, side, quantity, price, trade_date)
         SELECT $1::uuid, t.code, t.side, t.quantity, t.price, t.trade_date
         FROM unnest($2::text[], $3::text[], $4::bigint[], $5::numeric[], $6::date[])
              WITH ORDINALITY AS t(code, side, quantity, price, trade_date, ord)
         ORDER BY t.ord`,
        [
          USER_A_ID,
          trades.map((t) => t.code),
          trades.map((t) => t.side),
          trades.map((t) => t.quantity),
          trades.map((t) => t.price),
          trades.map((t) => t.tradeDate),
        ],
      )
    }
    if (otherUserTrades.length > 0) {
      await client.query(
        `INSERT INTO trades (user_id, stock_code, side, quantity, price, trade_date)
         SELECT $1::uuid, t.code, t.side, t.quantity, t.price, t.trade_date
         FROM unnest($2::text[], $3::text[], $4::bigint[], $5::numeric[], $6::date[])
              WITH ORDINALITY AS t(code, side, quantity, price, trade_date, ord)
         ORDER BY t.ord`,
        [
          USER_B_ID,
          otherUserTrades.map((t) => t.code),
          otherUserTrades.map((t) => t.side),
          otherUserTrades.map((t) => t.quantity),
          otherUserTrades.map((t) => t.price),
          otherUserTrades.map((t) => t.tradeDate),
        ],
      )
    }
    if (prices.length > 0) {
      await client.query(
        `INSERT INTO stock_prices (stock_code, trading_date, close_price)
         SELECT * FROM unnest($1::text[], $2::date[], $3::numeric[])`,
        [
          prices.map((p) => p.code),
          prices.map((p) => p.tradingDate),
          prices.map((p) => p.closePrice),
        ],
      )
    }
    if (splits.length > 0) {
      await client.query(
        `INSERT INTO stock_splits (stock_code, split_date, ratio)
         SELECT * FROM unnest($1::text[], $2::date[], $3::integer[])`,
        [
          splits.map((s) => s.code),
          splits.map((s) => s.splitDate),
          splits.map((s) => s.ratio),
        ],
      )
    }
    if (order.length > 0) {
      // 配列の順が position（0, 1, 2, …）になる。
      await client.query(
        `INSERT INTO holding_orders (user_id, stock_code, position)
         SELECT $1::uuid, o.code, (o.ord - 1)::int
         FROM unnest($2::text[]) WITH ORDINALITY AS o(code, ord)`,
        [USER_A_ID, order],
      )
    }
    if (usdJpyRates.length > 0) {
      await client.query(
        `INSERT INTO usd_jpy_rates (rate_date, rate)
         SELECT * FROM unnest($1::date[], $2::numeric[])`,
        [usdJpyRates.map((r) => r.date), usdJpyRates.map((r) => r.rate)],
      )
    }
    if (alertConditions.length > 0) {
      // 配列の順が id の順（足した順）になるよう、unnest の WITH ORDINALITY の順で入れる。
      await client.query(
        `INSERT INTO alert_conditions (user_id, kind, scope, stock_code, direction, threshold, created_at)
         SELECT c.user_id::uuid, c.kind, c.scope, c.stock_code, c.direction, c.threshold::numeric, c.created_at::timestamptz
         FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::text[], $6::text[], $7::text[])
              WITH ORDINALITY AS c(user_id, kind, scope, stock_code, direction, threshold, created_at, ord)
         ORDER BY c.ord`,
        [
          alertConditions.map((c) => c.userId ?? USER_A_ID),
          alertConditions.map((c) => c.kind),
          alertConditions.map((c) => c.scope),
          alertConditions.map((c) => c.stockCode ?? ''),
          alertConditions.map((c) => c.direction),
          alertConditions.map((c) => c.threshold),
          alertConditions.map((c) => c.createdAt ?? DEFAULT_ALERT_CONDITION_CREATED_AT),
        ],
      )
    }
    if (notifications.length > 0) {
      // 配列の順が id の順になるよう、unnest の WITH ORDINALITY の順で入れる。details は JSON の文字列にして jsonb に変える。
      await client.query(
        `INSERT INTO notifications (user_id, condition_id, kind, scope, condition_stock_code, direction, threshold,
                                    stock_code, trading_date, close_price, details, seen)
         SELECT n.user_id::uuid, n.condition_id, n.kind, n.scope, n.condition_stock_code, n.direction, n.threshold::numeric,
                n.stock_code, n.trading_date::date, n.close_price::numeric, n.details::jsonb, n.seen
         FROM unnest($1::text[], $2::bigint[], $3::text[], $4::text[], $5::text[], $6::text[], $7::text[],
                     $8::text[], $9::text[], $10::text[], $11::text[], $12::boolean[])
              WITH ORDINALITY AS n(user_id, condition_id, kind, scope, condition_stock_code, direction, threshold,
                                   stock_code, trading_date, close_price, details, seen, ord)
         ORDER BY n.ord`,
        [
          notifications.map((n) => n.userId ?? USER_A_ID),
          notifications.map((n) => n.conditionId ?? 0),
          notifications.map((n) => n.kind),
          notifications.map((n) => n.scope),
          notifications.map((n) => n.conditionStockCode ?? ''),
          notifications.map((n) => n.direction),
          notifications.map((n) => n.threshold),
          notifications.map((n) => n.stockCode),
          notifications.map((n) => n.tradingDate),
          notifications.map((n) => n.closePrice),
          notifications.map((n) => JSON.stringify(n.details)),
          notifications.map((n) => n.seen),
        ],
      )
    }
  })
}
