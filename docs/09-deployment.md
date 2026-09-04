# 部署与跨平台使用指南

Diff-Mem 编译产物是**静态链接的单文件可执行程序**，目标机器无需安装 Go、运行时或任何依赖库。

---

## 一、编译

### 1.1 命令速查

在本机（任意平台）交叉编译所有目标平台：

```powershell
# Windows (amd64)
go build -trimpath -ldflags "-s -w" -o build/diff-mem-mcp-windows-amd64.exe ./cmd/mcp-server
go build -trimpath -ldflags "-s -w" -o build/diff-mem-api-windows-amd64.exe ./cmd/server
go build -trimpath -ldflags "-s -w" -o build/diff-mem-agent-windows-amd64.exe ./cmd/agent

# Linux (amd64)
$env:GOOS = "linux"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"
go build -trimpath -ldflags "-s -w" -o build/diff-mem-mcp-linux-amd64 ./cmd/mcp-server
go build -trimpath -ldflags "-s -w" -o build/diff-mem-api-linux-amd64 ./cmd/server
go build -trimpath -ldflags "-s -w" -o build/diff-mem-agent-linux-amd64 ./cmd/agent
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED

# macOS (Apple Silicon)
$env:GOOS = "darwin"; $env:GOARCH = "arm64"; $env:CGO_ENABLED = "0"
go build -trimpath -ldflags "-s -w" -o build/diff-mem-mcp-macos-arm64 ./cmd/mcp-server
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED
```

### 1.2 参数说明

| 参数 | 作用 |
|---|---|
| `GOOS` / `GOARCH` | 目标操作系统 / CPU 架构，决定产物格式（PE / ELF / Mach-O） |
| `CGO_ENABLED=0` | 强制纯 Go 编译，交叉编译的前提。本项目全依赖无 C 代码（Badger、modernc.org/sqlite 均为纯 Go），永远可以设 0 |
| `-trimpath` | 去掉二进制里的本地路径，构建可复现 |
| `-ldflags "-s -w"` | 剥离符号表和 DWARF 调试信息，体积减约 30%（25 MB → 17 MB），生产环境建议加；需要 delve 调试时不要加 |

### 1.3 产物对照表

| 目标平台 | GOOS | GOARCH | 产物格式 |
|---|---|---|---|
| Windows 64 位 | windows | amd64 | `.exe`（PE） |
| Linux 64 位 | linux | amd64 | ELF（无后缀） |
| Linux ARM（如树莓派、ARM 云主机） | linux | arm64 | ELF |
| macOS Apple Silicon | darwin | arm64 | Mach-O |
| macOS Intel | darwin | amd64 | Mach-O |

> 注意：exe 只能跑在 Windows，ELF 只能跑在 Linux。但编译动作永远可以在任意一台装有 Go 1.25+ 的机器上完成。

### 1.4 体积说明

单文件 8~26 MB 属于正常水平，体积构成：

- Go runtime + 标准库（约 8.6 MB 地板价，静态链接的代价，也是零依赖的来源）
- modernc.org/sqlite（约 10 MB）：整个 SQLite 引擎的纯 Go 翻译版
- BadgerDB（约 4-5 MB）
- 你的业务代码：几百 KB

---

## 二、各平台运行方案

同一个二进制，用启动参数决定行为：

| 模式 | 参数 | 适用场景 |
|---|---|---|
| stdio | `-stdio` | 单人本地使用，MCP 客户端（Claude Desktop / Cursor）自动拉起进程 |
| SSE | `-host 0.0.0.0 -port 8787` | 服务器常驻，多客户端远程共享同一份记忆 |

所有模式下 `-store` 选择存储后端：`sqlite`（推荐）/ `badger` / `memory`（仅测试）。

### 2.1 Windows — 个人本地（stdio，推荐）

无需注册服务、不占端口。在 Claude Desktop / Cursor 配置中添加：

```json
{
  "mcpServers": {
    "diff-mem": {
      "command": "C:\\tools\\diff-mem\\diff-mem-mcp-windows-amd64.exe",
      "args": ["-stdio", "-store", "sqlite", "-data", "C:\\tools\\diff-mem\\data"]
    }
  }
}
```

客户端启动会话时自动拉起进程，退出时自动回收。

### 2.2 Windows — 服务器常驻（SSE + NSSM）

