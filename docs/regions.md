# 国内外独立部署

RunEverything **一套客户端**，按运行环境自动选择区域入口；**不跨境回退**。

| | 国内 | 国外 |
|--|------|------|
| getnode（Registrar + seeds + 管理台） | `https://getnode.intentcomputing.cn` | `https://getnode.intentcomputing.net` |
| 节点二级域（示例） | `*.intentcomputing.cn`（阿里云 DNS） | `*.runeverything.online`（Cloudflare） |
| 官网 | `intentcomputing.cn` | `intentcomputing.net`（待部署） |
| 种子 JSON | 仅 `/seeds.json` on CN getnode | `/seeds.json` on Intl getnode（可另加 GitHub 镜像） |

## 客户端行为

1. 识别区域（`RE_REGION=cn|intl` 可强制）：时区/语言启发式 → 公网 IP 国家码 → 默认 intl  
2. 国内：**只**访问 `getnode.intentcomputing.cn`（enroll / claim / report / seeds）  
3. 国外：**只**访问 `getnode.intentcomputing.net`  
4. 显式 `RE_REGISTRAR_URL` / `RE_SEEDS_URL` / `RE_SEEDS` 仍可覆盖（运维调试）

## 运维侧

- **国内**：已在 `8.137.32.163` 部署 CN getnode + 管理台 + 官网  
- **国外**：已在 `208.113.214.106` 部署 Intl getnode + 管理台；zone 配 `runeverything.online`，域名 `getnode.intentcomputing.net`（SSH `34417`）

两套 state / admin / seeds **互不同步**，避免节点与用户元数据跨境。
