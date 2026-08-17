package handlers

import (
	"fmt"
	"html/template"
	"net/http"

	"github.com/allcallall/backend/internal/auth"
	"github.com/gin-gonic/gin"
)

func (h *CommercialHandler) handleCurrentLegal(c *gin.Context) {
	JSONSuccess(c, http.StatusOK, gin.H{"legal": h.commerce.CurrentLegal()})
}

func (h *CommercialHandler) renderLegalPage(c *gin.Context, title string, body template.HTML) {
	legal := h.commerce.CurrentLegal()
	page := fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%s</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background:#f8fafc; color:#0f172a; margin:0; }
    main { max-width: 860px; margin: 0 auto; padding: 48px 20px 80px; }
    h1 { font-size: 32px; margin-bottom: 8px; }
    h2 { margin-top: 28px; font-size: 20px; }
    p, li { line-height: 1.75; color:#334155; }
    .meta { color:#64748b; margin-bottom: 24px; }
    .card { background:#fff; border-radius: 18px; padding: 24px; box-shadow: 0 8px 30px rgba(15,23,42,0.06); }
    a { color:#2563eb; }
  </style>
</head>
<body>
  <main>
    <h1>%s</h1>
    <p class="meta">AllCallAll · 联系邮箱 %s</p>
    <div class="card">%s</div>
  </main>
</body>
</html>`, title, title, legal.SupportEmail, body)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(page))
}

func (h *CommercialHandler) handleTermsPage(c *gin.Context) {
	body := template.HTML(`
<p>这些条款适用于 AllCallAll Android 首发版本。使用服务即代表你同意遵守以下规则。</p>
<h2>服务内容</h2>
<p>AllCallAll 提供 1 对 1 音视频通话、实时翻译、联系人管理、来电推送和与这些能力直接相关的付费权益。</p>
<h2>账号与安全</h2>
<p>你需要提供真实可访问的邮箱，并对账号下的行为负责。禁止冒充、骚扰、垃圾信息、诈骗和其他违法或侵权行为。</p>
<h2>订阅与付费</h2>
<p>Premium 订阅通过 Google Play 计费。购买是否生效以服务端 entitlement 状态为准。取消自动续费后，已支付周期内的权益会持续到到期日。</p>
<h2>可接受使用</h2>
<p>我们可以限制、冻结或删除违反条款的账号，并保留必要的非 PII 审计记录以处理安全、合规和支持问题。</p>
<h2>联系我们</h2>
<p>如果你对条款、订阅或账号状态有疑问，请联系支持邮箱。</p>`)
	h.renderLegalPage(c, "AllCallAll 服务条款", body)
}

func (h *CommercialHandler) handlePrivacyPage(c *gin.Context) {
	body := template.HTML(`
<p>AllCallAll 致力于最小化收集和最小化保留用户数据。</p>
<h2>我们收集什么</h2>
<ul>
  <li>账号信息：邮箱、显示名称、加密后的密码摘要</li>
  <li>服务运行数据：联系人、通话历史摘要、订阅 entitlement、翻译配额用量、推送 token</li>
  <li>支持与安全数据：黑名单关系、举报记录、删除审计摘要</li>
</ul>
<h2>我们不做什么</h2>
<ul>
  <li>不长期保存原始通话音频</li>
  <li>不将实时翻译结果做长期转写归档</li>
  <li>不在客户端本地持久化敏感认证 token 以外的会话密钥</li>
</ul>
<h2>我们如何使用数据</h2>
<p>这些数据只用于账号登录、通话连接、权益判断、推送送达、滥用防护和支持排查。</p>
<h2>删除与你的权利</h2>
<p>你可以在应用内发起账号删除。删除后会清除可识别的账号数据，只保留非 PII 删除审计摘要。</p>`)
	h.renderLegalPage(c, "AllCallAll 隐私政策", body)
}

func (h *CommercialHandler) handleDeleteAccountPage(c *gin.Context) {
	legal := h.commerce.CurrentLegal()
	body := template.HTML(fmt.Sprintf(`
<p>你可以在应用内的“设置 -> 删除账号”入口发起账号删除。</p>
<h2>删除会清除的数据</h2>
<ul>
  <li>账号邮箱与显示名称会被去标识化</li>
  <li>联系人关系、通话历史、FCM token、翻译配额、订阅 entitlement 记录会被清理</li>
  <li>邮箱验证码记录与法律接受记录会被清理</li>
</ul>
<h2>删除后仍会保留的内容</h2>
<p>为满足合规和支持排查，我们只保留不含可逆个人信息的删除审计摘要，例如删除时间和受影响记录数量。</p>
<h2>处理时效</h2>
<p>应用内删除流程成功后会立即生效。如需人工帮助，请联系 %s。</p>`, legal.SupportEmail))
	h.renderLegalPage(c, "AllCallAll 账号删除说明", body)
}

func (h *CommercialHandler) handleAcceptLegal(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := h.commerce.AcceptLegal(c.Request.Context(), claims.UserID); err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("accept legal failed")
		JSONError(c, http.StatusInternalServerError, "failed to accept legal documents")
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"message": "legal acceptance recorded"})
}