用 [NSSM](https://nssm.cc/) 注册为 Windows 服务：

```powershell
nssm install DiffMem C:\tools\diff-mem\diff-mem-mcp-windows-amd64.exe "-host 0.0.0.0 -port 8787 -store sqlite -data C:\tools\diff-mem\data"
nssm start DiffMem
```

开机自启与崩溃拉起均为 NSSM 默认行为。防火墙放行 8787 端口后，客户端连接 `http://<机器IP>:8787/mcp`。

### 2.3 Linux — 服务器常驻（SSE + systemd，生产推荐）

**目录约定**：

```
/opt/diff-mem/
├── diff-mem-mcp-linux-amd64   # 上传的二进制
└── data/                      # SQLite 数据目录
```

**部署步骤**：

```bash
# 1. 上传并赋权
scp build/diff-mem-mcp-linux-amd64 user@server:/opt/diff-mem/diff-mem-mcp
ssh user@server "chmod +x /opt/diff-mem/diff-mem-mcp"

# 2. 创建无登录权限的运行用户
useradd -r -s /usr/sbin/nologin diffmem
chown -R diffmem:diffmem /opt/diff-mem/data
```

**`/etc/systemd/system/diff-mem.service`**：

```ini
[Unit]
Description=Diff-Mem MCP Server
After=network.target

[Service]
ExecStart=/opt/diff-mem/diff-mem-mcp -host 0.0.0.0 -port 8787 -store sqlite -data /opt/diff-mem/data
Restart=on-failure
RestartSec=3
User=diffmem
NoNewPrivileges=true
ProtectSystem=strict
ReadWritePaths=/opt/diff-mem/data

[Install]
WantedBy=multi-user.target
```

```bash
systemctl daemon-reload
systemctl enable --now diff-mem
systemctl status diff-mem        # 确认 running
curl http://localhost:8787/health   # 健康检查
```

### 2.4 macOS — 个人本地

与 Windows 方式相同：客户端配置 `command` 指向 Mach-O 二进制 + `-stdio` 模式，无需 launchd。

---

## 三、客户端接入

### 3.1 Claude Desktop / Cursor（stdio）

见 2.1。`command` 填二进制的**绝对路径**，`args` 里的 `-data` 指向一个持久目录（别放临时目录，否则记忆会丢）。

### 3.2 远程 SSE 接入

```json
{
  "mcpServers": {
    "diff-mem": {
      "url": "http://your-server:8787/mcp"
    }
  }
}
```

编程对接（任意 MCP SDK）：

```typescript
const transport = new SSEClientTransport(new URL("http://your-server:8787/mcp"));
const client = new Client({ name: "my-app", version: "1.0" });
await client.connect(transport);
```

### 3.3 CLI Agent（HTTP API）

```bash
# 服务器端
./diff-mem-api-linux-amd64 -addr :8080 -store sqlite -data /opt/diff-mem/data

# 终端助手连接
./diff-mem-agent-linux-amd64 -server http://localhost:8080
```

---

## 四、安全注意事项

1. **SSE 模式当前无鉴权**（`internal/mcp/server.go` 中 `DisableLocalhostProtection: true`）。公网部署必须在前面加一层反代：
   - Nginx + Basic Auth / API Key
   - 或防火墙限制来源 IP
2. **`-store memory` 仅用于开发测试**，重启数据即失忆。
3. 数据目录（`-data`）包含全部记忆，注意备份与权限（Linux 下已通过 `diffmem` 专用用户 + `ProtectSystem=strict` 隔离）。

---

## 五、升级与回滚

- **升级**：替换二进制文件，重启服务（`systemctl restart diff-mem`）。数据格式由存储层自动兼容，`-data` 目录不动。
- **回滚**：换回旧二进制文件重启即可。
- **备份**：直接停服后拷贝 `-data` 目录（SQLite 是单文件 + WAL，也可在线 `sqlite3 diff-mem.db ".backup backup.db"`）。

## 六、常见问题

**Q: 交叉编译报错 `requires cgo`？**
确认 `CGO_ENABLED=0` 已设置。本项目所有依赖均为纯 Go，正常不会出现。

**Q: Linux 上跑不起来 `cannot execute binary file`？**
架构不匹配。`uname -m` 确认目标机是 x86_64（对应 amd64）还是 aarch64（对应 arm64），重新编译对应架构。

**Q: SQLite 数据库文件在哪？**
`-data` 目录下的 `diff-mem.db`（及 WAL 伴生文件 `diff-mem.db-wal`、`diff-mem.db-shm`）。

**Q: 如何从 Badger 迁移到 SQLite？**
目前数据格式互不兼容。可用小脚本遍历旧 `AllNodes()` 写入新 store（接口现成，约二十行），或通过 HTTP API 逐节点搬迁。
