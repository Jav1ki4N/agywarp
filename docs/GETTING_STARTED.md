# GETTING_STARTED

## VPN Service

This tool currently supoorts clash verge rev only, so before using make sure you have clash verge rev installed in your system.

For example, you are expected to get this result when clash verge rev is installed:

```bash
which clash-verge
/usr/bin/clash-verge
```

## Local proxy mode

Focus the Network Card with `Tab` and press `p` to switch between **SOCKS5**
(default) and **HTTP CONNECT**. Switching is locked while the service is ON,
starting/stopping, or refreshing. Press `space` to start or stop routing with
the selected mode. No automatic fallback occurs.

Both modes use the existing WARP local proxy port; no additional daemon or
listener is created. Mihomo's temporary proxy uses the selected protocol.
The preference is saved atomically to
`~/.config/agywarp/settings.json` (or `$XDG_CONFIG_HOME/agywarp/settings.json`).
Reopening an active session uses its recorded mode; older sessions use SOCKS5.

HTTP CONNECT carries TCP target connections, not application UDP traffic.
The outer WARP MASQUE tunnel still uses UDP through the selected Mihomo route.
Changing the local protocol does not guarantee that target DNS or WARP tunnel
failures will disappear. HTTP CONNECT support requires a compatible WARP client;
see the [Cloudflare Linux release notes](https://developers.cloudflare.com/changelog/post/2025-10-07-warp-linux-ga/).

## WARP outer route

The Output panel wraps long diagnostic lines. Press `o` to expand it to the
full dashboard area, then `o` again to return. Use `PgUp` / `PgDn` to review
older output and `End` to resume following the latest logs. These keys also
work in the normal dashboard when no editor or process picker is open.
Expanded output keeps the footer and pauses dashboard action keys; background
monitoring continues.

The Network Card shows the node, Mihomo proxy provider, and current Clash
Verge airport profile. A proxy provider is Mihomo's named proxy collection;
it is separate from the airport profile. Inline nodes may have no provider.
The section refreshes every two seconds while the dashboard is idle, and
when you press `r`.
