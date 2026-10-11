import pg from 'pg'

// E2E から DB（zero_e2e）のデータを入れ替えるヘルパー。
// プラン docs/plans/stock-holdings/plan.md 4.5「E2E」:
//   - zero_e2e は、API の webServer（task e2e:api）が起動時に cmd/migrate -recreate -seed db/seed/test.sql で作り直す。
//   - E2E から DB を操作する pg の接続先は、zero_e2e に固定した定数にする。DB 名が zero_e2e でなければ失敗させる。
//   - 各テストの beforeEach で TRUNCATE holdings, stock_prices と、test.sql にない銘柄マスタの削除を行ってから、
//     そのテストのデータを入れる。
// 保有データは利用者A のものだけを入れる（E2E の API は DEV_USER_ID=利用者A で起動する。プラン 4.5）。

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

/** 銘柄マスタに足す銘柄（test.sql にない銘柄。AC-18 の100銘柄など）。 */
export type StockSeed = { code: string; name: string }

/** 利用者A の保有データ1件。取得価格は浮動小数点を通さないよう、小数第1位までの文字列で渡す（例 "2500.0"。プラン 4.1）。 */
export type HoldingSeed = { code: string; quantity: number; acquisitionPrice: string }

/** 保存済みの終値1件。取引日は "YYYY-MM-DD"、終値は小数第1位までの文字列（例 "3000.0"）。 */
export type StockPriceSeed = { code: string; tradingDate: string; closePrice: string }

/** 1つのテストで入れるデータ。銘柄マスタ → 保有データ → 終値の順に入れる（FK のため）。 */
export type HoldingsTestData = {
  stocks?: StockSeed[]
  holdings?: HoldingSeed[]
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
 * 保有データと終値をすべて消し、test.sql にない銘柄マスタを消す（プラン 4.5 の beforeEach）。
 * 利用者と test.sql の銘柄マスタ（7203・6758・9984）は残す。
 */
export async function resetHoldingsData(): Promise<void> {
  await inE2ETransaction(async (client) => {
    await client.query('TRUNCATE holdings, stock_prices')
    await client.query('DELETE FROM stocks WHERE code <> ALL($1::text[])', [
      SEED_STOCK_CODES,
    ])
  })
}

/**
 * そのテストのデータを入れる。保有データはすべて利用者A のものとして入れる。
 */
export async function insertHoldingsData(
  data: HoldingsTestData,
): Promise<void> {
  const stocks = data.stocks ?? []
  const holdings = data.holdings ?? []
  const prices = data.prices ?? []

  await inE2ETransaction(async (client) => {
    if (stocks.length > 0) {
      await client.query(
        `INSERT INTO stocks (code, name)
         SELECT * FROM unnest($1::text[], $2::text[])`,
        [stocks.map((s) => s.code), stocks.map((s) => s.name)],
      )
    }
    if (holdings.length > 0) {
      await client.query(
        `INSERT INTO holdings (user_id, stock_code, quantity, acquisition_price)
         SELECT $1::uuid, code, quantity, price
         FROM unnest($2::text[], $3::bigint[], $4::numeric[]) AS h(code, quantity, price)`,
        [
          USER_A_ID,
          holdings.map((h) => h.code),
          holdings.map((h) => h.quantity),
          holdings.map((h) => h.acquisitionPrice),
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
