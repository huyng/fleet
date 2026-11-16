// Turning master timestamps into ages the page can show.
//
// Ages are measured against the master's clock, not the browser's. A laptop
// with a clock that is ten minutes fast would otherwise make every node look
// stale. The master sends its own time with each snapshot, so the offset is
// known and stays valid as the page sits open.

/**
 * Work out the server's clock from a snapshot.
 *
 * @param {string} serverTime RFC3339 time the master sent.
 * @param {number} fetchedAt Browser time when the response arrived.
 * @returns {{serverTime: number, fetchedAt: number}} Milliseconds, or zeros
 *   when the master sent no usable time.
 */
export function serverClock(serverTime, fetchedAt) {
  const parsed = Date.parse(serverTime);
  if (!Number.isFinite(parsed)) {
    return { serverTime: 0, fetchedAt };
  }
  return { serverTime: parsed, fetchedAt };
}

/**
 * How many seconds ago, on the master's clock, a node reported.
 *
 * @param {string} lastSeen RFC3339 time from the master.
 * @param {{serverTime: number, fetchedAt: number}} clock From serverClock.
 * @param {number} now Current browser time in milliseconds.
 * @returns {number} Age in seconds, or NaN when it cannot be worked out.
 */
export function secondsSince(lastSeen, clock, now) {
  const reported = Date.parse(lastSeen);
  if (!Number.isFinite(reported) || !clock?.serverTime) {
    return Number.NaN;
  }
  // The server's time now is its time at fetch, plus however long ago that was.
  const elapsedSinceFetch = now - clock.fetchedAt;
  return (clock.serverTime + elapsedSinceFetch - reported) / 1000;
}
