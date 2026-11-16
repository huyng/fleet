// React hooks that hold the page's state: the polled snapshot and the theme.

import { useCallback, useEffect, useRef, useState } from "react";

import { fetchNodes, REFRESH_MS } from "./api.js";
import { serverClock } from "./clock.js";

// Where the chosen theme is kept between visits.
const THEME_KEY = "fleet_theme";

/**
 * Poll the master for a snapshot, keeping the last good one when a poll fails.
 *
 * A single missed request should not blank the page, so the nodes stay on
 * screen and the failure is reported beside them.
 *
 * @returns {{nodes: Array<object>, clock: object, error: string,
 *   loading: boolean, updatedAt: number, refresh: function}} Current state.
 */
export function useNodes() {
  const [state, setState] = useState({
    nodes: [],
    clock: { serverTime: 0, fetchedAt: 0 },
    error: "",
    loading: true,
    updatedAt: 0,
  });

  // Stops a slow reply from updating state after the page has gone.
  const alive = useRef(true);

  const refresh = useCallback(async () => {
    try {
      const snapshot = await fetchNodes();
      if (!alive.current) {
        return;
      }
      const fetchedAt = Date.now();
      setState({
        nodes: snapshot.nodes,
        clock: serverClock(snapshot.serverTime, fetchedAt),
        error: "",
        loading: false,
        updatedAt: fetchedAt,
      });
    } catch (err) {
      if (!alive.current) {
        return;
      }
      setState((previous) => ({
        ...previous,
        loading: false,
        error: err.message || "could not reach the master",
      }));
    }
  }, []);

  useEffect(() => {
    alive.current = true;
    refresh();

    const timer = setInterval(refresh, REFRESH_MS);
    return () => {
      alive.current = false;
      clearInterval(timer);
    };
  }, [refresh]);

  return { ...state, refresh };
}

/**
 * The colour palette to use, starting from the saved choice and otherwise from
 * the operating system setting.
 *
 * @returns {[string, function]} The theme, and a function that flips it.
 */
export function useTheme() {
  const [theme, setTheme] = useState(() => {
    const stored = localStorage.getItem(THEME_KEY);
    if (stored === "light" || stored === "dark") {
      return stored;
    }
    return window.matchMedia?.("(prefers-color-scheme: light)").matches
      ? "light"
      : "dark";
  });

  useEffect(() => {
    document.documentElement.classList.toggle("light", theme === "light");
    localStorage.setItem(THEME_KEY, theme);
  }, [theme]);

  const flip = useCallback(() => {
    setTheme((current) => (current === "light" ? "dark" : "light"));
  }, []);

  return [theme, flip];
}

/**
 * Re-render every second so ages like "12s ago" stay true between polls.
 *
 * @returns {number} The current time in milliseconds.
 */
export function useTicker() {
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);

  return now;
}
