package middleware

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	// Creating a payment account can decode and re-encode a multi-megabyte
	// image. Keep this quota independent from ordinary betting and other member
	// writes so they cannot be used to replenish an upload budget.
	memberPaymentAccountCreateLimiter = newFixedWindowLimiter(10, time.Hour)
	// Private QR reads are independently bounded because each successful hit
	// streams a member-owned file. The key is authenticated workspace/member,
	// not a caller-controlled account id or client address.
	memberPaymentQRCodeReadLimiter = newFixedWindowLimiter(60, time.Minute)
	// Every failed old-password check invokes bcrypt. The per-member ceiling
	// cannot be replenished by rotating proxy addresses. A second, tighter
	// member-and-IP window slows retries from each trusted client address.
	memberPasswordChangeUserLimiter   = newFixedWindowLimiter(5, 15*time.Minute)
	memberPasswordChangeClientLimiter = newFixedWindowLimiter(3, 15*time.Minute)
)

// MemberPaymentAccountCreateRateLimit is shared across backend processes and
// scoped to the authenticated member in their server-resolved workspace.
func MemberPaymentAccountCreateRateLimit() gin.HandlerFunc {
	return memberPaymentAccountCreateLimiter.middlewareWithSubject("member-payment-account-create", func(c *gin.Context) string {
		return fmt.Sprintf("%d:%d", c.GetUint64("workspace_id"), c.GetUint64("user_id"))
	})
}

func MemberPaymentQRCodeReadRateLimit() gin.HandlerFunc {
	return memberPaymentQRCodeReadLimiter.middlewareWithSubject("member-payment-qr-read", func(c *gin.Context) string {
		return fmt.Sprintf("%d:%d", c.GetUint64("workspace_id"), c.GetUint64("user_id"))
	})
}

// MemberPasswordChangeRateLimit is the non-bypassable per-member ceiling.
func MemberPasswordChangeRateLimit() gin.HandlerFunc {
	return memberPasswordChangeUserLimiter.middlewareWithSubject("member-password-change-user", func(c *gin.Context) string {
		return fmt.Sprintf("%d", c.GetUint64("user_id"))
	})
}

// MemberPasswordChangeClientRateLimit adds a tighter member-and-client window
// using Gin's trusted-proxy-aware ClientIP. Different members behind one NAT
// remain isolated, while changing IP cannot replenish the per-member ceiling.
func MemberPasswordChangeClientRateLimit() gin.HandlerFunc {
	return memberPasswordChangeClientLimiter.middlewareWithSubject("member-password-change-client", func(c *gin.Context) string {
		return fmt.Sprintf("%d:%s", c.GetUint64("user_id"), c.ClientIP())
	})
}
