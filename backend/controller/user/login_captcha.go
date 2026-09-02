package user

import (
	"backend/captcha"
	"backend/constants"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

func noLoginCache(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
}

func LoginCaptcha(purpose string) gin.HandlerFunc {
	return func(c *gin.Context) {
		noLoginCache(c)
		challenge, err := captcha.Create(c.Request.Context(), purpose, c.ClientIP())
		if err != nil {
			constants.SendError(c, http.StatusServiceUnavailable, "验证码服务暂不可用，请稍后重试", nil)
			return
		}
		constants.SendSuccess(c, http.StatusOK, "ok", challenge)
	}
}

func verifyLoginCaptcha(c *gin.Context, purpose, id, code string) bool {
	err := captcha.Verify(c.Request.Context(), purpose, c.ClientIP(), id, code)
	if err == nil {
		return true
	}
	if errors.Is(err, captcha.ErrUnavailable) {
		constants.SendError(c, http.StatusServiceUnavailable, "验证码服务暂不可用，请稍后重试", nil)
	} else {
		constants.SendError(c, http.StatusBadRequest, "验证码错误或已过期，请刷新后重试", nil)
	}
	return false
}
