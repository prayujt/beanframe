import { expect, test } from '@playwright/test';
const rpc = '/beancount.v1.LedgerService/';
const signedOut = {
  authenticated: false,
  authEnabled: true,
  provider: 'Company SSO'
};

/** @param {import('@playwright/test').Page} page @param {string} label @param {string} iso */
async function chooseDate(page, label, iso) {
  await page.getByRole('button', { name: label, exact: true }).click();
  await page
    .getByLabel('Year', { exact: true })
    .selectOption(String(Number(iso.slice(0, 4))));
  await page
    .getByLabel('Month', { exact: true })
    .selectOption(String(Number(iso.slice(5, 7))));
  const dayLabel = new Date(iso + 'T12:00:00Z').toLocaleDateString('en-US', {
    weekday: 'long',
    month: 'long',
    day: 'numeric',
    year: 'numeric',
    timeZone: 'UTC'
  });
  await page.getByRole('button', { name: dayLabel, exact: true }).click();
}

test('signed-out UI uses Connect session and native login', async ({
  page
}) => {
  await page.route('**' + rpc + 'GetSession', (r) =>
    r.fulfill({ json: signedOut })
  );
  await page.goto('/');
  await expect(
    page.getByRole('heading', { name: 'Beanframe', exact: true })
  ).toBeVisible();
  await expect(
    page.getByRole('link', { name: 'Continue with Company SSO', exact: true })
  ).toHaveAttribute('href', '/auth/login');
  await expect(
    page.getByRole('navigation', { name: 'Main navigation' })
  ).toHaveCount(0);
});
test('unconfigured provider uses a neutral label and company branding is optional', async ({
  page
}) => {
  await page.route('**' + rpc + 'GetSession', (route) =>
    route.fulfill({ json: { ...signedOut, provider: '' } })
  );
  await page.goto('/');
  await expect(
    page.getByRole('link', {
      name: 'Continue with OpenID Connect',
      exact: true
    })
  ).toBeVisible();
  await expect(page.locator('.login-note')).toHaveCount(0);
});
test('sign-in branding loads external images and handles an unavailable logo', async ({
  page
}) => {
  await page.route('**' + rpc + 'GetSession', (r) =>
    r.fulfill({
      json: {
        ...signedOut,
        brandName: 'Example Finance',
        companyName: 'Example Company',
        brandLogoUrl: 'https://example.com/logo.png'
      }
    })
  );
  await page.route('https://example.com/logo.png', (r) =>
    r.fulfill({
      contentType: 'image/svg+xml',
      body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32" fill="#258fe6"/></svg>'
    })
  );
  await page.goto('/');
  await expect(page).toHaveTitle('Example Finance');
  await expect(page.locator('.login-note')).toHaveText('Example Company');
  await expect(
    page.getByRole('heading', { name: 'Example Finance' })
  ).toBeVisible();
  await expect(page.locator('.login-card img')).toBeVisible();
  await expect
    .poll(() =>
      page
        .locator('.login-card img')
        .evaluate((img) =>
          img instanceof HTMLImageElement ? img.naturalWidth : 0
        )
    )
    .toBe(32);
  await expect(page.locator('.login-card')).not.toContainText('Beancount');
  await page.route('https://example.com/logo.png', (r) => r.abort());
  await page.reload();
  await expect(page.locator('.login-card .brand-mark svg')).toBeVisible();
  await expect(
    page.getByRole('link', { name: 'Continue with Company SSO', exact: true })
  ).toBeVisible();
});
test('denied sign-in and session failures fail closed', async ({ page }) => {
  await page.route('**' + rpc + 'GetSession', (r) =>
    r.fulfill({ json: signedOut })
  );
  await page.goto('/?auth_error=access_denied');
  await expect(page.getByRole('status')).toHaveText(
    'Your account does not have access to this ledger.'
  );
  await expect(page).toHaveURL('/');
  await page.unroute('**' + rpc + 'GetSession');
  await page.route('**' + rpc + 'GetSession', (r) =>
    r.fulfill({
      status: 503,
      json: { code: 'unavailable', message: 'Offline' }
    })
  );
  await page.reload();
  await expect(page.getByRole('status')).toContainText('Offline');
  await expect(page.getByRole('button', { name: 'Try again' })).toBeVisible();
});
test('session polling keeps renewal status current and notices provider logout', async ({
  page
}) => {
  await page.clock.install();
  let authenticated = true;
  let checks = 0;
  await page.route('**' + rpc + 'GetSession', (r) => {
    checks++;
    return r.fulfill({
      json: {
        ...signedOut,
        authenticated,
        renewable: true,
        user: {
          name: 'Test user',
          groups: ['finance-users', 'finance-team']
        }
      }
    });
  });
  await page.goto('/');
  await page
    .getByRole('button', { name: 'Workspace settings', exact: true })
    .click();
  await expect(page.getByRole('list', { name: 'Your groups' })).toContainText(
    'finance-users'
  );
  await expect(page.getByRole('list', { name: 'Your groups' })).toContainText(
    'finance-team'
  );
  await expect(page.getByRole('dialog')).not.toContainText(
    'Renews automatically'
  );
  await expect(page.getByRole('dialog')).not.toContainText('Live connection');
  await page.clock.fastForward(60_000);
  await expect.poll(() => checks).toBeGreaterThan(1);
  authenticated = false;
  await page.clock.fastForward(60_000);
  await expect(
    page.getByRole('link', { name: 'Continue with Company SSO', exact: true })
  ).toBeVisible();
  await expect(
    page.getByRole('navigation', { name: 'Main navigation' })
  ).toHaveCount(0);
});
test('overview, reports, currency separation and deep links', async ({
  page
}) => {
  /** @type {string[]} */
  const failures = [];
  page.on('pageerror', (e) => failures.push(e.message));
  await page.goto('/');
  await expect(
    page.getByRole('heading', { name: 'Overview', exact: true })
  ).toBeVisible();
  await expect(page.getByText('$17.66', { exact: true }).first()).toBeVisible();
  await expect(page.locator('.connection')).toContainText('Live');
  await expect(page.locator('canvas')).toHaveCount(3);
  await expect(page.getByLabel('Report currency')).toHaveCount(0);
  await expect(
    page.getByRole('img', { name: 'Expense composition by account category' })
  ).toBeVisible();
  await page.goto('/income');
  await expect(page.locator('.statement-result')).toContainText('$17.66');
  await page.goto('/balance');
  await expect(page.locator('.statement-result')).toContainText('$17.66');
  await page.goto('/checks');
  await expect(
    page.getByRole('heading', { name: 'Your ledger is balanced' })
  ).toBeVisible();
  expect(failures).toEqual([]);
});
test('business overview rolls up accounts and drills into parent groups', async ({
  page
}) => {
  await page.goto('/');
  await expect(page.locator('.metrics')).toContainText('Net assets');
  await expect(page.locator('.metrics')).toContainText('Revenue');
  await expect(
    page.locator('.metric').filter({ hasText: /^Assets/ })
  ).toContainText('$117.66');
  await expect(
    page.locator('.metric').filter({ hasText: /^Liabilities/ })
  ).toContainText('$100.00');
  await expect(
    page.getByRole('region', { name: 'Assets composition', exact: true })
  ).toContainText('$117.66');
  await expect(
    page.getByRole('region', { name: 'Liabilities composition', exact: true })
  ).toContainText('$100.00');
  await expect(page.locator('.metrics')).not.toContainText('Net worth');
  const tree = page.getByRole('region', { name: 'Account breakdown' });
  const assets = tree
    .locator('tr')
    .filter({ has: page.getByRole('link', { name: 'Assets', exact: true }) });
  await expect(assets).toContainText('$117.66');
  const revenue = tree
    .locator('tr')
    .filter({ has: page.getByRole('link', { name: 'Revenue', exact: true }) });
  await expect(revenue).toContainText('$30.00');
  await tree
    .getByRole('button', { name: 'Collapse Assets', exact: true })
    .click();
  await expect(
    tree.getByRole('link', { name: 'Checking', exact: true })
  ).toHaveCount(0);
  await tree
    .getByRole('button', { name: 'Expand Assets', exact: true })
    .click();
  await expect(
    tree.getByRole('link', { name: 'Checking', exact: true })
  ).toBeVisible();
  await tree.getByLabel('Search account breakdown').fill('hosting');
  await expect(
    tree.getByRole('link', { name: 'Hosting', exact: true })
  ).toBeVisible();
  await expect(
    tree.getByRole('link', { name: 'Checking', exact: true })
  ).toHaveCount(0);
  await tree.getByLabel('Search account breakdown').fill('');
  await tree.getByRole('link', { name: 'Assets', exact: true }).click();
  await expect(page.getByLabel('Filter by account')).toHaveValue('Assets');
  await expect(page.locator('tbody tr')).toHaveCount(4);
});
test('command menu focuses immediately and navigates accounts with the keyboard', async ({
  page
}) => {
  await page.goto('/');
  await expect(page.locator('.connection')).toContainText('Live');
  await page.keyboard.press('Meta+k');
  const search = page.getByLabel('Find a page or account');
  await expect(search).toBeFocused();
  await search.fill('expenses hosting');
  await expect(
    page.getByRole('option', { name: 'Hosting Expenses:Hosting', exact: true })
  ).toBeVisible();
  await search.press('Enter');
  await expect(page).toHaveURL('/journal?account=Expenses%3AHosting');
  await expect(page.getByLabel('Filter by account')).toHaveValue(
    'Expenses:Hosting'
  );
  await page
    .getByRole('button', { name: 'Find an account', exact: true })
    .click();
  await expect(search).toBeFocused();
  await search.fill('liabilities');
  await search.press('ArrowDown');
  await search.press('Enter');
  await expect(page.getByLabel('Filter by account')).toHaveValue(
    'Liabilities:Founder'
  );
  await page.keyboard.press('Control+k');
  await expect(search).toBeFocused();
  await search.fill('European hosting');
  await expect(
    page.getByRole('option').filter({ hasText: 'European hosting' })
  ).toBeVisible();
  await search.press('Enter');
  await expect(
    page.getByRole('dialog', { name: 'Search workspace', exact: true })
  ).toHaveCount(0);
  await expect(
    page.getByRole('dialog', { name: 'Cloud', exact: true })
  ).toContainText('European hosting');
});
test('transaction form shows exact source alongside searchable posting accounts', async ({
  page
}) => {
  await page.goto('/journal');
  await page.getByRole('button', { name: 'New transaction' }).click();
  await page
    .getByLabel('Description', { exact: true })
    .fill('Business software');
  const account = page.getByLabel('Posting 1 account');
  await account.fill('expenses hosting');
  await account.press('Enter');
  await expect(account).toHaveValue('Expenses:Hosting');
  await page.getByLabel('Posting 1 amount').fill('14.25');
  await page.getByLabel('Posting 2 amount').fill('-14.25');
  await expect(page.locator('.live-source')).toContainText(
    '"Business software"'
  );
  await expect(page.locator('.live-source')).toContainText(
    'Expenses:Hosting  14.25 USD'
  );
  const fields = await page.locator('.transaction-fields').boundingBox();
  const preview = await page.locator('.transaction-preview').boundingBox();
  expect(preview?.x).toBeGreaterThan((fields?.x || 0) + (fields?.width || 0));
  await page.getByRole('button', { name: 'Source', exact: true }).click();
  await expect(page.getByLabel('Transaction source')).toHaveValue(
    /Business software/
  );
  await page
    .getByLabel('Transaction source')
    .fill(
      '2026-06-01 * "Source draft"\n  Expenses:Hosting  5 USD\n  Assets:Checking -5 USD\n'
    );
  await expect(page.locator('.live-source')).toContainText('Source draft');
  await expect(
    page.getByRole('button', { name: 'Fields', exact: true })
  ).toBeDisabled();
  await page.getByRole('button', { name: 'Cancel', exact: true }).click();
});
test('chart tooltips support keyboard inspection and light theme', async ({
  page
}) => {
  await page.goto('/');
  const chart = page.locator('canvas').first();
  await expect(chart).toBeVisible();
  await chart.focus();
  await expect(page.getByRole('tooltip')).toContainText('Jan 26');
  await expect(page.getByRole('tooltip')).toContainText('Revenue');
  await chart.press('ArrowRight');
  await expect(page.getByRole('tooltip')).toContainText('Feb 26');
  await expect(page.getByRole('tooltip')).toContainText('$30.00');
  await chart.press('Escape');
  await expect(page.getByRole('tooltip')).toHaveCount(0);
});
test('report calendar applies custom ranges, cancels drafts, and offers presets on mobile', async ({
  page
}) => {
  await page.clock.setFixedTime(new Date('2026-02-20T12:00:00Z'));
  await page.goto('/');
  const trigger = page.getByRole('button', {
    name: 'Report date range',
    exact: true
  });
  await trigger.click();
  await page.getByLabel('Year', { exact: true }).selectOption('2026');
  await page.getByLabel('Month', { exact: true }).selectOption('1');
  const january = page.locator('.calendar-grid').first();
  await january
    .getByRole('button', { name: 'Thursday, January 1, 2026', exact: true })
    .click();
  await january
    .getByRole('button', { name: 'Saturday, January 31, 2026', exact: true })
    .click();
  await page.getByRole('button', { name: 'Apply', exact: true }).click();
  await expect(trigger).toContainText('Jan 1, 2026');
  await expect(
    page.locator('.metric').filter({ hasText: 'Net profit' })
  ).toContainText('−$12.34');
  await trigger.click();
  await page.getByRole('button', { name: 'Next', exact: true }).click();
  await page.keyboard.press('Escape');
  await expect(trigger).toContainText('Jan 1, 2026');
  await expect(trigger).toBeFocused();
  await page.setViewportSize({ width: 390, height: 844 });
  await trigger.click();
  await expect(page.locator('.calendar-grid')).toHaveCount(1);
  const bounds = await page.locator('.range-popover').boundingBox();
  expect(bounds?.x).toBeGreaterThanOrEqual(0);
  expect((bounds?.x || 0) + (bounds?.width || 0)).toBeLessThanOrEqual(390);
  await page.getByRole('button', { name: 'This month', exact: true }).click();
  await expect(trigger).toContainText('Feb 1, 2026');
  await expect(
    page.locator('.metric').filter({ hasText: 'Net profit' })
  ).toContainText('$30.00');
  await trigger.click();
  await page.getByRole('button', { name: 'All time', exact: true }).click();
  await expect(trigger).toHaveText('All time');
  await expect(
    page.locator('.metric').filter({ hasText: 'Net profit' })
  ).toContainText('$17.66');
});
test('sidebar groups and breadcrumbs navigate, and expense chart drills into accounts', async ({
  page
}) => {
  await page.goto('/');
  const navigation = page.getByRole('navigation', { name: 'Main navigation' });
  await navigation
    .getByRole('button', { name: 'Reports', exact: true })
    .click();
  await expect(
    navigation.getByRole('link', { name: 'Income statement' })
  ).toHaveCount(0);
  await navigation
    .getByRole('button', { name: 'Reports', exact: true })
    .click();
  await navigation.getByRole('link', { name: 'Income statement' }).click();
  const breadcrumb = page.getByRole('navigation', { name: 'Breadcrumb' });
  await expect(breadcrumb).toContainText('Income statement');
  await breadcrumb.getByRole('link').click();
  await expect(page).toHaveURL('/');
  const category = page
    .locator('.expense-chart-legend')
    .getByRole('link', { name: /Hosting/ });
  await category.focus();
  await expect(
    page
      .getByRole('region', { name: 'Expenses composition', exact: true })
      .locator('.expense-ring-label')
  ).toContainText('$12.34');
  await category.click();
  await expect(page.getByLabel('Filter by account')).toHaveValue(
    'Expenses:Hosting'
  );
  await page.goto('/');
  await page
    .getByRole('region', { name: 'Liabilities composition', exact: true })
    .getByRole('link', { name: /Founder/ })
    .click();
  await expect(page.getByLabel('Filter by account')).toHaveValue(
    'Liabilities:Founder'
  );
  await expect(
    page.getByRole('button', { name: 'Workspace settings', exact: true })
  ).toHaveCount(1);
  await page
    .getByRole('button', { name: 'Workspace settings', exact: true })
    .click();
  await expect(page.getByRole('dialog')).toBeVisible();
});
test('journal filters, detail, CSV and account navigation', async ({
  page
}) => {
  await page.goto('/journal');
  await page.getByLabel('Search transactions').fill('European');
  await expect(page.locator('tbody tr')).toHaveCount(1);
  await page.getByRole('button', { name: 'Cloud', exact: true }).click();
  await expect(page.getByRole('dialog')).toContainText('European hosting');
  await expect(page.getByRole('dialog')).toContainText('EUR');
  await page.getByRole('button', { name: 'Close dialog' }).click();
  const download = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Export CSV' }).click();
  expect((await download).suggestedFilename()).toBe('transactions.csv');
  await page.goto('/accounts');
  await page.getByRole('link', { name: 'Expenses Hosting' }).click();
  await expect(page.getByLabel('Filter by account')).toHaveValue(
    'Expenses:Hosting'
  );
});
test('balanced UI write broadcasts to a second browser and preserves stale drafts', async ({
  page,
  browser
}) => {
  const observer = await browser.newPage();
  await page.goto('/journal');
  await observer.goto('/journal');
  await expect(observer.locator('.connection')).toContainText('Live');
  await observer.getByRole('button', { name: 'New transaction' }).click();
  await observer
    .getByLabel('Description', { exact: true })
    .fill('Unsaved observer draft');
  await page.getByRole('button', { name: 'New transaction' }).click();
  await chooseDate(page, 'Transaction date', '2026-04-10');
  await page.getByLabel('Payee', { exact: true }).fill('Browser integration');
  await page.getByLabel('Description', { exact: true }).fill('Realtime update');
  await page.getByLabel('Posting 1 account').fill('Expenses:Hosting');
  await page.getByLabel('Posting 1 amount').fill('8.25');
  await page.getByLabel('Posting 2 account').fill('Assets:Checking');
  await page.getByLabel('Posting 2 amount').fill('-8.25');
  await page
    .getByRole('button', { name: 'Add transaction', exact: true })
    .click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(
    observer.getByText('The ledger changed while this editor was open.', {
      exact: false
    })
  ).toBeVisible();
  await expect(observer.getByLabel('Description', { exact: true })).toHaveValue(
    'Unsaved observer draft'
  );
  await expect(
    observer.getByRole('button', { name: 'Add transaction', exact: true })
  ).toBeDisabled();
  await observer.getByRole('button', { name: 'Cancel', exact: true }).click();
  await expect(
    observer.getByRole('button', { name: 'Browser integration', exact: true })
  ).toBeVisible();
  await observer.close();
});
test('invalid write is rejected, source edit works, and history can restore', async ({
  page
}) => {
  await page.goto('/journal');
  await page.getByRole('button', { name: 'New transaction' }).click();
  await page.getByRole('button', { name: 'Source', exact: true }).click();
  await page
    .getByLabel('Transaction source')
    .fill(
      '2026-04-11 * "Invalid write"\n  Expenses:Hosting  12 USD\n  Assets:Checking -1 USD\n'
    );
  await page
    .getByRole('button', { name: 'Add transaction', exact: true })
    .click();
  await expect(page.getByRole('alert')).toContainText('Validation failed');
  await page.getByRole('button', { name: 'Cancel', exact: true }).click();
  await expect(
    page.getByRole('button', { name: 'Invalid write', exact: true })
  ).toHaveCount(0);
  await page.goto('/files');
  await page.getByRole('button', { name: /main.beancount/ }).click();
  const { content } = await (
    await page.request.post(rpc + 'ReadFile', {
      data: { path: 'main.beancount' }
    })
  ).json();
  await page
    .getByLabel('Ledger file content')
    .fill(content.replace('Example ledger', 'Edited example ledger'));
  await page.getByRole('button', { name: 'Save changes' }).click();
  await expect(page.locator('.workspace-brand')).toContainText(
    'Edited example ledger'
  );
  await page.goto('/history');
  await page.getByRole('button', { name: 'Restore before' }).first().click();
  await page.getByRole('button', { name: 'Confirm', exact: true }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(page.locator('.workspace-brand')).toContainText(
    'Example ledger'
  );
});
test('account lifecycle and delete confirmation', async ({ page }) => {
  await page.goto('/accounts');
  await page.getByRole('button', { name: 'Open account', exact: true }).click();
  await page.getByLabel('Account name').fill('Expenses:BrowserTest');
  await chooseDate(page, 'Opening date', '2026-01-01');
  await page
    .getByRole('dialog')
    .getByRole('button', { name: 'Open account', exact: true })
    .click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  const row = page
    .locator('tr')
    .filter({ has: page.getByRole('link', { name: 'Expenses BrowserTest' }) });
  await expect(row).toContainText('USD');
  await row.getByRole('button', { name: 'Close', exact: true }).click();
  await chooseDate(page, 'Closing date', '2026-12-31');
  await page
    .getByRole('button', { name: 'Close account', exact: true })
    .click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(row).toContainText('Closed');
  await page.goto('/journal');
  await page
    .getByRole('button', { name: 'Browser integration', exact: true })
    .click();
  await page.getByRole('button', { name: 'Delete', exact: true }).click();
  await page.getByRole('button', { name: 'Confirm', exact: true }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(
    page.getByRole('button', { name: 'Browser integration', exact: true })
  ).toHaveCount(0);
});
test('mobile layout, navigation, and keyboard palette', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/');
  await expect(
    page.getByRole('heading', { name: 'Overview', exact: true })
  ).toBeVisible();
  await expect(
    page.getByRole('navigation', { name: 'Main navigation' })
  ).toHaveCount(0);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth
    )
  ).toBe(true);
  await page.getByRole('button', { name: 'Show sidebar' }).click();
  await expect(
    page.getByRole('navigation', { name: 'Main navigation' })
  ).toBeVisible();
  await page.getByRole('button', { name: 'Search workspace' }).click();
  await page.getByLabel('Find a page or account').fill('History');
  await page
    .getByRole('dialog')
    .getByRole('option', { name: 'History', exact: true })
    .click();
  await expect(page).toHaveURL('/history');
  await page.setViewportSize({ width: 320, height: 740 });
  await page.goto('/');
  await expect(page.locator('.account-tree')).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth
    )
  ).toBe(true);
  await page.getByRole('button', { name: 'New transaction' }).click();
  await page.getByLabel('Posting 1 account').fill('hosting');
  await page.getByRole('option', { name: 'Hosting Expenses:Hosting' }).click();
  await expect(page.getByLabel('Posting 1 account')).toHaveValue(
    'Expenses:Hosting'
  );
  const dialog = page.getByRole('dialog');
  expect(
    await dialog.evaluate(
      (element) => element.scrollWidth <= element.clientWidth
    )
  ).toBe(true);
  await page.getByRole('button', { name: 'Cancel', exact: true }).click();
});

