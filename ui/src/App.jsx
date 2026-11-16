// The page shell: header, the grid of cards, and the states for loading,
// failure and nothing-to-show.

import { formatAge } from "./format.js";
import { useNodes, useTheme, useTicker } from "./hooks.js";
import { NodeCard } from "./NodeCard.jsx";

/**
 * The fleet mark: three stacked bars, so the logo needs no image file.
 *
 * @returns {JSX.Element} An inline SVG.
 */
function Mark() {
  return (
    <svg width="18" height="18" viewBox="0 0 18 18" aria-hidden="true">
      <rect x="1" y="2" width="16" height="4" rx="1.5" fill="currentColor" />
      <rect
        x="1"
        y="7.5"
        width="16"
        height="4"
        rx="1.5"
        fill="currentColor"
        opacity=".6"
      />
      <rect
        x="1"
        y="13"
        width="16"
        height="4"
        rx="1.5"
        fill="currentColor"
        opacity=".3"
      />
    </svg>
  );
}

/**
 * The light and dark toggle, showing the option you would switch to.
 *
 * @param {object} props
 * @param {string} props.theme The theme in use.
 * @param {function} props.onFlip Switches to the other theme.
 * @returns {JSX.Element} The button.
 */
function ThemeButton({ theme, onFlip }) {
  return (
    <button
      className="theme"
      type="button"
      onClick={onFlip}
      aria-label={`switch to the ${theme === "light" ? "dark" : "light"} theme`}
    >
      {theme === "light" ? (
        <svg
          width="14"
          height="14"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          aria-hidden="true"
        >
          <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
        </svg>
      ) : (
        <svg
          width="14"
          height="14"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          aria-hidden="true"
        >
          <circle cx="12" cy="12" r="4" />
          <path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" />
        </svg>
      )}
    </button>
  );
}

/**
 * The tally of how many nodes exist and how many are talking.
 *
 * @param {object} props
 * @param {number} props.total Every known node.
 * @param {number} props.stale How many have gone quiet.
 * @returns {JSX.Element} Three pills.
 */
function Tally({ total, stale }) {
  const up = total - stale;
  return (
    <ul className="tally">
      <li className="all">
        <span className="dot" />
        <b>{total}</b>
        {total === 1 ? "node" : "nodes"}
      </li>
      <li className="up">
        <span className="dot" />
        <b>{up}</b>up
      </li>
      <li className="stale">
        <span className="dot" />
        <b>{stale}</b>stale
      </li>
    </ul>
  );
}

/**
 * Shown before the first node reports, with the command that starts one.
 *
 * @returns {JSX.Element} A small terminal block.
 */
function EmptyState() {
  return (
    <div className="term">
      <div className="term-bar">
        <span className="term-dot r" />
        <span className="term-dot y" />
        <span className="term-dot g" />
        <span className="term-label">fleet</span>
      </div>
      <pre>
        <span className="tp">$</span> fleet node -config node.toml{"\n"}
        <span className="tc">no nodes have reported yet</span>
        <span className="cursor" />
      </pre>
    </div>
  );
}

/**
 * The whole page.
 *
 * @returns {JSX.Element} The app.
 */
export function App() {
  const { nodes, clock, error, loading, updatedAt, refresh } = useNodes();
  const [theme, flip] = useTheme();
  const now = useTicker();

  const stale = nodes.filter((node) => node.stale).length;
  // The browser measured this itself when the poll returned, so it is
  // compared against the browser clock rather than the master's.
  const updatedSeconds = updatedAt ? (now - updatedAt) / 1000 : Number.NaN;

  return (
    <>
      <nav className="nav">
        <div className="wrap nav-inner">
          <a className="brand" href="/">
            <Mark />
            fleet
          </a>
          <div className="nav-right">
            <button
              className="refresh"
              type="button"
              onClick={refresh}
              disabled={loading}
            >
              {loading ? "loading" : "refresh"}
            </button>
            <ThemeButton theme={theme} onFlip={flip} />
          </div>
        </div>
      </nav>

      <header className="wrap bar">
        <div className="bar-left">
          <span className="label">status</span>
          {nodes.length > 0 && <Tally total={nodes.length} stale={stale} />}
        </div>
        {updatedAt > 0 && (
          <span className="stamp">updated {formatAge(updatedSeconds)}</span>
        )}
      </header>

      <main className="wrap">
        {error && (
          <p className="notice" role="status">
            {error}
            {nodes.length > 0 ? ", showing the last good snapshot" : ""}
          </p>
        )}

        {nodes.length > 0 ? (
          <div className="nodes">
            {nodes.map((node) => (
              <NodeCard
                key={node.report?.hostname || "unknown"}
                node={node}
                clock={clock}
                now={now}
              />
            ))}
          </div>
        ) : (
          !loading && !error && <EmptyState />
        )}
      </main>

      <footer className="wrap foot">
        <span>fleet</span>
        <span>{nodes.length} known</span>
      </footer>
    </>
  );
}
