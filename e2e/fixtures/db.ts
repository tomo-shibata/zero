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

/** 1つのテストで入れるデータ。銘柄マスタ → 売買履歴 → 終値の順に入れる（FK のため）。 */
export type HoldingsTestData = {
  stocks?: StockSeed[]
  trades?: TradeSeed[]
  prices?: StockPriceSeed[]
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
 * 売買履歴と終値をすべて消し、test.sql にない銘柄マスタを消す（プラン 4.5 の beforeEach）。
 * 利用者と test.sql の銘柄マスタ（7203・6758・9984）は残す。
 */
export async function resetHoldingsData(): Promise<void> {
  await inE2ETransaction(async (client) => {
    await client.query('TRUNCATE trades, stock_prices')
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
  const prices = data.prices ?? []

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
  })
}
