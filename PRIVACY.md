# Mihomo TV privacy policy

Effective 25 November 2026. This policy covers the independently developed Mihomo TV client published by iori10500.

Mihomo TV routes IPv4 connections using a subscription you supply. It has no publisher-operated account, relay, advertising or analytics service. IPv6 traffic may reach destinations outside the VPN. The app inspects connection metadata, including the destination IP address and available host name, on the TV to apply your routing rules.

**On your TV.** The app stores the subscription address (which can include access credentials), cached node configurations, rule sources, routing preferences and recent direct or proxied status in its application data. The operating system manages VPN permission and interface configuration. A recent routing status can appear in device diagnostic logs. The app does not send a browsing history to the publisher.

**When data leaves the device.** Subscription and rule updates contact the providers configured by you. They receive the requested URL, including any access credentials for that source, and the source IP visible to them. A proxy node forwards connections routed through it and can observe connection metadata. Direct connections reach their destinations over your network. Third-party providers process what they receive under their own policies.

**DNS and tests.** The client configures the system VPN to use Cloudflare DNS at 1.1.1.1. A resolver can receive the domain being looked up. Automatic node selection and latency checks send requests through candidate nodes to Google's www.gstatic.com. A manual download test requests up to 4 MiB from speed.cloudflare.com. These services receive network data such as the IP from which the request reaches them.

**Your controls.** Disconnect from the home screen, change nodes or edit routing rules using the remote. Clear the app's storage in Fire TV settings to remove local configuration and cached credentials. Revoking a subscription or deleting records held by an external provider requires contacting that provider. For a private question about the app, use the developer support email shown in the Amazon Appstore listing. Please keep subscription links and keys out of public issues.

## 中文

本政策适用于 iori10500 独立开发的 Mihomo TV，自 2026 年 11 月 25 日起生效。

应用根据电视上连接的目标 IP 和可用域名匹配规则。系统隧道接管 IPv4，IPv6 可能从隧道外连接。订阅地址及其中的凭据、节点缓存、规则源、分流设置和最近的直连或代理状态保存在应用数据中。系统管理 VPN 权限与接口，设备诊断日志可能记录最近的分流状态。应用没有发布者运营的账号、节点、广告或分析服务，也不向发布者上传浏览历史。

订阅与规则的提供方会收到更新请求、该来源所需的凭据，以及所见的来源 IP。代理节点可观察经它转发的连接元数据。直连流量经用户网络到达目标。应用为系统 VPN 配置 Cloudflare DNS（1.1.1.1），解析服务可能收到查询域名。自动选点及延迟测试通过候选节点访问 Google 的 www.gstatic.com；手动下载测速向 speed.cloudflare.com 请求最多 4 MiB。各外部服务按其政策处理收到的信息。

可随时断开、更换节点或修改分流规则。清除 Fire TV 设置中本应用的存储数据，可删除本地配置与凭据缓存。撤销订阅或删除外部服务商保留的数据，请联系相应提供方。隐私问题可通过商店列出的开发者客服邮箱私下联系，请勿将订阅链接或密钥贴到公开问题区。
