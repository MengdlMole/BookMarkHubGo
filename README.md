# BookmarkHubGo

BookmarkHubGo 是一个全新的、本地优先的跨平台书签项目，不依赖原有 `BookmarkHub` 扩展代码。它由一个便携式 Go Core 和一个独立的 Chrome/Edge 扩展组成。

## 当前功能

- Windows、macOS、Linux 的单文件 Go Core
- Core 内嵌管理页面，启动后访问 `http://127.0.0.1:17836/`
- Chrome 和 Edge 共用一套 Manifest V3 扩展
- 保存或更新当前页面，Core 离线时在扩展中排队
- 任意层级分组、同层拖动排序、分组重命名和删除
- 分组树逐级展开和折叠，并记忆折叠状态
- 书签卡片视图和列表视图切换，并记忆视图偏好
- 每个书签可设置预设强调色边框，卡片和列表视图统一显示
- 标签、备注、星标、搜索、分组/标签/星标筛选和排序
- Netscape Bookmark HTML 导入和导出
- XBEL 1.0 导入和导出
- 每台设备使用独立 HTML 文件，支持坚果云等文件同步工具
- Lamport 修订号、删除墓碑、原子写入和滚动备份
- 便携式版本目录、离线升级和回滚启动器
- 仅绑定 `127.0.0.1`，扩展通过随机令牌配对
- 仅使用 Go 标准库，无第三方 Go 依赖

## 项目结构

```text
BookmarkHubGo/
├── cmd/
│   ├── bookmarkhub-core/       # 本地 API、管理页面和同步引擎入口
│   └── bookmarkhub-launcher/   # 便携启动、离线升级、回滚
├── internal/
│   ├── model/                  # 分组、书签、修订号及合并模型
│   ├── server/                 # HTTP API 和内嵌管理页面
│   └── storage/                # HTML/XBEL 与同步目录
├── extension/                  # 独立 Chrome/Edge 扩展
├── scripts/                    # 跨平台构建和升级包脚本
├── go.mod
└── LICENSE
```

## 开发运行

需要 Go 1.23 或更高版本：

```bash
go test ./...
go run ./cmd/bookmarkhub-core --home ./dev-home
```

然后打开：

```text
http://127.0.0.1:17836/
```

开发数据默认保存在：

```text
dev-home/
├── config/settings.json
└── data/sync/
    ├── devices/
    ├── backups/
    └── exports/
```

也可以直接指定坚果云本地目录：

```bash
go run ./cmd/bookmarkhub-core --home ./dev-home --sync-dir "/path/to/Nutstore/BookmarkHub"
```

## 安装浏览器扩展

Chrome：

1. 打开 `chrome://extensions/`。
2. 开启“开发者模式”。
3. 点击“加载已解压的扩展程序”。
4. 选择本项目的 `extension` 目录。

Edge：

1. 打开 `edge://extensions/`。
2. 开启“开发人员模式”。
3. 点击“加载解压缩的扩展”。
4. 选择本项目的 `extension` 目录。

首次连接：

1. 启动 BookmarkHub Core。
2. 在管理页面点击“设置”。
3. 复制扩展配对令牌。
4. 打开浏览器扩展并粘贴令牌。

扩展和 Core 可以分别升级。扩展固定连接 `127.0.0.1:17836`，Core 不会监听局域网地址。

## HTML 同步文件

每台设备只写自己的文件：

```text
bookmarkhub-macos-<device-id>.html
bookmarkhub-windows-<device-id>.html
bookmarkhub-linux-<device-id>.html
```

文件是可被 Chrome 和 Edge识别的 Netscape Bookmark HTML。分组使用嵌套的 `H3/DL`，标签使用 `TAGS`，备注使用 `DD`。星标、强调色、BookmarkHub 的 UUID、修订号和删除墓碑保存在标准 HTML 可忽略的自定义属性及 Base64 JSON 注释中；XBEL 导入导出同样保留星标和强调色。

