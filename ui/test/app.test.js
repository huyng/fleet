// Tests that the app actually renders, using jsdom as a stand-in browser.
//
// These cover what a pure helper test cannot: that the components mount
// without error, that a snapshot turns into cards, that a failed poll keeps
// the last good snapshot on screen, and that a node with missing figures says
// so instead of showing zeros.
//
// One jsdom instance is shared by the whole file. Creating one per test left
// React holding references to a window that had already been thrown away.

import test, { after, before } from "node:test";
import assert from "node:assert/strict";

import { JSDOM } from "jsdom";

import { interact, mount } from "./.build/harness.js";

// A snapshot shaped exactly like the master's /ui/api/nodes response.
const SNAPSHOT = {
  server_time: "2025-11-16T04:00:00Z",
  nodes: [
    {
      report: {
        hostname: "web-1",
        ip: "10.0.0.12",
        uptime_seconds: 90061,
        load: { "1m": 0.4, "5m": 0.5, "15m": 0.6 },
        memory: { used: 2 * 1024 ** 3, total: 8 * 1024 ** 3 },
        disk: { used: 95 * 1024 ** 3, total: 100 * 1024 ** 3 },
        agent_version: "0.1.0",
      },
      last_seen: "2025-11-16T03:59:48Z",
      stale: false,
      procs: [
        {
          pid: 421,
          user: "postgres",
          cpu_percent: 12.5,
          mem_percent: 2.4,
          command: "postgres",
        },
      ],
      procs_at: "2025-11-16T03:59:30Z",
    },
    {
      report: { hostname: "old-laptop", ip: "10.0.0.99", agent_version: "0.0.9" },
      last_seen: "2025-11-16T03:40:00Z",
      stale: true,
      procs: [],
      procs_at: "0001-01-01T00:00:00Z",
    },
  ],
};

// The shared browser-like environment.
let dom;
let saved;

before(() => {
  dom = new JSDOM('<!DOCTYPE html><html><body></body></html>', {
    url: "http://fleet.test/",
    pretendToBeVisual: false,
  });

  const globals = {
    window: dom.window,
    document: dom.window.document,
    navigator: dom.window.navigator,
    localStorage: dom.window.localStorage,
    HTMLElement: dom.window.HTMLElement,
    Node: dom.window.Node,
    Event: dom.window.Event,
    IS_REACT_ACT_ENVIRONMENT: true,
    // The app polls on a timer and ticks once a second. Tests drive updates
    // themselves, so nothing here should run on its own.
    setInterval: () => 0,
    clearInterval: () => {},
  };

  saved = new Map();
  for (const [name, value] of Object.entries(globals)) {
    // Some globals, such as navigator, are getter-only in node 22, so they
    // have to be redefined rather than assigned.
    saved.set(name, Object.getOwnPropertyDescriptor(globalThis, name));
    Object.defineProperty(globalThis, name, {
      value,
      configurable: true,
      writable: true,
    });
  }
});

after(() => {
  for (const [name, descriptor] of saved) {
    if (descriptor) {
      Object.defineProperty(globalThis, name, descriptor);
    } else {
      delete globalThis[name];
    }
  }
  dom.window.close();
});

/**
 * Replace fetch with a stub that answers with the given body.
 *
 * @param {object} body JSON body to return.
 * @param {number} status HTTP status, 200 unless given.
 */
function stubFetch(body, status = 200) {
  globalThis.fetch = async () => ({
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  });
}

/**
 * Mount the app into a fresh container and wait for its first paint.
 *
 * @returns {Promise<{container: Element, app: object}>} The mounted app.
 */
async function renderApp() {
  const container = dom.window.document.createElement("div");
  dom.window.document.body.replaceChildren(container);
  const app = await mount(container);
  return { container, app };
}

/**
 * Wait until the page contains the given text, or fail.
 *
 * @param {string} text Text to wait for.
 * @param {number} timeoutMs How long to wait before failing.
 */
