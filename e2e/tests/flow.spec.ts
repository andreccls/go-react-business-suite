import { expect, test } from '@playwright/test'
import { ADMIN, day, login } from './helpers'

const uid = Date.now().toString(36)

test('admin: create a service and a customer, book, hit a conflict, see the dashboard move, cancel', async ({ page }) => {
  await login(page)

  // The dashboard period reaches a week ahead so a booking for tomorrow is counted.
  await page.getByLabel('Até').fill(day(7))
  const kpis = page.getByRole('region', { name: 'Indicadores do período' })
  const total = async () => Number(await kpis.getByText('Agendamentos', { exact: true }).locator('xpath=following-sibling::div[1]').innerText())
  await expect(kpis).toBeVisible()
  const before = await total()

  // service
  await page.getByRole('link', { name: 'Serviços' }).click()
  await page.getByRole('button', { name: 'Novo serviço' }).click()
  const service = `Serviço E2E ${uid}`
  const form = page.getByRole('dialog')
  await form.getByLabel('Nome').fill(service)
  await form.getByLabel('Duração (minutos)').fill('40')
  await form.getByLabel('Preço (R$)').fill('77,70')
  await form.getByRole('button', { name: 'Salvar' }).click()
  await expect(page.getByRole('status')).toContainText(`Serviço "${service}" criado.`)
  await expect(page.getByRole('row', { name: new RegExp(service) })).toContainText(/R\$\s77,70/)

  // customers: two, to provoke the conflict with the second
  const names = [`Cliente A ${uid}`, `Cliente B ${uid}`]
  await page.getByRole('link', { name: 'Clientes' }).click()
  for (const [i, name] of names.entries()) {
    await page.getByRole('button', { name: 'Novo cliente' }).click()
    const form = page.getByRole('dialog')
    await form.getByLabel('Nome').fill(name)
    await form.getByLabel('E-mail').fill(`e2e-${i}-${uid}@example.com`)
    await form.getByRole('button', { name: 'Salvar' }).click()
    await expect(page.getByRole('status')).toContainText(`Cliente "${name}" cadastrado.`)
  }

  // booking for tomorrow
  const hour = String(6 + (Date.now() % 14)).padStart(2, '0')
  const book = async (customer: string) => {
    await page.getByRole('button', { name: 'Novo agendamento' }).click()
    const dialog = page.getByRole('dialog', { name: 'Novo agendamento' })
    await dialog.getByLabel('Cliente').selectOption({ label: customer })
    await dialog.getByLabel('Serviço').selectOption({ label: service })
    await dialog.getByLabel('Data').fill(day(1))
    await dialog.getByLabel('Horário').fill(`${hour}:00`)
    await dialog.getByRole('button', { name: 'Agendar' }).click()
    return dialog
  }
  await page.getByRole('link', { name: 'Agenda' }).click()
  await book(names[0]!)
  await expect(page.getByRole('status')).toContainText(`Agendamento de ${names[0]} (${service}) criado.`)
  const row = page.getByRole('row', { name: new RegExp(names[0]!) })
  await expect(row).toContainText('Agendado')
  await expect(row.getByRole('button', { name: /Concluir/ })).toBeDisabled() // not started yet

  // the same slot for someone else: 409, clear message, dialog stays open
  const dialog = await book(names[1]!)
  await expect(dialog.getByRole('alert')).toContainText('Esse horário conflita com outro agendamento')
  await dialog.getByRole('button', { name: 'Cancelar' }).click()
  await expect(dialog).toBeHidden()

  // the dashboard reflects the new appointment
  await page.getByRole('link', { name: 'Painel' }).click()
  await expect(page.getByRole('heading', { name: 'Painel de resultados' })).toBeVisible() // 'Até' also exists on the Agenda page
  await page.getByLabel('Até').fill(day(7))
  await expect.poll(total).toBe(before + 1)

  // cancel it (with confirmation)
  await page.getByRole('link', { name: 'Agenda' }).click()
  await page.getByRole('row', { name: new RegExp(names[0]!) }).getByRole('button', { name: /Cancelar/ }).click()
  await page.getByRole('dialog').getByRole('button', { name: 'Cancelar agendamento' }).click()
  await expect(page.getByRole('row', { name: new RegExp(names[0]!) })).toContainText('Cancelado')

  // the service is referenced by that (cancelled) appointment: deleting is refused with a clear message
  await page.getByRole('link', { name: 'Serviços' }).click()
  await page.getByRole('button', { name: `Excluir ${service}` }).click()
  await page.getByRole('dialog').getByRole('button', { name: 'Excluir' }).click()
  await expect(page.getByRole('dialog').getByRole('alert')).toContainText('Desative-o em vez disso')
})

