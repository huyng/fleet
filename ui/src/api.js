// The single call the page makes to the master.
//
// Kept apart from the components so the polling logic can be read on its own,
// and so a test can stub one function instead of the whole app.

// How often the page asks the master for a fresh snapshot, in milliseconds.
export const REFRESH_MS = 5000;

/**
 * Fetch the current node snapshot.
 *
 * @returns {Promise<{nodes: Array<object>, serverTime: string}>} What the
 *   master reported, with an empty list when it sent none.
 * @throws {Error} When the master cannot be reached or answers with a status
 *   other than 200.
 */
export async function fetchNodes() {
  const response = await fetch("/ui/api/nodes", {
    headers: { Accept: "application/json" },
  });

  if (!response.ok) {
    throw new Error(`the master answered ${response.status}`);
  }

  const body = await response.json();
  return {
    nodes: Array.isArray(body.nodes) ? body.nodes : [],
    serverTime: body.server_time || "",
  };
}
