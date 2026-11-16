// Tests for the formatting helpers. Plain node:test, no extra dependencies.

import test from "node:test";
import assert from "node:assert/strict";

import {
  formatAge,
  formatBytes,
  formatPercent,
  formatUptime,
  meterBand,
  plural,
  usedPercent,
} from "./format.js";

test("formatBytes", () => {
  const cases = new Map([
    [0, "0 MiB"],
    [MIB(), "1 MiB"],
    [1024 * 1024 * 1024, "1.0 GiB"],
    [1024 * 1024 * 1536, "1.5 GiB"],
    [500 * 1024 * 1024, "500 MiB"],
  ]);
  for (const [bytes, want] of cases) {
    assert.equal(formatBytes(bytes), want, `formatBytes(${bytes})`);
  }
});

test("formatBytes copes with junk", () => {
  assert.equal(formatBytes(undefined), "0 MiB");
  assert.equal(formatBytes(Number.NaN), "0 MiB");
  assert.equal(formatBytes(-5), "0 MiB");
});

test("formatUptime", () => {
  const cases = new Map([
    [0, "0m"],
    [61, "1m"],
    [3600, "1h 0m"],
    [86400, "1d 0h"],
    [90061, "1d 1h"],
  ]);
  for (const [seconds, want] of cases) {
    assert.equal(formatUptime(seconds), want, `formatUptime(${seconds})`);
  }
});

test("formatAge picks a readable unit", () => {
  const cases = new Map([
    [0, "just now"],
    [0.5, "just now"],
    [12, "12s ago"],
    [20 * 60, "20m ago"],
    [3 * 3600 + 5 * 60, "3h 5m ago"],
    [50 * 3600, "2d ago"],
  ]);
  for (const [seconds, want] of cases) {
    assert.equal(formatAge(seconds), want, `formatAge(${seconds})`);
  }
});

test("formatPercent", () => {
  assert.equal(formatPercent(12.5), "12.5%");
  assert.equal(formatPercent(0), "0.0%");
  assert.equal(formatPercent(undefined), "0.0%");
});

test("usedPercent stays inside the track", () => {
  assert.equal(usedPercent({ used: 5, total: 10 }), 50);
  assert.equal(usedPercent({ used: 0, total: 0 }), 0);
  assert.equal(usedPercent({ used: 20, total: 10 }), 100);
  assert.equal(usedPercent(undefined), 0);
});

test("meterBand changes at the documented thresholds", () => {
  assert.equal(meterBand({ used: 0, total: 100 }), "");
  assert.equal(meterBand({ used: 69, total: 100 }), "");
  assert.equal(meterBand({ used: 70, total: 100 }), "warn");
  assert.equal(meterBand({ used: 89, total: 100 }), "warn");
  assert.equal(meterBand({ used: 90, total: 100 }), "crit");
  assert.equal(meterBand({ used: 100, total: 100 }), "crit");
});

test("plural", () => {
  assert.equal(plural(0, "node", "nodes"), "nodes");
  assert.equal(plural(1, "node", "nodes"), "node");
  assert.equal(plural(2, "node", "nodes"), "nodes");
});

/** One mebibyte, written out to keep the tables above readable. */
function MIB() {
  return 1024 * 1024;
}
