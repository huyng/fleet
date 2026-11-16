// Tests for the clock helpers, using fixed times so nothing depends on when
// the suite runs.

import test from "node:test";
import assert from "node:assert/strict";

import { secondsSince, serverClock } from "./clock.js";

// A fixed instant: 2025-11-16T04:00:00Z.
const T0 = Date.UTC(2025, 10, 16, 4, 0, 0);

test("secondsSince measures against the master's clock", () => {
  const clock = serverClock(new Date(T0).toISOString(), T0);
  const reported = new Date(T0 - 12000).toISOString();

  // Nothing has elapsed since the fetch.
  assert.equal(secondsSince(reported, clock, T0), 12);
});

test("secondsSince grows as the page sits open", () => {
  const clock = serverClock(new Date(T0).toISOString(), T0);
  const reported = new Date(T0 - 12000).toISOString();

  // Five seconds later, without a new fetch.
  assert.equal(secondsSince(reported, clock, T0 + 5000), 17);
});

test("secondsSince ignores a browser clock that disagrees", () => {
  // The browser thinks it is an hour later than the master. Only the offset
  // between the two clocks matters, so the age is still 30 seconds.
  const clock = serverClock(new Date(T0).toISOString(), T0 + 3600_000);
  const reported = new Date(T0 - 30000).toISOString();

  assert.equal(secondsSince(reported, clock, T0 + 3600_000), 30);
});

test("secondsSince gives NaN rather than a wrong number", () => {
  assert.ok(Number.isNaN(secondsSince("not a time", serverClock("", 0), 0)));
  assert.ok(Number.isNaN(secondsSince(new Date(T0).toISOString(), { serverTime: 0 }, T0)));
});

test("serverClock falls back when the master sends no time", () => {
  const clock = serverClock("", 1234);
  assert.equal(clock.serverTime, 0);
  assert.equal(clock.fetchedAt, 1234);
});
