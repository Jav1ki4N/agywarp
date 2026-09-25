<div align="center">
    <h1>agywarp</h1>
</div>

<div align="center">

[![License](https://img.shields.io/badge/License-MIT-blue.svg?style=flat-square)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.27+-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev/)
[![Release](https://img.shields.io/badge/Release-v0.1.0--dev-informational?style=flat-square)](https://github.com/)

</div>

## Intro

### What

**agywarp** is a Linux CLI and TUI that routes selected processes through
Cloudflare WARP's local SOCKS5 proxy and Clash Verge Rev's Mihomo core. It
loads temporary routing rules into Mihomo, verifies the WARP exit before
activating application rules, and restores the generated base config when
the tunnel is stopped.

### Why

Process-specific WARP routing requires Mihomo rules and the WARP daemon to
work together. agywarp provides one control to verify the connection, load
the selected process rules, and remove the runtime routing when stopped.

- **Process routing**: Route enabled process groups through WARP using Mihomo.
- **Verified WARP exit**: Check the WARP connection before activating those rules.
- **Reversible setup**: Remove runtime rules and restore the generated base config on stop.

## Requirements

- Linux and Go 1.27.1 or newer to build from source.
- Clash Verge Rev with Mihomo running, TUN enabled, and process matching
  enabled. The current adapter expects its generated `clash-verge.yaml` and
  controller socket at `/tmp/verge/verge-mihomo.sock`.
- `warp-cli` and its daemon, configured in **WarpProxy** mode with a valid
  local proxy port.
- A clean generated Clash Verge config: no persistent `warp-svc` rule,
  `AGYWARP-WARP` or `WARP-LOCAL` application rule, or proxy with either
  reserved name. agywarp reports the offending location during preflight.

WARP's outer connection follows the target of the generated config's last
`MATCH` rule. If there is no `MATCH`, the runtime guard uses `DIRECT`.
The selected route must be able to reach the WARP edge; agywarp verifies the
WARP exit before loading application rules.

## Run

From the repository root:

```bash
go run .
```

To build a local executable:

```bash
go build -o agywarp .
./agywarp
```

A read-only preflight runs before the dashboard opens. On first launch, the
dashboard creates default process groups if `~/.config/agywarp/profiles.json`
does not exist. Review which groups are enabled before starting the tunnel.
An enabled group defines a routing rule; the running count only reports
matching processes found on the system.

## Dashboard controls

Use `Tab` or `Shift+Tab` to switch between Process Profiles and Network Card.

| Focus | Key | Action |
| --- | --- | --- |
| Process Profiles | `↑`/`↓` or `k`/`j` | Select a group. |
| Process Profiles | `a`, `e`, `Space`, `d` | Add, edit, enable/disable, or delete a group. Editing is locked while the tunnel is ON or changing state. |
| Network Card | `t` | While OFF, temporarily test WARP through the current outer route, then restore the prior state. |
| Network Card | `Space` | Turn the tunnel ON or OFF. |
| Network Card | `r` | Refresh displayed network status. |
| Dashboard | `q` or `Ctrl+C` | Quit when the tunnel is OFF and no editor is open. |

ON first loads a temporary Mihomo bootstrap config, connects WARP if needed,
and verifies its local proxy and WARP exit. It then loads rules for enabled
process groups and records a session. OFF restores the current generated base
config, verifies that live WARP rules and proxies are gone, removes the
session, and disconnects WARP only if agywarp connected it.

**Turn OFF before switching airport subscriptions or manually selected nodes
in Clash Verge.** Keep the dashboard open while ON: it checks for those
changes and attempts to stop agywarp routing if one occurs. Clash Verge is a
separate application, so this check cannot prevent the switch itself or run
after agywarp is forcibly terminated. Normal quitting is blocked while ON.

## Files and recovery

Dynamic injection changes Mihomo's **live configuration**, not the generated
`clash-verge.yaml` or raw airport subscriptions. An active session is recorded
at `<Clash Verge base>/.agywarp/session.json`. The process groups are stored
separately in `~/.config/agywarp/profiles.json`. During OFF, agywarp may also
remove specific WARP entries left by older versions in referenced airport
rule and proxy enhancements; ordinary ON does not write those files.

If the dashboard is unavailable while a session is active, run:

```bash
go run . stop
```

For an interrupted bootstrap or a stale session detected by preflight, run:

```bash
go run . recover
```

`recover` reloads the clean generated base where allowed and does **not**
change WARP's connection state or clean old airport enhancements. It refuses
an active session; use `stop` for that case. If either command reports an
error, inspect it before changing more routing state. A clean OFF requires
both an absent session and no live WARP rules or proxies; disk files alone
do not establish the live state.

See the [architecture and recovery details](docs/ARCHITECTURE.md) for exact
state transitions, file writes, verification conditions, and limitations.

## Development

```bash
go test ./...
go vet ./...
```