test('staff cannot delete or manage users; the session survives a reload', async ({ page }) => {
  await login(page)
  const email = `staff-${uid}@example.com`
  await page.getByRole('link', { name: 'Usuários' }).click()
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha inicial').fill('staff-e2e-password')
  await page.getByRole('button', { name: 'Criar usuário' }).click()
  await expect(page.getByText(email)).toBeVisible()
  await page.getByRole('button', { name: 'Sair' }).click()
  await expect(page.getByRole('button', { name: 'Entrar' })).toBeVisible()

  await login(page, { email, password: 'staff-e2e-password' })
  await expect(page.getByRole('link', { name: 'Usuários' })).toHaveCount(0)
  await page.getByRole('link', { name: 'Serviços' }).click()
  await expect(page.getByRole('button', { name: /Editar/ }).first()).toBeVisible()
  await expect(page.getByRole('button', { name: /Excluir/ })).toHaveCount(0)

  await page.reload() // access token is gone, the refresh token in sessionStorage brings the session back
  await expect(page.getByRole('heading', { name: 'Serviços', exact: true })).toBeVisible()
  await page.goto('/usuarios')
  await expect(page.getByRole('heading', { name: 'Acesso restrito' })).toBeVisible()
})

test('wrong password is explained, and the admin from .env.example is the only way in', async ({ page }) => {
  await page.goto('/agenda')
  await expect(page).toHaveURL(/\/login$/)
  await page.getByLabel('E-mail').fill(ADMIN.email)
  await page.getByLabel('Senha').fill('wrong-password')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page.getByRole('alert')).toContainText('E-mail ou senha incorretos.')
})

test('keyboard only: skip link, dialog focus handling, Escape; no console errors', async ({ page }) => {
  const errors: string[] = []
  page.on('pageerror', (e) => errors.push(e.message))
  page.on('console', (m) => {
    // 4xx are expected API answers; COOP is only ignored because the test reaches nginx as http://frontend (not localhost/HTTPS)
    if (m.type() === 'error' && !/status of 4\d\d|Cross-Origin-Opener-Policy/.test(m.text())) errors.push(m.text())
  })

  await page.goto('/')
  await page.getByLabel('E-mail').fill(ADMIN.email)
  await page.getByLabel('Senha').fill(ADMIN.password)
  await page.getByLabel('Senha').press('Enter')
  await expect(page.getByRole('heading', { name: 'Painel de resultados' })).toBeVisible()

  await page.keyboard.press('Tab')
  await expect(page.getByRole('link', { name: 'Ir para o conteúdo' })).toBeFocused()

  await page.getByRole('link', { name: 'Serviços' }).focus()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('heading', { name: 'Serviços', exact: true })).toBeVisible()

  const opener = page.getByRole('button', { name: 'Novo serviço' })
  await opener.focus()
  await page.keyboard.press('Enter')
  const dialog = page.getByRole('dialog', { name: 'Novo serviço' })
  await expect(dialog.getByLabel('Nome')).toBeFocused()
  for (let i = 0; i < 8; i++) {
    await page.keyboard.press('Shift+Tab') // focus must wrap inside the dialog, never reach the page behind
    expect(await page.evaluate(() => document.activeElement?.closest('[role=dialog]') !== null)).toBe(true)
  }
  await page.keyboard.press('Escape')
  await expect(dialog).toBeHidden()
  await expect(opener).toBeFocused()
  expect(errors).toEqual([])
})

test('phone: no page needs horizontal scrolling', async ({ browser }) => {
  const context = await browser.newContext({ viewport: { width: 360, height: 780 } })
  const page = await context.newPage()
  await login(page)
  for (const [link, heading] of [['Painel', 'Painel de resultados'], ['Agenda', 'Agenda'], ['Serviços', 'Serviços'], ['Clientes', 'Clientes'], ['Usuários', 'Usuários']] as const) {
    await page.getByRole('link', { name: link, exact: true }).click()
    await expect(page.getByRole('heading', { name: heading, exact: true })).toBeVisible()
    // wait until every loading placeholder (role=status) is gone; empty states have no role, so do not require data
    await expect(page.getByRole('status')).toHaveCount(0)
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
    expect(overflow, `${link}: page is wider than the viewport`).toBeLessThanOrEqual(0)
  }
  await context.close()
})