async function waitForText(text, timeoutMs = 2000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (dom.window.document.body.textContent.includes(text)) {
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  assert.fail(
    `page never showed "${text}". Body was:\n${dom.window.document.body.textContent}`,
  );
}

test("renders a card for each node", async () => {
  stubFetch(SNAPSHOT);
  const { app } = await renderApp();

  try {
    await waitForText("web-1");

    assert.equal(dom.window.document.querySelectorAll("article.node").length, 2);

    const body = dom.window.document.body.textContent;
    // Figures are formatted for humans, not dumped as raw numbers.
    assert.match(body, /1d 1h/);
    assert.match(body, /2\.0 GiB/);
    assert.match(body, /0\.40/);
    assert.match(body, /v0\.1\.0/);
  } finally {
    await app.unmount();
  }
});

test("marks a quiet node as stale", async () => {
  stubFetch(SNAPSHOT);
  const { app } = await renderApp();

  try {
    await waitForText("old-laptop");

    const stale = dom.window.document.querySelector("article.node.is-stale");
    assert.ok(stale, "no card was marked stale");
    assert.match(stale.textContent, /stale/);

    // A node that never sent figures says so rather than showing zeros.
    assert.match(stale.textContent, /not reported/);
    assert.doesNotMatch(stale.textContent, /0 MiB of 0 MiB/);
  } finally {
    await app.unmount();
  }
});

test("counts nodes in the tally", async () => {
  stubFetch(SNAPSHOT);
  const { app } = await renderApp();

  try {
    await waitForText("web-1");

    const tally = dom.window.document.querySelector(".tally").textContent;
    assert.match(tally, /2nodes/);
    assert.match(tally, /1up/);
    assert.match(tally, /1stale/);
  } finally {
    await app.unmount();
  }
});

test("a disk at 95% is in the critical band", async () => {
  stubFetch(SNAPSHOT);
  const { app } = await renderApp();

  try {
    await waitForText("web-1");

    const crit = dom.window.document.querySelector(".meter-fill.crit");
    assert.ok(crit, "no bar was in the critical band");
    // The width follows the figure: 95% of the track.
    assert.match(crit.getAttribute("style"), /width: 95%/);
  } finally {
    await app.unmount();
  }
});

test("shows the running programs when a node sent some", async () => {
  stubFetch(SNAPSHOT);
  const { app } = await renderApp();

  try {
    await waitForText("web-1");

    const details = dom.window.document.querySelector("details.procs");
    assert.ok(details, "no process section");
    assert.match(details.textContent, /1 program/);
    assert.match(details.textContent, /postgres/);
    assert.match(details.textContent, /421/);

    // Only the node that sent a list gets a section.
    assert.equal(
      dom.window.document.querySelectorAll("details.procs").length,
      1,
    );
  } finally {
    await app.unmount();
  }
});

test("says so when no nodes have reported", async () => {
  stubFetch({ server_time: "2025-11-16T04:00:00Z", nodes: [] });
  const { app } = await renderApp();

  try {
    await waitForText("no nodes have reported yet");
    assert.equal(
      dom.window.document.querySelectorAll("article.node").length,
      0,
    );
  } finally {
    await app.unmount();
  }
});

test("reports a failed poll without losing the page", async () => {
  // First poll succeeds, so there is something on screen to keep.
  stubFetch(SNAPSHOT);
  const { app } = await renderApp();
  await waitForText("web-1");

  try {
    // Then the master goes away and the refresh button is used.
    stubFetch({}, 500);
    await interact(async () => {
      dom.window.document.querySelector("button.refresh").click();
    });

    await waitForText("the master answered 500");

    // The nodes are still there, and the message says they are old.
    assert.equal(
      dom.window.document.querySelectorAll("article.node").length,
      2,
    );
    assert.match(dom.window.document.body.textContent, /last good snapshot/);
  } finally {
    await app.unmount();
  }
});

test("the theme button flips the palette and remembers it", async () => {
  stubFetch({ server_time: "2025-11-16T04:00:00Z", nodes: [] });
  const { app } = await renderApp();

  try {
    await waitForText("no nodes have reported yet");

    const before = dom.window.document.documentElement.classList.contains("light");
    await interact(async () => {
      dom.window.document.querySelector("button.theme").click();
    });
    const after = dom.window.document.documentElement.classList.contains("light");

    assert.notEqual(after, before, "the palette did not change");
    assert.equal(
      dom.window.localStorage.getItem("fleet_theme"),
      after ? "light" : "dark",
    );
  } finally {
    await app.unmount();
  }
});
