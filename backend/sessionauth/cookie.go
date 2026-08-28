package sessionauth

import (
	"net/http"
	"strings"
	"time"
)

type Scope string

const (
	ScopeManagement Scope = "management"
	ScopeMember     Scope = "member"

	ManagementCookieName = "wangzhe_management_session"
	MemberCookieName     = "wangzhe_member_session"
)

func CookieName(scope Scope) string {
	if scope == ScopeMember {
		return MemberCookieName
	}
	return ManagementCookieName
}

// ScopeForPath keeps management and member sessions independent on the local
// development backend, where both frontends share one host and port 8080.
func ScopeForPath(path string) Scope {
	if path == "/api/member" || strings.HasPrefix(path, "/api/member/") {
		return ScopeMember
	}
	return ScopeManagement
}

// TokenFromRequest prefers the HttpOnly session cookie. Authorization remains
// a migration path for existing non-browser clients, but it can never override
// a cookie already presented by the browser.
func TokenFromRequest(request *http.Request) (token string, fromCookie bool) {
	if request == nil {
		return "", false
	}
	if cookie, err := request.Cookie(CookieName(ScopeForPath(request.URL.Path))); err == nil {
		return strings.TrimSpace(cookie.Value), true
	}
	authorization := strings.TrimSpace(request.Header.Get("Authorization"))
	parts := strings.Fields(authorization)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return parts[1], false
	}
	return authorization, false
}

func NewCookie(scope Scope, token string, secure bool, lifetime time.Duration) *http.Cookie {
	maxAge := int(lifetime / time.Second)
	if maxAge < 1 {
		maxAge = 1
	}
	return &http.Cookie{
		Name:     CookieName(scope),
		Value:    token,
		Path:     "/api",
		MaxAge:   maxAge,
		Expires:  time.Now().UTC().Add(lifetime),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
}

func ExpiredCookie(scope Scope, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     CookieName(scope),
		Value:    "",
		Path:     "/api",
		MaxAge:   -1,
		Expires:  time.Unix(1, 0).UTC(),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
}
