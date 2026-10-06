import { expect, type Page } from '@playwright/test'

export const ADMIN = {
  email: process.env.ADMIN_EMAIL ?? 'admin@example.com',
  password: process.env.ADMIN_PASSWORD ?? 'dev-only-admin-password',
}

export async function login(page: Page, who: { email: string; password: string } = ADMIN) {
  await page.goto('/')
  await page.getByLabel('E-mail').fill(who.email)
  await page.getByLabel('Senha').fill(who.password)
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page.getByRole('heading', { name: 'Painel de resultados' })).toBeVisible()
}

/** `YYYY-MM-DD` of today + n days in the business zone (the browser under test uses the same zone). */
export function day(offset = 0): string {
  const d = new Date(Date.now() + offset * 86_400_000)
  return new Intl.DateTimeFormat('sv-SE', { timeZone: 'America/Sao_Paulo' }).format(d)
}
