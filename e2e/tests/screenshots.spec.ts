// Not a test of behaviour: regenerates docs/images/*.png from the running stack (./e2e/run.sh screenshots).
import { expect, test, type Browser, type Page } from '@playwright/test'
import { day, login } from './helpers'

test.skip(!process.env.SCREENSHOTS, 'only with SCREENSHOTS=1')

const OUT = '/shots'
const desktop = { width: 1280, height: 800 }
const phone = { width: 390, height: 844 }

async function session(browser: Browser, viewport: typeof desktop, colorScheme: 'light' | 'dark') {
  const context = await browser.newContext({ viewport, colorScheme, locale: 'pt-BR', timezoneId: 'America/Sao_Paulo' })
  const page = await context.newPage()
  await login(page)
  return page
}

async function openBooking(page: Page, slot: { date: string; time: string }) {
  await page.getByRole('button', { name: 'Novo agendamento' }).click()
  const dialog = page.getByRole('dialog', { name: 'Novo agendamento' })
  await expect(dialog.getByLabel('Cliente').locator('option')).not.toHaveCount(1)
  await dialog.getByLabel('Cliente').selectOption({ index: 1 })
  await dialog.getByLabel('Serviço').selectOption({ index: 1 })
  await dialog.getByLabel('Data').fill(slot.date)
  await dialog.getByLabel('Horário').fill(slot.time)
  return dialog
}

test('dashboard, light and dark', async ({ browser }) => {
  for (const scheme of ['light', 'dark'] as const) {
    const page = await session(browser, desktop, scheme)
    await expect(page.getByRole('region', { name: 'Indicadores do período' })).toBeVisible()
    await expect(page.getByRole('img', { name: /Receita realizada por dia/ })).toBeVisible()
    await expect(page.getByRole('region', { name: 'Top serviços' }).getByRole('row')).not.toHaveCount(1)
    await page.screenshot({ path: `${OUT}/${scheme === 'light' ? '01-painel-claro' : '02-painel-escuro'}.png` })
    await page.context().close()
  }
})

test('agenda: the overlap conflict (409) in the booking dialog', async ({ browser }) => {
  const page = await session(browser, desktop, 'light')
  await page.getByRole('link', { name: 'Agenda' }).click()
  const slot = { date: day(3), time: `${String(7 + (Date.now() % 12)).padStart(2, '0')}:00` }
  let dialog = await openBooking(page, slot)
  await dialog.getByRole('button', { name: 'Agendar' }).click()
  // First run: the booking succeeds and we book the same slot again; a re-run already has it taken.
  await expect(page.getByRole('status').or(dialog.getByRole('alert'))).toBeVisible()
  if (await page.getByRole('status').isVisible()) {
    dialog = await openBooking(page, slot)
    await dialog.getByRole('button', { name: 'Agendar' }).click()
  }
  await expect(dialog.getByRole('alert')).toContainText('conflita com outro agendamento')
  await page.screenshot({ path: `${OUT}/03-agenda-conflito.png` })
  await page.context().close()
})

test('services, dark', async ({ browser }) => {
  const page = await session(browser, desktop, 'dark')
  await page.getByRole('link', { name: 'Serviços' }).click()
  await expect(page.getByRole('heading', { name: 'Serviços', exact: true })).toBeVisible()
  await expect(page.getByRole('row')).not.toHaveCount(1)
  await page.screenshot({ path: `${OUT}/04-servicos-escuro.png` })
  await page.context().close()
})

test('phone: agenda cards (light) and booking dialog (dark)', async ({ browser }) => {
  let page = await session(browser, phone, 'light')
  await page.getByRole('link', { name: 'Agenda' }).click()
  await expect(page.getByRole('region', { name: 'Lista de agendamentos' })).toContainText('Agendado')
  await page.screenshot({ path: `${OUT}/05-agenda-celular.png` })
  await page.context().close()

  page = await session(browser, phone, 'dark')
  await page.getByRole('link', { name: 'Agenda' }).click()
  await openBooking(page, { date: day(4), time: '10:00' })
  await page.screenshot({ path: `${OUT}/06-novo-agendamento-celular-escuro.png` })
  await page.context().close()
})
