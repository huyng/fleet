// Take screenshots of the running master's page.
//
// Usage:
//   node test/screenshot.mjs <url> <outDir>
//
// This is a development aid, not part of the test suite: it needs the headless
// browser that playwright install can fetch, and a master to point at.

import { mkdir } from "node:fs/promises";
import { chromium } from "playwright";

const url = process.argv[2] || "http://127.0.0.1:8080/";
const outDir = process.argv[3] || "/tmp/shots";

// The widths to capture. 1280 is a laptop, 390 an iPhone.
const viewports = [
  { name: "desktop", width: 1280, height: 900 },
  { name: "mobile", width: 390, height: 844 },
];

await mkdir(outDir, { recursive: true });

const browser = await chromium.launch({
  args: ["--no-sandbox", "--disable-gpu", "--disable-dev-shm-usage"],
});

// Anything the page logs is worth seeing: a React warning or a failed fetch
// would be invisible in a screenshot.
const problems = [];

for (const viewport of viewports) {
  for (const theme of ["dark", "light"]) {
    const context = await browser.newContext({
      viewport: { width: viewport.width, height: viewport.height },
      deviceScaleFactor: 2,
      colorScheme: theme,
    });
    const page = await context.newPage();

    page.on("console", (message) => {
      if (message.type() === "error" || message.type() === "warning") {
        problems.push(`${viewport.name}/${theme} console: ${message.text()}`);
      }
    });
    page.on("pageerror", (err) => {
      problems.push(`${viewport.name}/${theme} page error: ${err.message}`);
    });
    page.on("requestfailed", (request) => {
      problems.push(`${viewport.name}/${theme} failed: ${request.url()}`);
    });

    await page.goto(url, { waitUntil: "networkidle" });
    // The app polls, so give it a moment to settle on real data.
    await page.waitForTimeout(1500);

    const file = `${outDir}/${viewport.name}-${theme}.png`;
    await page.screenshot({ path: file, fullPage: true });
    console.log(`wrote ${file}`);

    await context.close();
  }
}

// Also capture the process list folded open, since that is the densest part
// of the page and the part most likely to break.
{
  const context = await browser.newContext({
    viewport: { width: 1280, height: 900 },
    deviceScaleFactor: 2,
    colorScheme: "dark",
  });
  const page = await context.newPage();
  await page.goto(url, { waitUntil: "networkidle" });
  await page.waitForTimeout(1000);

  const details = page.locator("details.procs").first();
  if (await details.count()) {
    await details.click();
    await page.waitForTimeout(300);
    await page.screenshot({ path: `${outDir}/procs-open.png`, fullPage: true });
    console.log(`wrote ${outDir}/procs-open.png`);
  } else {
    console.log("no process list on the page, skipped the open view");
  }

  await context.close();
}

await browser.close();

if (problems.length > 0) {
  console.log("\nproblems seen while loading:");
  for (const problem of new Set(problems)) {
    console.log(`  ${problem}`);
  }
} else {
  console.log("\nno console errors, warnings or failed requests");
}
