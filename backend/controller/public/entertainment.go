package publicctrl

import (
	"backend/services"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type EntertainmentHandler struct {
	entertainment *services.EntertainmentAdminService
}

func NewEntertainmentHandler(db *gorm.DB) *EntertainmentHandler {
	return &EntertainmentHandler{entertainment: services.NewEntertainmentAdminService(db)}
}

// Portal validates a launch token and renders a bridge page for third-party entry.
func (h *EntertainmentHandler) Portal(c *gin.Context) {
	code := strings.TrimSpace(c.Query("code"))
	username := strings.TrimSpace(c.Query("user"))
	token := strings.TrimSpace(c.Query("token"))
	ts := strings.TrimSpace(c.Query("ts"))
	platform, err := h.entertainment.VerifyLaunchToken(code, username, ts, token)
	if err != nil {
		c.Data(http.StatusUnauthorized, "text/html; charset=utf-8", []byte(`<!doctype html><html lang="zh-CN"><body style="font-family:sans-serif;padding:24px"><h2>进入失败</h2><p>链接已失效或签名不正确，请返回钱包重新进入。</p></body></html>`))
		return
	}
	html := `<!doctype html>
<html lang="zh-CN">
<head><meta charset="utf-8"/><meta name="viewport" content="width=device-width,initial-scale=1"/>
<title>` + platform.Name + ` · 王者娱乐</title>
<style>
body{margin:0;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:linear-gradient(160deg,#0f2744,#1b98a7);color:#fff;min-height:100vh;display:grid;place-items:center;padding:24px}
.card{max-width:420px;width:100%;background:rgba(255,255,255,.08);border:1px solid rgba(255,255,255,.18);border-radius:18px;padding:28px;backdrop-filter:blur(8px)}
h1{margin:0 0 8px;font-size:24px}p{line-height:1.6;color:#dff7fb}small{opacity:.75}
.btn{display:inline-block;margin-top:18px;padding:12px 18px;border-radius:999px;background:#fff;color:#0f2744;text-decoration:none;font-weight:700}
</style></head>
<body><div class="card">
<h1>` + platform.Name + `</h1>
<p>帐号 <b>` + username + `</b> 已通过王者娱乐鉴权。</p>
<p>平台编号：<code>` + platform.Code + `</code></p>
<p><small>演示桥接页：生产环境请将 API 地址配置为第三方真实 launch URL。</small></p>
<a class="btn" href="javascript:history.back()">返回上一页</a>
</div></body></html>`
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}
