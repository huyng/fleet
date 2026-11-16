# fleet

Monitor and control a group of Linux and macOS machines from one command line
tool.

One binary, two roles:

- **master** — collects status reports from the other machines and serves a
  web page showing the whole group.
- **node** — reports its status to the master and carries out the commands the
  master sends.

A node only ever connects out to the master, so a fleet works across networks
where the master cannot reach the nodes.

## Install

```
curl -fsSL https://raw.githubusercontent.com/huyng/fleet/main/install.sh | sh
```

That picks the tarball for your machine, checks it against the release
checksums, and puts `fleet` on PATH. If `/usr/local/bin` is not writable it
installs to `~/.local/bin` and tells you to add that to PATH.

To control where it goes, or the version, download the script first and run it
with settings:

```
curl -fsSL -O https://raw.githubusercontent.com/huyng/fleet/main/install.sh
sh install.sh                      # latest release
FLEET_BIN_DIR=~/bin sh install.sh  # somewhere else
FLEET_VERSION=0.1.0 sh install.sh  # a specific release
```

The script covers Linux and macOS on amd64 and arm64.

If you have Go, the usual way works too, because the built page is committed
and needs no Node:

```
go install github.com/huyng/fleet/cmd/fleet@latest
```

That puts the binary in `$(go env GOPATH)/bin`.

## Quick start

