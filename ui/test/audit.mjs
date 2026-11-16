// Measure the running page for the layout faults that are easy to miss
// without eyes: horizontal overflow, clipped text, overlapping cards, and
// text that is too low contrast to read.
//
// Usage:
//   node test/audit.mjs <url>
//
// Prints a report and exits non-zero when something is clearly wrong.

import { chromium } from "playwright";

const url = process.argv[2] || "http://127.0.0.1:8080/";

/**
 * Parse a CSS colour into channels plus alpha.
 *
 * @param {string} color Such as "rgb(1, 2, 3)" or "rgba(1, 2, 3, 0.08)".
 * @returns {number[]} Red, green, blue and alpha.
 */
function parseColor(color) {
  const numbers = (color.match(/[\d.]+/g) || []).map(Number);
  if (numbers.length < 3) {
    return [0, 0, 0, 0];
  }
  return [numbers[0], numbers[1], numbers[2], numbers.length > 3 ? numbers[3] : 1];
}

/**
 * Lay a translucent colour over an opaque one.
 *
 * @param {number[]} fg Colour on top, with alpha.
 * @param {number[]} bg Opaque colour underneath.
 * @returns {number[]} The resulting opaque colour.
 */
function composite(fg, bg) {
  const alpha = fg[3];
  return [
    fg[0] * alpha + bg[0] * (1 - alpha),
    fg[1] * alpha + bg[1] * (1 - alpha),
    fg[2] * alpha + bg[2] * (1 - alpha),
    1,
  ];
}

/**
 * Relative luminance, per the WCAG definition.
 *
 * @param {number[]} color Channels, alpha ignored.
 * @returns {number} Luminance from 0 to 1.
 */
