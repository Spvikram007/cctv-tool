import { test, expect } from '@playwright/test';

test.describe('CCTV Sentinel Universal — Platform Core & Navigation', () => {
  test('loads dashboard and displays system health', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('body')).toContainText('CCTV SENTINEL');
    await expect(page.locator('body')).toContainText('Tech Defence Solutions');
  });

  test('navigates through all core platform modules', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('body')).toContainText('PLATFORM MODULES');

    // 1. Auto Discovery
    await page.click('button:has-text("Auto Discovery")');
    await expect(page.locator('body')).toContainText('DISCOVERY ENGINE');

    // 2. Device Inventory
    await page.click('button:has-text("Device Inventory")');
    await expect(page.locator('body')).toContainText('ENROLLED CCTV FLEET INVENTORY');

    // 3. Live Video Grid
    await page.click('button:has-text("Live Video Grid")');
    await expect(page.locator('body')).toContainText('LIVE');

    // 4. Universal Config
    await page.click('button:has-text("Universal Config")');
    await expect(page.locator('body')).toContainText('CONFIGURATION');

    // 5. Recorder & XVR
    await page.click('button:has-text("Recorder & XVR")');
    await expect(page.locator('body')).toContainText('RECORDER');

    // 6. PTZ & Alarms
    await page.click('button:has-text("PTZ & Alarms")');
    await expect(page.locator('body')).toContainText('PTZ ROBOTIC CONTROL');

    // 7. Diagnostics & RTSP
    await page.click('button:has-text("Diagnostics & RTSP")');
    await expect(page.locator('body')).toContainText('DIAGNOSTICS');

    // 8. Calculators
    await page.click('button:has-text("Calculators")');
    await expect(page.locator('body')).toContainText('CALCULATOR');

    // 9. Batch Configuration
    await page.click('button:has-text("Batch Configuration")');
    await expect(page.locator('body')).toContainText('BATCH');

    // 10. Firmware & Updates
    await page.click('button:has-text("Firmware & Updates")');
    await expect(page.locator('body')).toContainText('FIRMWARE');

    // 11. Hardware Matrix
    await page.click('button:has-text("Hardware Matrix")');
    await expect(page.locator('body')).toContainText('MATRIX');

    // 12. Licensing & GST
    await page.click('button:has-text("Licensing & GST")');
    await expect(page.locator('body')).toContainText('LICENSE');

    // 13. Audit & Reports
    await page.click('button:has-text("Audit & Reports")');
    await expect(page.locator('body')).toContainText('AUDIT');
  });
});