Build from source (needs Go and Node, see [Building](#building)):

```
make
```

Then in one terminal, on the machine that watches the group:

```
./fleet master -config examples/master.toml
```

And in another, on each machine in the group:

```
./fleet node -config examples/node.toml
```

Both examples ship with `secret = "change-me"` and listen on localhost, so
running both on one machine works with no edits. Open
`http://127.0.0.1:8080/` for the status page.

To watch other machines, change `listen` in the master config to an address
they can reach and `master` in the node config to match, and pick a real
secret:

```
head -c 32 /dev/urandom | base64
```

## Build

```
make
```

That installs the page dependencies if they are missing, builds the page, then
builds `./fleet` with the page embedded. The result is a static binary you can
copy to any machine of the same architecture.

```
make help        list every target
make test        Go and frontend tests
make check       formatting, vet and both test suites
make release     tarballs and checksums for every platform, in dist/
```

Without make, the same two steps by hand:

```
cd ui && npm ci && npm run build
cd .. && go build ./cmd/fleet
```

The built page in `internal/web/dist` is committed, so `go build ./cmd/fleet`
alone works on a fresh checkout. Rerun the frontend build after changing
anything under `ui/src`.

## Run

Both roles read one TOML file. `examples/master.toml` and
`examples/node.toml` document every setting.

```
fleet master -config master.toml
fleet node -config node.toml
fleet version
```

Ctrl-C stops either role cleanly.

## What the page shows

A bar across the top shows how many nodes there are and how many are up or
stale, and when the page last refreshed. Under it, one card per node: an up or
stale badge, the address and agent version, uptime, when it last reported, the
three load averages, and memory and disk as bars with the figures beside them.
Bars turn amber past 70% and red past 90%. Last known programs sit at the
bottom of a card, folded away until clicked, busiest first.

The page is a small React app. It refreshes itself every five seconds, so
nothing is stale just because you left a tab open, and it keeps the last good
data on screen if a refresh fails. It follows the operating system's light or
dark setting, remembers a manual choice, and folds down to a single column on
a narrow screen.

There is no login: the page and the snapshot it reads are open to anyone who
can reach the port. Bind the master to a trusted address or put it behind a
proxy.

## Commands

The page is read only, so these go through the API. Reboot a node:

```
curl -X POST -H 'X-Fleet-Secret: $SECRET' http://master:8080/api/v1/nodes/web-1/reboot
```

Ask a node for its list of running programs, then read the list back:

```
curl -X POST -H "X-Fleet-Secret: $SECRET" http://master:8080/api/v1/nodes/web-1/procs
curl      -H "X-Fleet-Secret: $SECRET" http://master:8080/api/v1/nodes/web-1/procs
```

A node picks commands up on its next report, so allow one interval.

## API

Every `/api` request needs the `X-Fleet-Secret` header.

| Method and path | What it does |
| --- | --- |
| `POST /api/v1/report` | node sends its status |
| `GET /api/v1/nodes` | every known node |
| `GET /api/v1/nodes/{host}` | one node |
| `POST /api/v1/nodes/{host}/reboot` | queue a reboot |
| `POST /api/v1/nodes/{host}/procs` | queue a program list collection |
| `PUT /api/v1/nodes/{host}/procs` | node sends its program list |
| `GET /api/v1/nodes/{host}/procs` | last program list |
| `GET /api/v1/commands?hostname={host}` | node collects its queued commands |

A status report looks like this:

```json
{
  "hostname": "web-1",
  "ip": "10.0.0.12",
  "uptime_seconds": 86400,
  "load": {"1m": 0.4, "5m": 0.5, "15m": 0.6},
  "memory": {"used": 2147483648, "total": 8589934592},
  "disk": {"used": 107374182400, "total": 536870912000},
  "agent_version": "0.1.0",
  "reported_at": "2025-11-16T03:30:00Z"
}
```

Two endpoints serve the page and take no secret, because a browser cannot keep
one:

| Method and path | What it does |
| --- | --- |
| `GET /` | the page itself; every other non-API path serves it too, so the app owns its own routes |
| `GET /app.js`, `/app.css`, `/favicon.svg` | the built bundle and tab icon |
| `GET /ui/api/nodes` | the snapshot the page reads, including the master's clock |

The snapshot carries `server_time`, so the page can work out how long ago a
node reported without trusting the browser's clock.

## When a node says a report failed

The node logs the method and full URL it called, so the message points at the
cause:

```
fleet node: report failed: reading boot time: unexpected EOF
fleet node: polling for commands failed: Get "http://127.0.0.1:8080/api/v1/commands?hostname=starlight.local":
    dial tcp 127.0.0.1:8080: connect: connection refused
    nothing is listening at that address: start the master, or check the address
    and port in the node config
```

- **`connection refused`** means nothing is listening on that address. Start
  the master, or fix the port in the node config. The node keeps retrying, so
  it joins the group by itself once the master is up.
- **`unexpected EOF`, or `no such file or directory` on `/proc/...`** means the
  node binary is older than the machine it runs on. Linux reads `/proc`;
  macOS reads the same figures from `sysctl` and `vm_stat`. Rebuild with
  `make`.
- **A 404 on `/api/v1/report`** means the address is not a fleet master at
  all. Check the `master` line in the node config: scheme, address and port,
  with no path on the end.
- **A node that has never reported** is shown as unknown rather than as an
  error. Asking for its commands returns an empty list.

The master logs one line per request, which shows whether the node's calls
arrive at all:

```
fleet master: 2026/11/16 03:45:38 POST /api/v1/report 204 in 0s
fleet master: 2026/11/16 03:45:38 GET /api/v1/commands?hostname=web-1 200 in 0s
```

If a node's report never appears in the master log, the node is talking to
something else. If the master log has no `listening on` line at all, it never
started, usually because the port was already taken.

## Development

While working on the page, rebuild it on every save in one terminal:

```
cd ui && npm run watch
```

and run `make` after it to pick the bundle up. The master embeds the page, so
a rebuilt bundle does nothing until the binary is rebuilt and the master
restarted.

Layout:

```
install.sh          the installer served to users
Makefile            build, test and check targets
release.mk          packaging for a release (also included by Makefile)
cmd/fleet/          CLI: flags, subcommand dispatch, main()
internal/config/    TOML loading and validation
internal/status/    status gathering, plus the wire format
internal/procs/     the running program list
internal/protocol/  command names and the secret header
internal/agent/     node side: report loop, HTTP client, commands
internal/master/    node registry, command queue, API, UI snapshot
internal/web/       serves the built page from internal/web/dist
ui/                 the React app: src/, public/ (favicon), tests
```

Platform specific code sits behind build tags: `status/system_linux.go` reads
`/proc`, `status/system_darwin.go` reads `sysctl` and `vm_stat`,
`status/system_other.go` reports that the platform is not supported yet, and
`procs/args_<os>.go` holds the `ps` columns.

## Releasing

Bump `Version` in `internal/version/version.go`, rebuild the page if it
changed, commit, then tag and push:

```
git tag v0.1.0
git push origin v0.1.0
```

`.github/workflows/release.yml` builds the four tarballs, checks that the tag
matches the version in the source, and attaches everything plus `install.sh` to
the GitHub release. `make release` produces the same files locally in `dist/`,
including `checksums.txt`.

The page bundle in `internal/web/dist` is committed, so the release job needs
only Go. If you changed anything under `ui/src`, run `make` and commit the
rebuilt bundle before tagging, or the release will carry the previous page.

## Tests

`go test ./...` covers the Go side, `cd ui && npm test` covers the page, and
`make check` runs both plus formatting and vet. There are also two browser
scripts that need a running master:

```
make audit       URL=http://127.0.0.1:8080/
make screenshots URL=http://127.0.0.1:8080/ OUT=/tmp/shots
```

`audit` measures the page for overflow, clipping, overlapping cards and
contrast, and exits non-zero on a problem. `screenshots` writes dark and light
shots at desktop and mobile widths. Both drive a headless Chromium that
Playwright fetches (`cd ui && npx playwright install --only-shell chromium`);
see `AGENTS.md` for the extra libraries it needs on a bare Debian image.

## Limits worth knowing

- The master keeps state in memory only. Restart it and the node list is
  rebuilt as the nodes report again.
- A node only knows the master by URL, so the master never opens a connection
  to a node. A node behind a firewall works as long as it can reach the
  master.
- There is no TLS in the program. Put both sides behind a proxy or a private
  network if the secret would otherwise cross an untrusted link.
- Nothing on the page needs a login, and the snapshot endpoint is open. Treat
  the master's address as the boundary.
- Status figures come from `/proc` on Linux and from `sysctl` plus `vm_stat`
  on macOS. Those are the only two systems the node gathers on so far; on
  anything else the node logs that it cannot read the figures and keeps
  retrying.
- On macOS the node runs `sysctl` and `vm_stat`, so a node started by launchd
  must still find them. The usual absolute paths in `/usr/bin` and `/usr/sbin`
  are tried before `PATH`. The `procs` command also needs `ps`.
- Reboot on a Mac needs a `reboot_command` that exists there, for example
  `["sudo", "shutdown", "-r", "now"]`, since there is no systemd.
- The page is read only. Rebooting and collecting program lists go through the
  API with `curl` for now.
