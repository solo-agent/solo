import { expect, type Locator, type Page } from '@playwright/test';

export async function selectValue(scope: Page | Locator, label: string, value: string) {
  const trigger = scope.getByRole('button', { name: label, exact: true });
  await trigger.click();
  await trigger.page().getByRole('listbox').locator(`[data-value=${JSON.stringify(value)}]`).click();
  await expect(trigger).toHaveAttribute('aria-expanded', 'false');
}
