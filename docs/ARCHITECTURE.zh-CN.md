# 架构

[English](ARCHITECTURE.md)

## 适用范围与组件

agywarp 是 Linux TUI：它让选定进程通过 Cloudflare WARP 的本地 SOCKS5 代理和
Clash Verge Rev 的 Mihomo 内核出站。`internal/process` 保存进程组并扫描进程；
`internal/clash` 构造临时 Mihomo 配置并调用控制接口；`internal/warp` 操作
`warp-cli`；`internal/checker` 验证出口；`internal/tui` 协调这些操作。

当前 Clash 适配器假定生成配置位于
`~/.local/share/io.github.clash-verge-rev.clash-verge-rev/clash-verge.yaml`，
控制套接字位于 `/tmp/verge/verge-mihomo.sock`。生成配置由 Clash Verge 管理；
其他 Mihomo 客户端需要不同的适配器。

## 磁盘写入与实时状态

| 对象 | ON／测试时 | OFF／恢复时 |
| --- | --- | --- |
| 生成的 `clash-verge.yaml` 和原始订阅 | 只读，agywarp 不改写。 | 只读；Clash Verge 可能自行重新生成基础配置。 |
| Mihomo 实时配置 | 通过 `PUT /configs?force=true` 加载内联配置。 | 从当前生成的基础配置重载，并通过控制接口核验。 |
| `<Clash Verge 基础目录>/.agywarp/session.json` | 完整运行规则加载并核验后创建。 | OFF 成功后删除；`recover` 可删除过期会话。 |
| 机场规则／代理扩展 | 动态注入期间不写入。 | OFF 仅在发现旧版 agywarp WARP 条目时定向清理；`recover` 不清理扩展。 |
| `~/.config/agywarp/profiles.json` | 读取已启用进程组。 | ON／OFF 不修改；创建、编辑进程组或首次初始化默认组时保存。 |
| WARP 守护进程 | ON 或手动测试前状态不是 CONNECTED 时，agywarp 才调用 Connect。 | 只有 agywarp 主动连接过，OFF 才断开；`recover` 不改变连接状态。 |

会话文件先写入临时文件、同步后原子重命名。清理旧扩展也使用临时文件和重命名。
强制终止可能留下 `session-*.tmp` 或 `.agywarp-clean-*.tmp`；它们不会作为
Mihomo 规则加载。删除会话后，空的 `.agywarp` 目录可能保留。

## 预检与进程组

打开仪表盘前，只读预检会解析进程组 JSON，并检查生成的基础配置、控制接口、
TUN、进程匹配和 WARP 的 WarpProxy 模式。基础配置不得持久化 `warp-svc` 规则、
目标为 `AGYWARP-WARP`／`WARP-LOCAL` 的规则，或使用这两个保留名称的代理。
预检还会对比实时应用规则和会话；即使没有应用规则或会话，也能识别遗留的
临时 guard／代理。它检查当前生成的基础配置，不遍历每份未启用订阅。

已启用进程组只编译为 `PROCESS-NAME` 和 `PROCESS-PATH` 规则。旧的域名匹配器
不会注入运行时，因为它可能影响其他应用。隧道处于 ON 或状态切换中时，
进程组编辑被锁定。进程组“已启用”不表示对应进程当前正在运行；进程扫描
仅用于显示。

## ON、测试与 OFF

ON 首先加载在内存中构造的引导配置：加入名为 `AGYWARP-WARP` 的本地 SOCKS5
代理和 `PROCESS-NAME,warp-svc,...` guard，此时尚无应用规则。guard 指向生成
配置中最后一条 `MATCH` 规则的目标代理或组；若没有 `MATCH`，使用 `DIRECT`。
目标是 `REJECT`、`REJECT-DROP`、`WARP-LOCAL` 或 `AGYWARP-WARP` 时会拒绝
启动。这些修改仅存在于 Mihomo 实时配置，不会在磁盘上固定某个订阅节点。

若 WARP 原先不是 CONNECTED，agywarp 会调用 Connect，等待本地代理就绪，
并验证代理出口确实经过 WARP。之后加载包含已启用进程规则的完整内存配置。
只有确认规则已在 Mihomo 中生效，才写入 `session.json`。会话记录基础配置
路径及 SHA-256、预期规则、WARP 是否由 agywarp 连接、当前机场 UID 和手动
选择组的选项。启动失败会尝试重载基础配置并恢复 WARP 原有连接状态。

聚焦 Network Card 且隧道为 OFF 时，按 `t` 可沿同一外层路由进行临时连接
测试。测试结束会恢复基础配置和 WARP 原有连接状态，不创建运行会话。
按空格可开启或关闭隧道。

OFF 需要会话文件。它核对基础配置路径；若基础配置已改变，且新配置含持久化
WARP 路由规则，则拒绝自动恢复。它会清除 `profiles.yaml` 引用的机场规则扩展
中已知的旧版 WARP 规则；仅当 `WARP-LOCAL` 是 `127.0.0.1:40000` 的 SOCKS5
代理时删除该代理。它不扫描原始订阅、合并扩展或脚本。随后 OFF 将当前基础
配置加载到 Mihomo，确认实时 WARP 规则、guard 和代理均已消失，再删除
会话；只有 agywarp 原先连接了 WARP，才会断开它。任一步骤失败都会报错。

## 切换机场与节点

**先关闭隧道，再到 Clash Verge 切换机场或手动选择的节点。** 隧道 ON 时，
仪表盘阻止正常的 `q` 和 `Ctrl+C` 退出。仪表盘保持打开时，每两秒对比生成
配置哈希、机场 UID 和 Mihomo `Selector` 组的选项；发现变化后尝试 OFF 并
报告结果。Clash Verge 是独立应用，agywarp 无法在它切换前拦截。程序崩溃或
被强制终止后不再监测；非 `Selector` 组的自动节点变化不在此检查范围内。

仪表盘无法使用但会话仍在时，可运行 `agywarp stop` 执行 OFF。若 OFF 报错，
应先检查错误原因，再继续调整路由。

## 中断恢复与清理核验

引导阶段中断，可能只留下实时 `warp-svc` guard 或 `AGYWARP-WARP` 代理，
没有应用规则与会话。预检会识别这种孤立状态。Clash Verge 在外部重载也可能
清除实时规则，却留下过期会话。允许恢复时，`agywarp recover` 会重载干净的
生成配置并删除过期会话；它不会改变 WARP 连接状态。若会话仍处于活动状态、
会话存在但基础配置哈希已变化，或基础配置含运行时应用规则，它会拒绝执行。
活动会话应使用 `agywarp stop`。

确认 OFF 干净，不能只看一个文件，应同时核对：

1. `session.json` 不存在。
2. Mihomo 实时 `/rules` 没有 `warp-svc`、指向 `AGYWARP-WARP` 或
   `WARP-LOCAL` 的规则；实时 `/proxies` 也没有这两个保留代理。
3. 生成的基础配置和相关机场扩展没有持久化的 agywarp WARP 条目。
4. 若连接原由 agywarp 建立，WARP 已恢复到之前的连接状态。

程序在 OFF 和预检期间核验实时残留；检查所有未启用机场文件及遗留临时文件
仍需单独检查磁盘。TUN 已启用、WARP 出口验证成功，也不能单独证明每个
选定应用的流量都进入了 Mihomo。

## TUI 结构

`internal/tui/model.go` 将消息转交给当前页面；
`internal/tui/pages/home.go` 协调命令与状态切换；
`internal/tui/components` 绘制进程列表、Network Card、流量图、控制台和
输入控件；`internal/tui/preflight.go` 在进入终端备用屏幕前运行。
