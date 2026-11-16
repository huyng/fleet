// The card for one node, and the small pieces it is made of.

import {
  formatAge,
  formatBytes,
  formatPercent,
  formatUptime,
  meterBand,
  usedPercent,
} from "./format.js";
import { secondsSince } from "./clock.js";

/**
 * A usage bar with the figures beside it.
 *
 * @param {object} props
 * @param {{used: number, total: number}} props.usage Bytes used and total.
 * @param {string} props.label What the bar measures, for screen readers.
 * @returns {JSX.Element} The bar, or a note that nothing was reported.
 */
export function Meter({ usage, label }) {
  if (!usage?.total) {
    return <span className="unknown">not reported</span>;
  }

  const percent = usedPercent(usage);
  const band = meterBand(usage);

  return (
    <div className="meter">
      <div
        className="meter-track"
        role="img"
        aria-label={`${label} ${formatPercent(percent)} used`}
      >
        <div
          className={band ? `meter-fill ${band}` : "meter-fill"}
          style={{ width: `${percent}%` }}
        />
      </div>
      <span className="meter-value">
        <b>{formatBytes(usage.used)}</b> of {formatBytes(usage.total)}
      </span>
    </div>
  );
}

/**
 * One labelled row of figures.
 *
 * @param {object} props
 * @param {string} props.label Short label, shown in the left column.
 * @param {React.ReactNode} props.children The figures.
 * @returns {JSX.Element} The row.
 */
function Row({ label, children }) {
  return (
    <div className="row">
      <dt>{label}</dt>
      <dd>{children}</dd>
    </div>
  );
}

/**
 * The running programs of a node, folded away until asked for.
 *
 * The list is only as fresh as the last time a master asked for it, so the
 * header says when that was.
 *
 * @param {object} props
 * @param {Array<object>} props.procs The process list.
 * @param {number} props.ageSeconds How long ago it was collected.
 * @returns {JSX.Element} The foldable table.
 */
function ProcessList({ procs, ageSeconds }) {
  return (
    <details className="procs">
      <summary>
        {procs.length} {procs.length === 1 ? "program" : "programs"}
      </summary>
      <div className="cb">
        <div className="cb-head">
          <span>programs</span>
          <span>{formatAge(ageSeconds)}</span>
        </div>
        <div className="proc-scroll">
          <table>
            <thead>
              <tr>
                <th scope="col" className="num">pid</th>
                <th scope="col">user</th>
                <th scope="col" className="num">cpu</th>
                <th scope="col" className="num">mem</th>
                <th scope="col">command</th>
              </tr>
            </thead>
            <tbody>
              {procs.map((proc) => (
                <tr key={proc.pid}>
                  <td className="num">{proc.pid}</td>
                  <td>{proc.user}</td>
                  <td className="num">{formatPercent(proc.cpu_percent)}</td>
                  <td className="num">{formatPercent(proc.mem_percent)}</td>
                  <td className="cmd">{proc.command}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </details>
  );
}

/**
 * Everything known about one node.
 *
 * @param {object} props
 * @param {object} props.node One entry from the snapshot endpoint.
 * @param {object} props.clock The master's clock, from serverClock.
 * @param {number} props.now Current browser time in milliseconds.
 * @returns {JSX.Element} The card.
 */
export function NodeCard({ node, clock, now }) {
  const report = node.report ?? {};
  const procs = Array.isArray(node.procs) ? node.procs : [];
  const seenSeconds = secondsSince(node.last_seen, clock, now);
  const procsSeconds = secondsSince(node.procs_at, clock, now);

  return (
    <article className={node.stale ? "node is-stale" : "node"}>
      <div className="node-head">
        <div>
          <h2 className="node-name">{report.hostname || "unnamed"}</h2>
          <div className="chips">
            <span className="chip">{report.ip || "no address"}</span>
            {report.agent_version && (
              <span className="chip">v{report.agent_version}</span>
            )}
          </div>
        </div>
        {node.stale ? (
          <span className="pill stale">
            <span className="dot" />
            stale
          </span>
        ) : (
          <span className="pill up">
            <span className="dot" />
            up
          </span>
        )}
      </div>

      <dl className="rows">
        <Row label="uptime">
          {report.uptime_seconds ? (
            formatUptime(report.uptime_seconds)
          ) : (
            <span className="unknown">not reported</span>
          )}
        </Row>

        <Row label="seen">{formatAge(seenSeconds)}</Row>

        <Row label="load">
          <span className="loads">
            <span>
              <b>{(report.load?.["1m"] ?? 0).toFixed(2)}</b>
              <i>1m</i>
            </span>
            <span>
              <b>{(report.load?.["5m"] ?? 0).toFixed(2)}</b>
              <i>5m</i>
            </span>
            <span>
              <b>{(report.load?.["15m"] ?? 0).toFixed(2)}</b>
              <i>15m</i>
            </span>
          </span>
        </Row>

        <Row label="memory">
          <Meter usage={report.memory} label="memory" />
        </Row>

        <Row label="disk">
          <Meter usage={report.disk} label="disk" />
        </Row>
      </dl>

      {procs.length > 0 && (
        <ProcessList procs={procs} ageSeconds={procsSeconds} />
      )}
    </article>
  );
}
