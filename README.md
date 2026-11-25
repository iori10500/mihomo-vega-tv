# Mihomo for Vega TV

适配 Amazon Vega OS 的遥控器 VPN 客户端，使用 MetaCubeX 的 Mihomo 和 Vega 系统 VPN 接口。界面采用 React Native for Vega 与 C++ TurboModule。此项目由社区独立开发。已知硬件目标是 Fire TV Stick 4K Select（32 位 ARMv7、Vega OS 2.1）。

## 用遥控器操作

1. 在「订阅管理」输入自己的 HTTPS 节点订阅，选择「保存并连接」，首次使用时在系统对话框允许 VPN。主页的大按钮控制连接。
2. 「节点」默认自动选择。列表显示当前节点和最近延迟。打开节点详情可固定节点、测延迟或测下载速度。整组延迟测试并发为二，下载测试通过单个候选节点向 Cloudflare 请求 4 MiB。结果表示该测试端点的响应时间或下载速率。可随时取消。
3. 「规则 → 视频服务」可设置哔哩哔哩、腾讯视频、爱奇艺、优酷和芒果 TV。规则按服务域名识别。个人规则支持精确域名、域名后缀和 IPv4 网段。远程规则集支持 HTTPS、YAML（payload）、文本和 MRS（domain/ipcidr）。每个源最多下载 4 MiB。连接期间每日检查更新，也可手动更新。
4. 智能模式依次匹配局域网、视频服务明确指定的动作、自定义规则与订阅列表、视频服务默认直连、国内域名或 IP 直连、其余走代理。全局模式中局域网直连，其余走代理。更改作用于新连接。当前系统隧道接管 IPv4。

## 构建和侧载

本项目的交叉编译脚本面向 Apple Silicon macOS，需 Amazon Vega SDK 2.0.15159、Node.js/npm、Python 3、Git 和 Go **1.26.11**。Go 版本固定是因为该构建含有 ARMv7 共享库入口的栈对齐修正。按 Amazon 的 [Vega SDK 文档](https://developer.amazon.com/docs/vega/latest/overview.html) 安装并激活 SDK，确保其 bin 目录中的 vega 可用。

```sh
npm ci
npm run build
vega device list
vega device install-app -d DEVICE_SERIAL -p build/armv7-release/mihomo-tv_armv7.vpkg
```

首次构建会先准备 SDK 工具链，然后获取固定版本的 Mihomo、交叉编译核心并打包。已有完整上游检出时可将 MIHOMO_SOURCE 指向该目录，脚本会验证提交号。安装时使用自己的开发者账号和已配对的设备。参见 [Amazon 实机安装说明](https://developer.amazon.com/docs/vega/latest/run-apps)。

内核回归测试可在 Mac 上运行 `./build-core.sh --test`，覆盖旧配置迁移、节点选择、规则优先级、规则集解析和更新失败后的回读。

需要内存采样时，在干净检出根目录创建 .measure-memory 再构建。该测试构建每五秒通过 Vega 应用日志报告当前进程的 PSS 和 RSS（KiB），分别标为 ui 和 vpn，-1 表示系统未提供该指标。常规构建默认关闭采样。

## 上游与许可证

自有源码采用 GPL-3.0-only。核心来自 [MetaCubeX/mihomo](https://github.com/MetaCubeX/mihomo)，固定在 v1.19.34。服务和地区数据来自 [meta-rules-dat](https://github.com/MetaCubeX/meta-rules-dat)。规则文件的来源、固定版本和 SHA-256 见 [Rule-sources.txt](assets/raw/Rule-sources.txt)。腾讯视频规则使用视频服务的域名事实，参考来源列于同一文件。Amazon SDK 由开发者自行安装，遵循其许可条款。节点和规则订阅由使用者自行提供。