test('ledger editor highlights source, fills the viewport, and keeps keyboard editing intact', async ({
  page
}) => {
  await page.setViewportSize({ width: 1920, height: 1080 });
  await page.goto('/files?path=transactions/2026.beancount');
  const editor = page.getByRole('textbox', { name: 'Ledger file content' });
  await expect(editor).toBeVisible();
  await expect(
    page.locator('.ledger-code-editor .syntax-date').first()
  ).toBeVisible();
  await expect(
    page.locator('.ledger-code-editor .syntax-account').first()
  ).toBeVisible();
  await expect(
    page.locator('.ledger-code-editor .syntax-string').first()
  ).toBeVisible();
  const panel = await page.locator('.file-editor').boundingBox();
  if (!panel) throw new Error('File editor is missing');
  expect(panel.width).toBeGreaterThan(1300);
  expect(panel.height).toBeGreaterThan(800);
  expect(panel.y + panel.height).toBeLessThan(1080);
  const firstLine = await page.locator('.cm-line').first().boundingBox();
  const firstNumber = await page
    .locator('.cm-lineNumbers .cm-gutterElement')
    .nth(1)
    .boundingBox();
  if (!firstLine || !firstNumber)
    throw new Error('Editor text or line numbers missing');
  expect(Math.abs(firstLine.y - firstNumber.y)).toBeLessThan(3);
  expect(firstLine.x).toBeGreaterThan(firstNumber.x);
  const nonce = await page
    .locator('meta[name="style-nonce"]')
    .getAttribute('content');
  expect(nonce).not.toContain('__LEDGER_STYLE_NONCE__');
  expect(
    await page
      .locator('style')
      .evaluateAll((styles) =>
        styles.some(
          (style) =>
            style.nonce ===
            document
              .querySelector('meta[name="style-nonce"]')
              ?.getAttribute('content')
        )
      )
  ).toBe(true);

  await editor.press('ControlOrMeta+End');
  await editor.press('Enter');
  await page.keyboard.insertText('; browser editor note');
  await expect(page.locator('.file-heading')).toContainText('Unsaved');
  await expect(
    page.locator('.ledger-code-editor .syntax-comment').last()
  ).toHaveText('; browser editor note');
  await editor.press('ControlOrMeta+z');
  await expect(editor).not.toContainText('browser editor note');
  await editor.press('ControlOrMeta+Shift+z');
  await expect(editor).toContainText('browser editor note');
  await editor.press('ControlOrMeta+s');
  await expect(page.locator('.file-heading')).not.toContainText('Unsaved');
  const saved = await (
    await page.request.post(rpc + 'ReadFile', {
      data: { path: 'transactions/2026.beancount' }
    })
  ).json();
  expect(saved.content).toContain('; browser editor note');
  await page.getByRole('button', { name: 'Toggle color theme' }).click();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  await expect(
    page.locator('.ledger-code-editor .syntax-comment').last()
  ).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390
  );
  const mobilePanel = await page.locator('.file-editor').boundingBox();
  if (!mobilePanel) throw new Error('Mobile file editor is missing');
  expect(mobilePanel.width).toBeLessThan(390);
  expect(mobilePanel.height).toBeGreaterThan(400);
});

test('ledger editor respects read-only access and file switches', async ({
  page
}) => {
  await page.route('**' + rpc + 'GetSession', (r) =>
    r.fulfill({
      json: {
        ...signedOut,
        authenticated: true,
        canWrite: false,
        user: { name: 'Reader' }
      }
    })
  );
  await page.goto('/files?path=main.beancount');
  const editor = page.getByRole('textbox', { name: 'Ledger file content' });
  await expect(editor).toHaveAttribute('aria-readonly', 'true');
  await expect(editor).toContainText('option');
  await editor.press('ControlOrMeta+End');
  await page.keyboard.type('unexpected edit');
  await expect(editor).not.toContainText('unexpected edit');
  await expect(
    page.getByRole('button', { name: 'Save changes' })
  ).toBeDisabled();
  await page.getByRole('button', { name: /accounts.beancount/ }).click();
  await expect(editor).toContainText('open');
  await expect(editor).not.toContainText('option');
});
