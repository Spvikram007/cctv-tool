import { test, expect } from '@playwright/test';

test.describe('CCTV Sentinel Universal — Phase 7 PTZ & Alarms Suite', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('body')).toContainText('PLATFORM MODULES');
    await page.click('button:has-text("PTZ & Alarms")');
    await expect(page.locator('body')).toContainText('PTZ ROBOTIC CONTROL & UNIFIED ALARM OPERATIONS');
  });

  test('Screen 1: PTZ Live Control — Capability Detection on Vatilon H43 vs Fixed Camera', async ({ page }) => {
    const unitSelector = page.locator('select').first();
    const options = await unitSelector.locator('option').allInnerTexts();
    const vatilonOption = options.find((opt) => opt.includes('Vatilon') || opt.includes('H43') || opt.includes('104'));

    if (vatilonOption) {
      await unitSelector.selectOption({ label: vatilonOption });
      await expect(page.locator('body')).toContainText('VERIFIED PTZ MOTORIZED CAMERA');
      await expect(page.locator('button[title="Tilt Up"]')).toBeVisible();
      await expect(page.locator('button[title="Pan Left"]')).toBeVisible();
      await expect(page.locator('button[title="Emergency Stop All Motors"]')).toBeVisible();
    }

    const fixedOption = options.find((opt) => opt.includes('101') || opt.includes('102') || opt.includes('103') || opt.includes('DS-2CD') || opt.includes('DH-'));
    if (fixedOption) {
      await unitSelector.selectOption({ label: fixedOption });
      await expect(page.locator('body')).toContainText('FIXED CAMERA - PTZ CONTROLS LOCKED');
      await expect(page.locator('body')).toContainText('This physical hardware unit is a fixed focal-length camera');
    }
  });

  test('Screen 2: PTZ Presets — View coordinate memory', async ({ page }) => {
    await page.getByRole('button', { name: /Presets/i }).click();
    await expect(page.locator('body')).toContainText('PRESET');
  });

  test('Screen 3: Patrol Tours — View waypoint cycles and cruise engine', async ({ page }) => {
    await page.getByRole('button', { name: /Patrol Tours/i }).click();
    await expect(page.locator('body')).toContainText('PATROL');
  });

  test('Screen 4: Event Subscriptions — View active listener threads', async ({ page }) => {
    await page.getByRole('button', { name: /Subscriptions/i }).click();
    await expect(page.locator('body')).toContainText('ACTIVE CCTV EVENT SUBSCRIPTIONS');
    await expect(page.locator('th:has-text("PROTOCOL")')).toBeVisible();
    await expect(page.locator('th:has-text("ENDPOINT")')).toBeVisible();
  });

  test('Screen 5: Live Events — View telemetry and category filter', async ({ page }) => {
    await page.getByRole('button', { name: /Live Events/i }).click();
    await expect(page.locator('body')).toContainText('REAL-TIME SURVEILLANCE EVENT TELEMETRY');
    const categorySelect = page.locator('select:has-text("ALL CATEGORIES")');
    await expect(categorySelect).toBeVisible();
  });

  test('Screen 6: Alarm Dashboard — View KPI metrics and incident queue', async ({ page }) => {
    await page.getByRole('button', { name: /Alarm Dashboard/i }).click();
    await expect(page.locator('body')).toContainText('TOTAL ALARMS');
    await expect(page.locator('body')).toContainText('CRITICAL SEVERITY');
    await expect(page.locator('body')).toContainText('UNACKNOWLEDGED');
    await expect(page.locator('body')).toContainText('RESOLVED / CLOSED');
    await expect(page.locator('body')).toContainText('UNRESOLVED ALARM INCIDENT QUEUE');
  });

  test('Screen 7: Alarm Logs — Searchable table and export links', async ({ page }) => {
    await page.getByRole('button', { name: /Alarm Logs/i }).click();
    await expect(page.locator('body')).toContainText('ALARM AUDIT LOGS & EVIDENCE ARCHIVE');
    await expect(page.locator('a:has-text("EXPORT CSV")')).toBeVisible();
    await expect(page.locator('a:has-text("EXPORT JSON")')).toBeVisible();
  });

  test('Screen 8: Alarm Rules — Builder form and active automation rules', async ({ page }) => {
    await page.getByRole('button', { name: /Alarm Rules/i }).click();
    await expect(page.locator('body')).toContainText('ALARM');
    await expect(page.locator('button:has-text("ACTIVATE ALARM RULE")')).toBeVisible();
  });

  test('Screen 9: Evidence Viewer — Truth Rule compliance audit', async ({ page }) => {
    await page.getByRole('button', { name: /Evidence Viewer/i }).click();
    await expect(page.locator('body')).toContainText('ALARM EVIDENCE VIEWER & TRUTHFUL CAPTURE AUDIT');
    await expect(page.locator('body')).toContainText(/Truth Rule Verified|TRUTHFUL TIMESTAMP AUDIT/i);
  });

  test('Screen 10: Event Matrix — Fleet-wide hardware certification table', async ({ page }) => {
    await page.getByRole('button', { name: /Event Matrix/i }).click();
    await expect(page.locator('body')).toContainText('UNIVERSAL CCTV FLEET EVENT & PTZ COMPATIBILITY MATRIX');
    await expect(page.locator('th:has-text("DEVICE MODEL")')).toBeVisible();
    await expect(page.locator('th:has-text("PTZ MOTOR")')).toBeVisible();
    await expect(page.locator('th:has-text("EVENT PROTOCOL")')).toBeVisible();
  });
});
