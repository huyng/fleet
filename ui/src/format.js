// Formatting helpers for the figures the master reports.
//
// Everything here is pure so it can be unit tested without a browser. The
// master sends raw numbers; all human readable text is made here.

// The byte and percentage thresholds used across the page.
const MIB = 1024 * 1024;
const GIB = 1024 * MIB;

/**
 * Format a byte count as GiB or MiB, whichever reads better.
 *
 * @param {number} bytes Count of bytes. Missing or negative input gives "0 MiB".
 * @returns {string} For example "1.4 GiB" or "512 MiB".
 */
export function formatBytes(bytes) {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return "0 MiB";
  }
  if (bytes >= GIB) {
    return `${(bytes / GIB).toFixed(1)} GiB`;
  }
  return `${Math.round(bytes / MIB)} MiB`;
}

/**
 * Format a number of seconds of uptime as days, hours or minutes.
 *
 * Only the two largest units are kept, so the result stays short:
 * "3d 4h", "5h 12m", "9m".
 *
 * @param {number} seconds Seconds the machine has been up.
 * @returns {string} A short duration, never empty.
 */
export function formatUptime(seconds) {
  if (!Number.isFinite(seconds) || seconds < 0) {
    return "0m";
  }

  const totalMinutes = Math.floor(seconds / 60);
  const days = Math.floor(totalMinutes / (60 * 24));
  const hours = Math.floor(totalMinutes / 60) % 24;
  const minutes = totalMinutes % 60;

  if (days > 0) {
    return `${days}d ${hours}h`;
  }
  if (hours > 0) {
    return `${hours}h ${minutes}m`;
  }
  return `${minutes}m`;
}

/**
 * Describe an age in the largest unit that still means something.
 *
 * @param {number} seconds How long ago something happened.
 * @returns {string} "just now", "42s ago", "7m ago", "3h 5m ago" or "2d ago".
 */
export function formatAge(seconds) {
  if (!Number.isFinite(seconds) || seconds < 1) {
    return "just now";
  }
  if (seconds < 60) {
    return `${Math.floor(seconds)}s ago`;
  }
  if (seconds < 3600) {
    return `${Math.floor(seconds / 60)}m ago`;
  }
  if (seconds < 86400) {
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor(seconds / 60) % 60;
    return `${hours}h ${minutes}m ago`;
  }
  return `${Math.floor(seconds / 86400)}d ago`;
}

/**
 * Format a percentage with one decimal place.
 *
 * @param {number} value The percentage.
 * @returns {string} For example "12.5%".
 */
export function formatPercent(value) {
  if (!Number.isFinite(value) || value < 0) {
    return "0.0%";
  }
  return `${value.toFixed(1)}%`;
}

/**
 * Work out the used share of a used and total pair.
 *
 * @param {{used: number, total: number}} usage The pair from the master.
 * @returns {number} A percentage clamped to 0 to 100, so a bar never overflows.
 */
export function usedPercent(usage) {
  const total = usage?.total ?? 0;
  const used = usage?.used ?? 0;
  if (total <= 0) {
    return 0;
  }
  const percent = (used / total) * 100;
  return Math.min(100, Math.max(0, percent));
}

/**
 * Pick the colour band for a usage figure.
 *
 * @param {{used: number, total: number}} usage The pair from the master.
 * @returns {string} "" for normal, "warn" past 70%, "crit" past 90%.
 */
export function meterBand(usage) {
  const percent = usedPercent(usage);
  if (percent >= 90) {
    return "crit";
  }
  if (percent >= 70) {
    return "warn";
  }
  return "";
}

/**
 * Pick the singular or plural form of a word for a count.
 *
 * @param {number} count The number the word describes.
 * @param {string} one The form for exactly one.
 * @param {string} many The form for any other count, including zero.
 * @returns {string} The matching word.
 */
export function plural(count, one, many) {
  return count === 1 ? one : many;
}
