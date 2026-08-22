package constants

import (
	"backend/errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

type response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// 成功返回
func SendSuccess(c *gin.Context, code int, message string, data interface{}) {
	resp := response{
		Code:    code,
		Message: message,
		Data:    data,
	}
	c.JSON(code, resp)
}

// SendError 错误返回 - 自动识别业务错误和系统错误
func SendError(c *gin.Context, defaultCode int, defaultMessage string, err error) {
	if err == nil {
		c.JSON(defaultCode, response{
			Code:    defaultCode,
			Message: defaultMessage,
		})
		return
	}

	// 检查是否是自定义错误类型
	if appErr, ok := err.(*errors.AppError); ok {
		httpCode := http.StatusInternalServerError
		message := appErr.Message

		// 根据错误类型设置HTTP状态码
		if appErr.Type == errors.ErrTypeBusiness {
			// 业务错误：根据错误代码设置合适的状态码
			switch appErr.Code {
			case "USERNAME_EXISTS", "EMAIL_EXISTS":
				httpCode = http.StatusConflict // 409
			case "USER_NOT_FOUND", "INVALID_CREDENTIALS", "USER_DISABLED":
				httpCode = http.StatusUnauthorized // 401
			case "FORBIDDEN", "ADMIN_REQUIRED":
				httpCode = http.StatusForbidden // 403
			case "DRAW_NOT_FOUND", "GAME_NOT_FOUND", "CHANNEL_NOT_FOUND":
				httpCode = http.StatusNotFound
			case "INSUFFICIENT_BALANCE":
				httpCode = http.StatusBadRequest
			case "INVALID_REQUEST":
				httpCode = http.StatusBadRequest // 400
			default:
				httpCode = http.StatusBadRequest // 400
			}
		} else {
			// 系统错误：记录日志但不暴露详细错误给客户端
			log.Printf("System Error [%s]: %v", appErr.Code, appErr.Err)
			httpCode = http.StatusInternalServerError // 500
			message = "系统内部错误，请稍后重试"                  // 不暴露真实错误信息
		}

		c.JSON(httpCode, response{
			Code:    httpCode,
			Message: message,
		})
		return
	}

	// 兼容旧代码：普通错误
	httpCode := defaultCode
	message := defaultMessage

	// 如果错误包含错误信息，记录日志但不暴露给客户端
	log.Printf("Error: %v", err)
	if defaultCode >= 500 {
		// 系统错误，不暴露详细错误
		message = "系统内部错误，请稍后重试"
	}

	c.JSON(httpCode, response{
		Code:    httpCode,
		Message: message,
	})
}
