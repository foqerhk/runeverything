# 国内外独立部署

RunEverything **一套客户端**，按运行环境自动选择区域入口；**不跨境回退**。

| | 国内 | 国外 |
|--|------|------|
| getnode（Registrar + seeds + 管理台） | `https://getnode.intentcomputing.cn` | `https://getnode.intentcomputing.net` |
| 节点二级域（示例） | `*.intentcomputing.cn`（阿里云 DNS） | `*.runeverything.online`（Cloudflare） |
| 官网 | `intentcomputing.cn` | `intentcomputing.net`（待部署） |
| 种子 JSON | 仅 `/seeds.json` on CN getnode | `/seeds.json` on Intl getnode（可另加 GitHub 镜像） |

## 客户端行为

1. 识别区域（`RE_REGION=cn|intl` 可强制）：时区/语言启发式 → 公网 IP 国家码（`RE_GEO_URL` / `geo_url`）→ 默认 intl  
2. 国内：**只**访问 `getnode.intentcomputing.cn`（enroll / claim / report / seeds）  
3. 国外：**只**访问 `getnode.intentcomputing.net`  
4. 显式 `RE_REGISTRAR_URL` / `RE_SEEDS_URL` / `RE_SEEDS` 仍可覆盖（运维调试）  
5. **界面/日志语言**：系统中文（含繁体）→ 简体中文；其他 → 英文。可用 `RE_LANG=zh|en` 强制

### NAT / 公网判定与出口 IP 接口

是否在 NAT 后：出口公网 IP 是否绑在本机网卡上（与 OS 无关）。  
出口 IP 探测按区域优先、另一区域兜底：

| 区域优先 | 内置默认 |
|----------|----------|
| 国内 | `https://ip.3322.net`、`https://myip.ipip.net/s` |
| 国外 | `https://api.ipify.org`、`https://ifconfig.me/ip` |

可更换（优先级：环境变量 > `config.json` > 内置）：

```bash
runeverything config set-ip-echo --cn URL1,URL2 --intl URL3,URL4
runeverything config set-geo-url 'http://ip-api.com/json/?fields=status,countryCode'
# 或：RE_IP_ECHO_CN / RE_IP_ECHO_INTL / RE_GEO_URL
```

`RE_GEO_URL` 需返回 JSON：`{"status":"success","countryCode":"CN"}`（与 ip-api.com `fields=status,countryCode` 兼容）。

## 运维侧

- **国内**：已在 `8.137.32.163` 部署 CN getnode + 管理台 + 官网  
- **国外**：已在 `208.113.214.106` 部署 Intl getnode + 管理台；zone 配 `runeverything.online`，域名 `getnode.intentcomputing.net`（SSH `34417`）

两套 state / admin / seeds **互不同步**，避免节点与用户元数据跨境。

### 区域官方中继

getnode 机器同时跑本区的官方中继（systemd `runeverything-relay.service`，二进制 `/opt/runeverything/runeverything-relay`）。TLS 由 nginx + Let's Encrypt 终止后反代到 `127.0.0.1:8787`，UDP 直接对外 `:8787`（防火墙需放行 `8787/udp`）。

| | 国内 | 国外 |
|--|------|------|
| WSS | `wss://8e2cee2dd1.intentcomputing.cn/re2` | `wss://ed9741c9d4.intentcomputing.net/re2` |
| UDP | `8e2cee2dd1.intentcomputing.cn:8787` | `ed9741c9d4.intentcomputing.net:8787` |
| DNS | 阿里云 | Cloudflare（`intentcomputing.net` zone，仅 DNS 不走代理） |
| `/seeds.json` 来源 | 从 GitHub `docs/seeds.json` 同步 | 本机静态文件 `/var/lib/runeverything/seeds.json` |

国外节点 systemd 里带 `RE_REGION=intl`，启动参数 `-public wss://ed9741c9d4.intentcomputing.net/re2 -public-udp ed9741c9d4.intentcomputing.net:8787 -share=true`。

更新中继：

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" \
  -o /tmp/runeverything-relay-linux ./cmd/relay
# 上传为 /opt/runeverything/runeverything-relay.new，备份旧文件后替换
systemctl restart runeverything-relay
curl -s http://127.0.0.1:8787/healthz   # → ok
```

中继状态（配对令牌、session_ticket、当前控制端）只在内存里：**重启后已配对手机需重新扫码**，尽量避开使用高峰。协议改动应保持向后兼容（旧客户端不带新字段时按旧行为处理），国内外节点可以分别升级。
