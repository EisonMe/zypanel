# ZyPanel

一个轻量的服务器管理面板：用 Docker 跑起来，在一个页面里看容器、systemd 服务、Cloudflare Tunnel、监听端口和操作审计。

Go 单二进制实现，前端用标准库 `html/template` 渲染，不需要 Node、不需要数据库。

> 这是 ZiYin Server Panel 的 Go 重写版，原 Python 版本已归档。

---

## 功能

| 模块 | 说明 |
| --- | --- |
| 仪表盘 | 主机名、运行时长、负载、内存、根分区占用、容器与服务计数 |
| 容器 | 列表 / 详情 / 端口与挂载 / 日志 / 启动 / 停止 / 重启 / 删除 / prune |
| 服务 | systemd 单元列表与 start / stop / restart / enable |
| 隧道 | 读取并编辑 `cloudflared` ingress，自动备份、校验、失败回滚、重启服务 |
| 端口 | 解析 `ss -ltnp` 展示监听端口与进程 |
| 审计 | 所有写操作落 JSONL，页面可查 |
| 设置 | 展示当前生效的配置与功能开关 |

每个危险操作都需要二次确认（输入 `DELETE` / `PRUNE`），全部写操作都有 CSRF 校验，所有外部命令都以 argv 形式执行、不经过 shell。

---

## 快速开始

```bash
git clone https://github.com/EisonMe/zypanel.git
cd zypanel
cp .env.example .env
# 编辑 .env：至少设置 ZYPANEL_SECRET_KEY 和 ZYPANEL_ADMIN_PASSWORD
docker compose up -d
```

打开 http://127.0.0.1:8090 。

生成一个强随机密钥：

```bash
openssl rand -hex 32
```

---

## 配置

所有配置来自环境变量，完整清单见 [`.env.example`](.env.example)。

必填：

| 变量 | 说明 |
| --- | --- |
| `ZYPANEL_SECRET_KEY` | 会话签名密钥，至少 16 字符 |
| `ZYPANEL_ADMIN_PASSWORD` | 管理员密码（启动时用 bcrypt 哈希） |
| `ZYPANEL_ADMIN_PASSWORD_HASH` | 也可以直接给 bcrypt hash，优先级高于上一条 |

常用：

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `ZYPANEL_ADDR` | `:8090` | 监听地址 |
| `ZYPANEL_COOKIE_SECURE` | `1` | HTTPS 下保持开启 |
| `ZYPANEL_DATA_DIR` | `/data` | 审计日志与收藏数据目录 |
| `ZYPANEL_DOCKER` | `1` | 启用容器管理 |
| `ZYPANEL_SYSTEMD` | `0` | 启用 systemd 管理（需挂载 `/run` 与 `pid: host`） |
| `ZYPANEL_CLOUDFLARED` | `0` | 启用隧道配置编辑 |
| `ZYPANEL_OPENCLAW` | `0` | 启用 OpenClaw 状态页（需镜像内自带 CLI） |
| `ZYPANEL_READ_ONLY` | `0` | 只读模式，隐藏全部写操作 |
| `ZYPANEL_CLOUDFLARED_SUFFIX_ALLOWLIST` | 空 | 只允许在这些域名后缀下新增路由，逗号分隔 |

---

## 部署拓扑

```
浏览器 ──HTTPS──> Cloudflare ──隧道──> cloudflared ──> 127.0.0.1:8090 ──> zypanel 容器
```

容器默认只把端口绑定在 `127.0.0.1`，公网入口交给 Cloudflare Tunnel 或你自己的反向代理。

### 需要 systemd 管理时

`systemctl` 必须能访问宿主机的 systemd。这要求放宽容器隔离：

```yaml
    pid: host
    privileged: true
    volumes:
      - /run:/run:ro
```

镜像里带了一个 `systemctl-host` 包装脚本，它用 `nsenter` 进入 PID 1 的命名空间执行宿主机上的 `systemctl`，所以不需要在镜像里再装一套 systemd。启用后容器对宿主机拥有接近 root 的权限，请自行评估风险；不需要就别开（默认关闭）。

### 需要编辑 cloudflared 配置时

```yaml
    volumes:
      - /etc/cloudflared:/etc/cloudflared
```

面板会先写 `.bak.<timestamp>` 备份，再原子替换；如果 `cloudflared tunnel ingress validate` 不通过会自动回滚。

两项都开启时可以直接用 [docker-compose.full.yml](docker-compose.full.yml)：

```bash
docker compose -f docker-compose.full.yml up -d
```

对应的 `.env`：

```ini
ZYPANEL_SYSTEMD=1
ZYPANEL_CLOUDFLARED=1
ZYPANEL_CLOUDFLARED_CONFIG=/etc/cloudflared/config.yml
ZYPANEL_CLOUDFLARED_SUFFIX_ALLOWLIST=.example.com
```

`ZYPANEL_CLOUDFLARED_SUFFIX_ALLOWLIST` 建议一定设置，它限制面板只能新增这些域名后缀下的路由。

---

## 本地构建

```bash
go build ./cmd/zypanel
./zypanel
```

或者：

```bash
docker build -t zypanel:local .
```

CI 会在每次 push 时执行 `gofmt`、`go vet`、构建和镜像构建；打 `v*` 标签时自动构建多架构镜像推送到 GHCR，并把各平台二进制挂到 Release。

---

## 安全说明

- 单管理员账号，密码用 bcrypt 校验；会话是 HMAC-SHA256 签名的 HttpOnly Cookie。
- 所有写操作校验 CSRF（double-submit cookie）。
- 子进程一律 `exec.Command(argv...)`，不经过 shell，参数做白名单/格式校验。
- 容器名、systemd 单元名都有严格正则约束。
- 审计日志记录用户、来源 IP、动作、目标与结果。
- 容器默认以 root 运行，因为需要读 `/var/run/docker.sock`。如果你的环境允许，改成专用用户并加入宿主机的 `docker` 组会更安全。

发现安全问题请开 issue，不要公开提 PR 之前先私下说明。

---

## 目录结构

```
cmd/zypanel/            程序入口
internal/config/        环境变量解析
internal/auth/          bcrypt 校验、会话签名、CSRF
internal/audit/         JSONL 审计
internal/execx/         无 shell 的命令执行封装
internal/web/           路由、中间件、页面处理
internal/web/templates  Go 模板
internal/web/static     内嵌静态资源
```

---

## License

MIT
