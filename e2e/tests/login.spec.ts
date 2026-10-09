import { expect, test } from '@playwright/test'

// 仕様: ログイン画面が表示され、ログインボタンを押すとダッシュボードへ遷移する。
// （認証処理は未実装のデモ）

test('ログイン画面が表示される', async ({ page }) => {
  await page.goto('/login')

  await expect(page.getByRole('heading', { name: 'ログイン' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'ログイン' })).toBeVisible()
})

test('ログインボタンを押すとダッシュボードへ遷移する', async ({ page }) => {
  await page.goto('/login')

  // 必須項目を入力してから送信する（認証処理自体は未実装）
  await page.getByLabel('メールアドレス').fill('you@example.com')
  await page.getByLabel('パスワード').fill('password')
  await page.getByRole('button', { name: 'ログイン' }).click()

  await expect(page).toHaveURL(/\/dashboard$/)
  await expect(
    page.getByRole('heading', { name: 'ダッシュボード' }),
  ).toBeVisible()
})

test('メールアドレスとパスワードが空のままでは遷移しない', async ({ page }) => {
  await page.goto('/login')

  await page.getByRole('button', { name: 'ログイン' }).click()

  // ブラウザの入力チェックで送信が止まり、ログイン画面に留まる
  const email = page.getByLabel('メールアドレス')
  expect(
    await email.evaluate((el) => (el as HTMLInputElement).validity.valueMissing),
  ).toBe(true)
  await expect(page).toHaveURL(/\/login$/)
  await expect(page.getByRole('heading', { name: 'ダッシュボード' })).toHaveCount(0)
})

test('メールアドレスの形式が正しくなければ遷移しない', async ({ page }) => {
  await page.goto('/login')

  const email = page.getByLabel('メールアドレス')
  await email.fill('abc')
  await page.getByLabel('パスワード').fill('password')
  await page.getByRole('button', { name: 'ログイン' }).click()

  expect(
    await email.evaluate((el) => (el as HTMLInputElement).validity.typeMismatch),
  ).toBe(true)
  await expect(page).toHaveURL(/\/login$/)
  await expect(page.getByRole('heading', { name: 'ダッシュボード' })).toHaveCount(0)
})

test('ダッシュボードでログアウトを押すとログイン画面へ戻る', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('メールアドレス').fill('you@example.com')
  await page.getByLabel('パスワード').fill('password')
  await page.getByRole('button', { name: 'ログイン' }).click()
  await expect(page).toHaveURL(/\/dashboard$/)

  await page.getByRole('button', { name: 'ログアウト' }).click()

  await expect(page).toHaveURL(/\/login$/)
  await expect(page.getByRole('heading', { name: 'ログイン' })).toBeVisible()
})

test('未知のパスはログイン画面にリダイレクトされる', async ({ page }) => {
  await page.goto('/')

  await expect(page).toHaveURL(/\/login$/)
})
