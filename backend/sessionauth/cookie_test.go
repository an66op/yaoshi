package sessionauth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTokenFromRequestPrefersScopedCookieAndKeepsBearerCompatibility(t *testing.T) {
	member := httptest.NewRequest(http.MethodGet, "/api/member/me", nil)
	member.Header.Set("Authorization", "Bearer legacy-token")
	member.AddCookie(&http.Cookie{Name: MemberCookieName, Value: "member-cookie"})
	member.AddCookie(&http.Cookie{Name: ManagementCookieName, Value: "management-cookie"})
	if token, fromCookie := TokenFromRequest(member); token != "member-cookie" || !fromCookie {
		t.Fatalf("member token = %q cookie=%v", token, fromCookie)
	}

	management := httptest.NewRequest(http.MethodGet, "/api/admin/me", nil)
	management.Header.Set("Authorization", "Bearer legacy-token")
	management.AddCookie(&http.Cookie{Name: MemberCookieName, Value: "member-cookie"})
	management.AddCookie(&http.Cookie{Name: ManagementCookieName, Value: "management-cookie"})
	if token, fromCookie := TokenFromRequest(management); token != "management-cookie" || !fromCookie {
		t.Fatalf("management token = %q cookie=%v", token, fromCookie)
	}

	bearer := httptest.NewRequest(http.MethodGet, "/api/admin/me", nil)
	bearer.Header.Set("Authorization", "Bearer legacy-token")
	if token, fromCookie := TokenFromRequest(bearer); token != "legacy-token" || fromCookie {
		t.Fatalf("bearer token = %q cookie=%v", token, fromCookie)
	}
}

func TestSessionCookieSecurityAttributes(t *testing.T) {
	cookie := NewCookie(ScopeManagement, "signed-token", true, 90*time.Minute)
	if cookie.Name != ManagementCookieName || cookie.Path != "/api" {
		t.Fatalf("unexpected cookie identity: %#v", cookie)
	}
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("missing cookie protections: %#v", cookie)
	}
	if cookie.MaxAge != 5400 {
		t.Fatalf("MaxAge = %d, want 5400", cookie.MaxAge)
	}

	expired := ExpiredCookie(ScopeMember, false)
	if expired.Name != MemberCookieName || expired.MaxAge >= 0 || expired.Value != "" || !expired.HttpOnly {
		t.Fatalf("invalid expired cookie: %#v", expired)
	}
}