function luminance(color) {
  const [r, g, b] = color.slice(0, 3).map((value) => {
    const channel = value / 255;
    return channel <= 0.03928
      ? channel / 12.92
      : ((channel + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

/**
 * Contrast ratio between two opaque colours.
 *
 * @param {number[]} a Colour channels.
 * @param {number[]} b Colour channels.
 * @returns {number} Ratio from 1 to 21.
 */
function contrast(a, b) {
  const la = luminance(a);
  const lb = luminance(b);
  const [light, dark] = la > lb ? [la, lb] : [lb, la];
  return (light + 0.05) / (dark + 0.05);
}

const browser = await chromium.launch({
  args: ["--no-sandbox", "--disable-gpu", "--disable-dev-shm-usage"],
});

const report = [];
let failed = false;

/**
 * Record one finding.
 *
 * @param {boolean} ok Whether the check passed.
 * @param {string} message What was found.
 */
function check(ok, message) {
  report.push(`${ok ? "ok   " : "FAIL "} ${message}`);
  if (!ok) {
    failed = true;
  }
}

for (const viewport of [
  { name: "desktop", width: 1280, height: 900 },
  { name: "mobile", width: 390, height: 844 },
]) {
  for (const theme of ["dark", "light"]) {
    const context = await browser.newContext({
      viewport: { width: viewport.width, height: viewport.height },
      colorScheme: theme,
    });
    const page = await context.newPage();
    await page.goto(url, { waitUntil: "networkidle" });
    await page.waitForTimeout(1200);

    const label = `${viewport.name}/${theme}`;

    const metrics = await page.evaluate(() => {
      const root = document.documentElement;
      const body = document.body;

      // Elements whose content is wider than their own box: clipped text.
      const clipped = [];
      for (const el of document.querySelectorAll(
        ".node-name, .meter-value, .chip, .tally li, .loads b, .stamp, .label",
      )) {
        if (el.scrollWidth > el.clientWidth + 1) {
          clipped.push(`${el.className || el.tagName}: ${el.textContent.trim()}`);
        }
      }

      // Cards that sit on top of each other.
      const boxes = [...document.querySelectorAll("article.node")].map((el) => {
        const box = el.getBoundingClientRect();
        return {
          name: el.querySelector(".node-name")?.textContent ?? "?",
          left: box.left,
          right: box.right,
          top: box.top,
          bottom: box.bottom,
        };
      });
      const overlaps = [];
      for (let i = 0; i < boxes.length; i++) {
        for (let j = i + 1; j < boxes.length; j++) {
          const a = boxes[i];
          const b = boxes[j];
          const horizontal = a.left < b.right - 1 && b.left < a.right - 1;
          const vertical = a.top < b.bottom - 1 && b.top < a.bottom - 1;
          if (horizontal && vertical) {
            overlaps.push(`${a.name} overlaps ${b.name}`);
          }
        }
      }

      const style = getComputedStyle(body);
      const muted = getComputedStyle(
        document.querySelector(".stamp") || body,
      ).color;

      // Small coloured text on tinted backgrounds: pills and labels. The
      // backgrounds are translucent, so they have to be composited down to the
      // page before a contrast figure means anything.
      const tinted = [];
      for (const selector of [".pill.up", ".pill.stale", ".label", ".chip", ".row dt", ".tally li"]) {
        const el = document.querySelector(selector);
        if (!el) {
          continue;
        }
        const layers = [];
        let node = el;
        while (node) {
          layers.push(getComputedStyle(node).backgroundColor);
          node = node.parentElement;
        }
        tinted.push({
          selector,
          color: getComputedStyle(el).color,
          // Outermost first, so the page background is the base.
          layers: layers.reverse(),
          fontSize: getComputedStyle(el).fontSize,
        });
      }

      // Text that is much smaller than the 14px base is hard to read.
      const tiny = [];
      for (const el of document.querySelectorAll(".chip, .tally li, .stamp, .row dt")) {
        const size = Number.parseFloat(getComputedStyle(el).fontSize);
        if (size < 10) {
          tiny.push(`${el.className}: ${size}px`);
        }
      }

      // A master with no nodes shows the empty state instead of a grid, so
      // the grid measurements are only meaningful when it is there.
      const grid = document.querySelector(".nodes");

      return {
        overflowX: root.scrollWidth > root.clientWidth + 1,
        scrollWidth: root.scrollWidth,
        clientWidth: root.clientWidth,
        hasGrid: Boolean(grid),
        columns: grid ? getComputedStyle(grid).gridTemplateColumns : "",
        cardCount: boxes.length,
        cardWidth: boxes[0] ? Math.round(boxes[0].right - boxes[0].left) : 0,
        clipped,
        overlaps,
        tiny,
        bodyBg: style.backgroundColor,
        bodyColor: style.color,
        mutedColor: muted,
        fontFamily: style.fontFamily,
        monoFamily: getComputedStyle(document.querySelector(".stamp") || body).fontFamily,
        tinted,
      };
    });

    check(!metrics.overflowX,
      `${label}: no horizontal overflow (scroll ${metrics.scrollWidth} vs view ${metrics.clientWidth})`);
    check(metrics.clipped.length === 0,
      `${label}: nothing clipped ${metrics.clipped.length ? "-> " + metrics.clipped.join(", ") : ""}`);
    check(metrics.overlaps.length === 0,
      `${label}: cards do not overlap ${metrics.overlaps.length ? "-> " + metrics.overlaps.join(", ") : ""}`);
    check(metrics.tiny.length === 0,
      `${label}: no text under 10px ${metrics.tiny.length ? "-> " + metrics.tiny.join(", ") : ""}`);

    const bodyBg = parseColor(metrics.bodyBg);
    const bodyContrast = contrast(parseColor(metrics.bodyColor), bodyBg);
    check(bodyContrast >= 7,
      `${label}: body text contrast ${bodyContrast.toFixed(1)}:1 (want 7+)`);

    const mutedContrast = contrast(parseColor(metrics.mutedColor), bodyBg);
    check(mutedContrast >= 4.5,
      `${label}: muted text contrast ${mutedContrast.toFixed(1)}:1 (want 4.5+)`);

    // Small text needs 4.5:1. This is where a bright accent used as a colour
    // for letters goes wrong in the light palette.
    for (const item of metrics.tinted) {
      let background = [255, 255, 255, 1];
      for (const layer of item.layers) {
        background = composite(parseColor(layer), background);
      }
      const ratio = contrast(parseColor(item.color), background);
      check(ratio >= 4.5,
        `${label}: ${item.selector} contrast ${ratio.toFixed(2)}:1 at ${item.fontSize} (want 4.5+)`);
    }

    if (!metrics.hasGrid) {
      check(metrics.cardCount === 0,
        `${label}: no grid and no cards, which is the empty state`);
    } else if (viewport.name === "desktop") {
      check(metrics.cardWidth >= 300 && metrics.cardWidth <= 460,
        `${label}: card width ${metrics.cardWidth}px`);
      check(!metrics.columns.startsWith("1 "),
        `${label}: grid has ${metrics.columns.split(" ").length} column(s)`);
    } else {
      check(metrics.columns.split(" ").length === 1,
        `${label}: single column on mobile (${metrics.columns})`);
      check(metrics.cardWidth <= viewport.width,
        `${label}: card width ${metrics.cardWidth}px fits ${viewport.width}px`);
    }

    check(metrics.fontFamily.toLowerCase().includes("outfit"),
      `${label}: headings use Outfit (${metrics.fontFamily.split(",")[0]})`);
    check(metrics.monoFamily.toLowerCase().includes("jetbrains"),
      `${label}: figures use JetBrains Mono (${metrics.monoFamily.split(",")[0]})`);

    await context.close();
  }
}

await browser.close();

console.log(report.join("\n"));
if (failed) {
  console.log("\nsomething above needs fixing");
  process.exitCode = 1;
} else {
  console.log("\nall layout checks passed");
}
