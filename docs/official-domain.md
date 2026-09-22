# 官方域名：客户端约定（Registrar 服务端闭源）

志愿者**不必自备域名**。官方在**官网单独部署**闭源 Registrar（不进入本仓库 / 不公开 GitHub）：发放子域、写 Cloudflare **DNS-only（灰云关闭）** A 记录、主动巡检与剔除。本仓库只保留**客户端**：志愿者 `claim`、家庭用户 `report`、Relay ACME。

> **RE2.1 / REUDP**：画面与键鼠走 **UDP**。Cloudflare 代理模式无法转发 UDP，因此节点 A 记录必须 **DNS-only**；`wss` 由志愿者本机 ACME 提供证书。

```
志愿者 Relay ──POST /v1/enroll──► 官网 Registrar（闭源）──► 专属 join_token（落盘本机）
            ──POST /v1/claim───► 同上（Bearer=专属 token）──Aliyun/CF DNS──► A（DNS-only）
家庭 Agent   ──POST /v1/report─► 同上（真实连接失败上报）
手机 / Agent ──wss://{slug}.{zone}/re2 + UDP──► 直连志愿者
             zone = intentcomputing.cn（国内）或 runeverything.online（国外）
```

## 本仓库里有什么

| 路径 | 作用 |
|------|------|
| `internal/registrar/client.go` | HTTP：`/v1/enroll`、`/v1/claim`、`/v1/report` |
| `internal/registrar/token.go` | 开启分享时自动 enroll，token 存 `~/.runeverything/node-join-token.json` |
| `internal/registrar/report_async.go` | 家庭端失败时异步上报 |
| `internal/registrar/autoclaim.go` | 志愿者 `RE_AUTO_DOMAIN=1` 自动 claim |
| `internal/registrar/autotls.go` | 志愿者本机 ACME / wss |

**没有** Cloudflare Token、DNS 写入、sweep 剔除逻辑——这些只在官网私有部署里。

## 志愿者（公开客户端）

开启分享 + 自动域名即可；**不必再手工要 join token**：

```bash
export RE_AUTO_DOMAIN=1
export RE_SHARE_RELAY=1          # 默认就是开；关掉则不会 enroll/claim
export RE_REGISTRAR_URL=https://getnode.intentcomputing.cn
sudo -E runeverything-relay -auto-domain   # 需开放 80/443
```

首次会 `POST /v1/enroll` 领取专属 token 并保存；之后用该 token `claim`。也可设 `RE_JOIN_TOKEN` 跳过 enroll（运维调试用）。

## 家庭用户上报（公开）

Agent / P2P 探活在真实失败时自动 `POST /v1/report`（需 `RE_REGISTRAR_URL`；**不要**配置 join token）。可用 `RE_REPORT=0` 关闭。

| reason | 含义 |
|--------|------|
| `not_relay` / `protocol` | 不像本项目协议 |
| `peers_fail` / `health_fail` / `tls_fail` | 探活或证书失败 |
| `dial_fail` | 连不上（可能瞬断） |

请求体：

```json
{
  "url": "wss://abc.nodes.example.com/re2",
  "reason": "not_relay",
  "detail": "optional",
  "reporter_id": "device-uuid"
}
```

服务端（闭源）负责去重、计分、删 DNS；客户端只负责诚实上报。

## 公开 API 约定（官网实现）

### `POST /v1/enroll`（公开，限速）

志愿者开启分享时自动调用，领取**该 node 专属** `join_token`（服务端只存哈希；重复 enroll 会轮换）。

```json
{"node_id":"device-uuid"}
```

```json
{"node_id":"device-uuid","join_token":"rejt_…","rotated":false,"message":"…"}
```

### `POST /v1/claim`（需专属 token 或运维 admin token）

```http
Authorization: Bearer <join_token>
Content-Type: application/json

{"ip":"203.0.113.10","node_id":"device-uuid"}
```

```json
{"hostname":"a1b2c3d4e5.nodes.example.com","wss_url":"wss://a1b2c3d4e5.nodes.example.com/re2","ip":"203.0.113.10"}
```

### `POST /v1/report`（公开，无 Join Token）

见上。响应建议：

```json
{"accepted":true,"hostname":"abc.nodes.example.com","score":5.0,"revoked":false,"message":"recorded"}
```

### `GET /healthz`

返回 `ok`。

## 环境变量（客户端）

| 变量 | 说明 |
|------|------|
| `RE_AUTO_DOMAIN` | 志愿者自动 claim + ACME（需开启分享） |
| `RE_SHARE_RELAY` | 分享中继；开启时才会 enroll / 自动域名 |
| `RE_REGISTRAR_URL` | 官网 Registrar（enroll + claim + report） |
| `RE_JOIN_TOKEN` | 可选；有则跳过 enroll（运维调试） |
| `RE_REPORT` | 家庭上报，默认开（`0` 关） |
| `RE_ACME_EMAIL` / `RE_ACME_STAGING` / `RE_CERT_DIR` / `RE_DNS_WAIT` | ACME / DNS 等待 |

## 运维（闭源侧，不在本仓库）

Registrar 服务端为闭源，**不在本仓库**。

官方自行：Cloudflare DNS Edit Token、`RE_BASE_DOMAIN`、主动 sweep、按上报计分剔除。  
`POST /v1/enroll` 向开启分享的志愿者自动发专属 token（按 IP 限速）；可选保留 admin `RE_JOIN_TOKEN` 作运维后门。对外 HTTPS 暴露 enroll/claim/report。
