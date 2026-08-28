# AllCallAll — WAF 规则与防护接入

在流量入口前置 WAF，拦截 OWASP Top 10 风险并针对**生成式 AI 接口**做专项加固。
提供两套可互换的规则集，按你们的边缘架构二选一或叠加使用。

## 规则集

| 文件 | 适用场景 | 风格 |
| --- | --- | --- |
| [`cloudflare-ruleset.yaml`](./cloudflare-ruleset.yaml) | 走 Cloudflare / 边缘网关 | 分阶段 Custom Rules，含 AI 滥用专项 + 边缘限速 |
| [`modsecurity-rules.conf`](./modsecurity-rules.conf) | 自建反向代理（nginx + Coraza/ModSecurity3） | OWASP CRS 补充规则，含提示注入 / SSRF / 命令注入拦截 |

两者规则语义对齐，覆盖：

- **A01 路径穿越/LFI**
- **A03 SQL 注入 / 命令注入**
- **A07/A10 SSRF**（拦截元数据端点 `169.254.169.254`、内网地址）
- **AI 滥用**：提示注入（`ignore previous instructions`、`<system>` 等）、RAG 批量爬取、免费额度 token  harvesting
- **限速兜底**：单 IP 600 req/min（边缘 + 后端滑动窗口双重防护）

## 接入方式

### Cloudflare
```bash
# 灰度：先将关键规则 action 改为 log，观察 Analytics → Security Events 误杀
cf api --post accounts/$ACCOUNT_ID/rulesets/zones/$ZONE_ID/phases/http_request_firewall/custom \
  -d @cloudflare-ruleset.yaml
```
确认无误后将 `log` 升级为 `block`，并开启 Managed Rules（OWASP CRS）。

### nginx + Coraza
```nginx
server {
  listen 443 ssl;
  modsecurity on;
  modsecurity_rules_file /etc/modsecurity/crs-setup.conf;
  modsecurity_rules_file /etc/modsecurity/modsecurity.conf;
  modsecurity_rules_file /etc/modsecurity/allcallall-rules.conf;
  location /api/ { proxy_pass http://allcallall-api; }
}
```

## 与后端限速的关系

WAF 是**第一道防线**（边缘/网关层），后端仍有 Phase 0 的滑动窗口限速
（`RATE_LIMIT_ENABLED` + `RATE_LIMIT_RPS`）与租户隔离中间件。两层独立、互不影响，
前者防恶意来源、后者防租户级突发，形成纵深防御。

## 上线建议

1. 先 `log` 模式跑 1–2 周，重点排查 `AI abuse` 与 `mass assignment` 误杀。
2. 提示注入规则针对已知模式；**不能替代** 后端 `CONTENT_MODERATION_ENABLED`
   的内容审核（异步、非阻塞）。WAF 拦截明显攻击，审核兜底语义层风险。
3. 规则应随业务迭代评审（纳入 Phase 4 的季度安全流程）。
