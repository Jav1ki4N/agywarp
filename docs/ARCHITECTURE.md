# Architecture

[简体中文](ARCHITECTURE.zh-CN.md)

## Scope and components

agywarp is a Linux TUI for routing selected processes through Cloudflare WARP's
local SOCKS5 or HTTP CONNECT proxy and Clash Verge Rev's Mihomo core. `internal/process` stores
process groups and scans running processes. `internal/clash` builds temporary
Mihomo configs and calls its controller. `internal/warp` operates `warp-cli`;
`internal/tui` coordinates the workflow.

The Clash adapter assumes a generated base config at
`~/.local/share/io.github.clash-verge-rev.clash-verge-rev/clash-verge.yaml`
and a controller socket at `/tmp/verge/verge-mihomo.sock`. Clash Verge owns the
generated file. Other Mihomo clients require a different adapter.

## Disk writes and live state

| Item | ON | OFF / recovery |
| --- | --- | --- |
| Generated `clash-verge.yaml` and raw subscriptions | Read only; agywarp does not rewrite them. | Read only; Clash Verge may regenerate the base independently. |
| Mihomo live config | Reloaded through `PUT /configs?force=true` with an inline payload. | Reloaded from the current generated base. |
| `<Clash Verge base>/.agywarp/session.json` | Created after full runtime rules are loaded. | Deleted after successful OFF; `recover` can remove a stale session. |
| Airport rule and proxy enhancements | Not written by dynamic injection. | OFF removes specific legacy agywarp WARP entries if present; `recover` does not clean them. |
| `~/.config/agywarp/profiles.json` | Read to compile enabled process groups. | Unchanged by ON/OFF. Creating or editing groups, or initializing defaults, saves it. |
| `~/.config/agywarp/settings.json` | Read for the local proxy protocol preference; `p` saves it atomically while OFF and idle. | Kept across OFF/recovery. Respects `XDG_CONFIG_HOME`. |
| WARP daemon | agywarp calls Connect only if it was not already CONNECTED before ON. | OFF disconnects only if agywarp connected it; `recover` leaves it unchanged. |

The session is written via a synced temporary file and atomic rename. Legacy
enhancement cleanup also uses a temporary file and rename. Forced termination
can leave `session-*.tmp` or `.agywarp-clean-*.tmp`; neither is loaded as a
Mihomo rule. The empty `.agywarp` directory may remain after OFF.

## Preflight and process groups

Read-only preflight runs before the dashboard opens. It parses the process
profile JSON and checks the generated base, controller, TUN, process matching,
and WARP WarpProxy mode. The base must not contain a persistent `warp-svc`
rule, a rule targeting `AGYWARP-WARP` or `WARP-LOCAL`, or a proxy with either
reserved name. Preflight compares live application rules with the session and
detects orphaned live guards/proxies even without application rules or a
session. It checks the active generated base, not every inactive subscription.

Enabled groups compile to `PROCESS-NAME` and `PROCESS-PATH` rules. Legacy
domain matchers are ignored for runtime injection because they could affect
other applications. Group editing is locked while the tunnel is ON or changing
state. An enabled group does not imply that its process is currently running;
process discovery is display data.

## ON and OFF

ON first loads a bootstrap config built in memory. It adds a local proxy using the selected SOCKS5 or HTTP protocol,
named `AGYWARP-WARP` and a `PROCESS-NAME,warp-svc,...` guard, but no
application rules yet. The guard targets the last `MATCH` rule's proxy or
group in the generated base; without `MATCH`, it uses `DIRECT`. Targets
`REJECT`, `REJECT-DROP`, `WARP-LOCAL`, and `AGYWARP-WARP` are rejected. This
does not pin a subscription node on disk.

If WARP was not already CONNECTED, agywarp calls Connect and waits for the local
proxy. It then loads the full in-memory config with enabled process rules and
creates `session.json`. The session records the base path and SHA-256
hash, expected rules, whether agywarp connected WARP, the active airport UID,
manual selector choices, and the local proxy protocol. A startup failure attempts to reload the base
and restore WARP's previous connection state.

Network Card `space` starts or stops the tunnel. `p` switches the local proxy
protocol while OFF and idle; it is locked during active operations. HTTP CONNECT
supports TCP targets only; it does not change the outer WARP UDP tunnel.
No automatic fallback is used. Active sessions keep their recorded protocol
on restart; older sessions default to SOCKS5.

The Network Card displays the node and a combined Source row: the current
airport name, with a distinct Mihomo proxy provider appended in parentheses.
Without an airport name, Source uses the provider name. Inline nodes may have
no provider. See
[Getting started](GETTING_STARTED.md#warp-outer-route).

OFF requires a session. It checks the base path; if the base changed, it
refuses to restore a changed base with persistent WARP routing rules. It
removes known legacy WARP rules from the rule enhancements referenced by
`profiles.yaml`, and removes a `WARP-LOCAL` SOCKS5 proxy only when its address
is `127.0.0.1:40000`. It does not scan raw subscriptions, merge enhancements,
or scripts. OFF then loads the current base into Mihomo, removes the runtime
WARP rules, guard, and proxies, deletes the session, and disconnects
WARP if agywarp originally connected it. A failed step reports an error.

## Switching airports and nodes

Turn the tunnel OFF **before** switching an airport or manually selected node
in Clash Verge. The dashboard blocks normal `q` and `Ctrl+C` exits while ON.
While open, it compares the generated base hash, airport UID, and Mihomo
`Selector` choices with the session every two seconds. On a change it
attempts OFF and reports the result. It cannot prevent a switch in the
separate Clash Verge application before that switch happens. Monitoring stops
after a crash or forced termination, and does not track automatic changes in
non-`Selector` groups.

If the dashboard is unavailable but a session is active, `agywarp stop`
performs OFF from the CLI. If OFF fails, inspect the reported condition before
changing more routing state.

## Interrupted runs and recovery

An interrupted bootstrap can leave a live `warp-svc` guard or
`AGYWARP-WARP` proxy without application rules or a session. Preflight detects
this orphaned state. An external Clash Verge reload can instead remove live
runtime rules while leaving a stale session. Where allowed, `agywarp recover`
reloads the clean generated base and removes a stale session; it leaves WARP's
connection state unchanged. It refuses an active session, a changed base hash
when a session exists, and a base containing runtime application rules. Use
`agywarp stop` for an active session.

## TUI structure

`internal/tui/model.go` delegates messages to the current page.
`internal/tui/pages/home.go` coordinates commands and state transitions.
`internal/tui/components` renders the process list, Network Card, traffic
chart, console, and input controls. `internal/tui/preflight.go` runs before
the alternate screen opens.