不要让两个复制出来的 Core 配置共享相同 `deviceId`。将程序复制到新设备时，应删除新副本的 `config/settings.json`，让它生成新的设备 ID。

## 导入导出

管理页面支持：

- 合并导入 HTML/XBEL：按 URL 去重
- 替换导入 HTML/XBEL：旧记录写入删除墓碑
- 导出 `bookmarks.html`，供 Chrome/Edge 导入
- 导出 `bookmarkhub-export.xbel`

同步目录中的设备 HTML 是 BookmarkHub 的同步数据源。浏览器导入该文件后再导出，可能丢弃 BookmarkHub 的自定义标签和同步属性，因此不应使用浏览器导出的文件覆盖 `devices` 中的文件。

## 便携构建

macOS/Linux：

```bash
chmod +x scripts/*.sh
./scripts/build.sh 0.1.0
```

Windows PowerShell：

```powershell
.\scripts\build.ps1 -Version 0.1.0
```

构建结果示例：

```text
dist/bookmarkhub-windows-amd64/
├── bookmarkhub.exe
├── current.json
└── versions/
    └── 0.1.0/
        └── bookmarkhub-core.exe

dist/bookmarkhub-extension-0.1.0.zip
```

桌面程序与浏览器扩展是两个独立发布包，可分别下载和升级。

用户只需解压整个目录并运行 `bookmarkhub` 或 `bookmarkhub.exe`。配置保存在根目录的 `config`，书签同步目录可以指向程序目录外部，因此升级不会覆盖数据。

### macOS 首次运行被拦截

当前开源构建没有使用 Apple Developer ID 签名和公证，从网络下载后可能被 Gatekeeper 拦截。请确认安装包来源可信，然后解压完整目录，在终端执行：

```bash
sh /完整路径/bookmarkhub-macos-arm64/macos-first-run.command
```

Apple Silicon（M1/M2/M3/M4 等）选择 `arm64`，Intel Mac 选择 `amd64`。首次运行脚本只恢复本包内二进制的执行权限，并移除它们各自的 `com.apple.quarantine` 属性；不会关闭 Gatekeeper，也不会修改系统全局安全策略。也可以先尝试运行一次，然后按照 Apple 官方方式前往“系统设置 → 隐私与安全性”点击“仍要打开”。

正式对外发布时，推荐使用 Developer ID 对两个 macOS 二进制签名并提交 Apple 公证，从根本上避免此提示。

构建脚本默认输出：

- macOS amd64、arm64
- Linux amd64、arm64
- Windows amd64、arm64

## 离线升级与回滚

创建升级包：

```bash
./scripts/make-update.sh 0.2.0 windows amd64 \
  ./bookmarkhub-core.exe ./bookmarkhub-update-0.2.0-windows-amd64.zip
```

关闭正在运行的 Core，然后执行：

```bash
bookmarkhub upgrade ./bookmarkhub-update-0.2.0-windows-amd64.zip
bookmarkhub version
bookmarkhub rollback
```

启动器会：

1. 校验升级平台、架构和 SHA-256。
2. 将 Core 解压到新的 `versions/<version>`。
3. 原子切换 `current.json`。
4. 保留前一个版本供回滚。

如果 `config/update-public-key` 中配置了 Base64 编码的 Ed25519 公钥，启动器还会强制校验升级包中的 `manifest.sig`。未配置公钥的开发版本仅执行 SHA-256 完整性校验，正式发布时应配置签名公钥。

## API

```text
GET  /api/v1/status
GET  /api/v1/state
POST /api/v1/bookmarks
POST /api/v1/bookmarks/delete
POST /api/v1/groups
POST /api/v1/groups/reorder
POST /api/v1/groups/delete
POST /api/v1/import
GET  /api/v1/export?format=html|xbel
POST /api/v1/settings/sync-dir
```

扩展请求必须携带：

```text
X-BookmarkHub-Token: <pairing-token>
```

## 许可证

MIT。Go Core、管理页面和浏览器扩展均可独立修改与分发。
