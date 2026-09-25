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



**agywarp** is a TUI tool that routes selected processes through 
[**Cloudflare WARP**](https://developers.cloudflare.com/warp-client/get-started/linux/)'s local proxy and 
[**Clash Verge Rev**](https://github.com/clash-verge-rev/clash-verge-rev)'s [**Mihomo**](https://github.com/MetaCubeX/mihomo) core. It loads temporary 
routing rules into Mihomo, verifies the WARP exit before activating 
application rules, and restores the generated base config when
the tunnel is stopped.

### Why

This tool is originally built to proxy [**Google Antigravity**](https://antigravity.google/) CLI / [**VS Code**](https://code.visualstudio.com/) Extension
through CloudFlare warp service, as common proxy services' IP can be easily blocked 
by Google for being an IDC IP.  

Process-specific WARP routing requires Mihomo rules and the WARP daemon to
work together. agywarp provides one control to verify the connection, load
the selected process rules, and remove the runtime routing when stopped.

- **Process routing**: Route enabled process groups through WARP using Mihomo.
- **Verified WARP exit**: Check the WARP connection before activating those rules.
- **Reversible setup**: Remove runtime rules and restore the generated base config on stop.

## Getting started

### Requirements

- A x86 machine running a Linux distro.
- Clash Verge Rev with Mihomo running, [**TUN**](https://wiki.metacubex.one/en/config/inbound/tun/) enabled, and process matching
  enabled. The current adapter expects its generated `clash-verge.yaml` and
  controller socket at `/tmp/verge/verge-mihomo.sock`.
- [**`warp-cli`**](https://developers.cloudflare.com/warp-client/get-started/linux/) and its daemon.

### Launch service

Please see [GETTIN_STARTED.md](./docs/GETTING_STARTED.md) for details.

## License

This project is licensed under the [MIT License](../LICENSE).

## Credits

<div align = "center">
    <img src = "assets/go-gopher.png" width=75 alt="go-gopher">
</div>






