# GETTING_STARTED

## VPN Service

This tool currently supoorts [Clash Verge Rev](https://github.com/clash-verge-rev/clash-verge-rev/releases) only, so before using make sure you have clash verge rev installed in your system. For example, you are expected to get this result when clash verge rev is installed:

```bash
which clash-verge
/usr/bin/clash-verge
```
Also, make sure the [TUN](https://wiki.metacubex.one/en/config/inbound/tun/) sevice is available as this tool will use it to route traffic from processes. 

## Local warp proxy

Before using the tool, make sure a [Cloudflare WARP CLI](https://developers.cloudflare.com/warp-client/get-started/linux/) is installed:

```bash
which warp-cli
/usr/bin/warp-cli
```
Your [local WARP proxy](https://developers.cloudflare.com/cloudflare-one/team-and-resources/devices/cloudflare-one-client/configure/modes/#local-proxy-mode) `warp-svc` is expected to run on `port:40000` on your loopback ip.

```bash
warp-cli settings
Merged configuration:
(not set)	Compliance Environment: Normal
(derived)	Always On: false
(override)	Switch Locked: false
(user set)	Mode: WarpProxy on port 40000 # <
#....
```
And use [MASQUE](https://developers.cloudflare.com/warp-client/get-started/linux/#switch-tunnel-protocol) as warp tunnel protocol:

```bash
(consumer overrides)	WARP tunnel protocol: MASQUE
(not set)	MASQUE Protocol Settings:
  HTTP Version: MASQUE (HTTP/3 with HTTP/2 fallback)
(not set)	PKIX config: not set
(network policy)	Post-quantum support for MASQUE: Enabled (downgrades allowed)
```

If not, please refer to the [Cloudflare WARP Linux guide](https://developers.cloudflare.com/warp-client/get-started/linux/).

## Launch the service

```bash
agywarp
```
or in source code repo (requires [Go](https://go.dev/doc/install)):

```bash
go run .
```

## TUI

1. **Choose your airport and node in Clash Verge before starting routing.** Keep
   TUN and process matching enabled.

2. **Set up process groups.** Use `Tab` / `Shift+Tab` to focus the process list.
   Select a group with `↑` / `↓`, press `a` to add one, or `e` to edit it.
   In the editor, enter a group name and use `Tab` to reach the Add Path field.
   Enter an executable path or process name, such as `/usr/bin/firefox` or
   `firefox`, and press `Enter` to append it. Repeat for other processes in the
   group. Press `Ctrl+S` to save, or `Esc` to leave without saving.

3. **Enable the groups you want to route.** In the process list, select each
   group and use `Space` to toggle it ON or OFF as needed. Newly saved groups
   are enabled by default. Group ON selects routing intent; it does not start
   the service. At least one group must be enabled.

4. **Start routing.** Focus Network Card with `Tab`. While the service is OFF
   and idle, press `p` to choose SOCKS5 (default) or HTTP CONNECT. HTTP CONNECT
   supports TCP targets only. Press `Space` to start and wait for Service to
   show ON. If startup fails, read the error in Output.

5. **Use the selected applications and view output.** Network Card shows the
   node and Source; press `r` to refresh. Focus Output with `Tab` to see its
   shortcuts in the footer. Use `PgUp` / `PgDn` to scroll, `End` to follow the
   latest logs, and `o` to expand or collapse Output.

6. **Stop routing before making changes.** Focus Network Card and press
   `Space` to turn the service OFF. Wait for completion before editing groups,
   changing the proxy mode, or switching the airport or node in Clash Verge.
   Keep the dashboard open while ON. Press `q` to quit after stopping.
