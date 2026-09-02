package user

import (
	"backend/captcha"
	"backend/cluster"
	"backend/config"
	usermodel "backend/data/models/user"
	"backend/services"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
)

type captchaAuthProbe struct {
	services.AuthService
	calls int
}

func (s *captchaAuthProbe) Login(_, _, _ string) (*usermodel.User, string, error) {
	s.calls++
	return &usermodel.User{UserID: 1, Role: "tenant", Status: 1}, "test-session", nil
}
func (s *captchaAuthProbe) LoginMember(_, _, _ string) (*usermodel.User, string, error) {
	s.calls++
	return &usermodel.User{UserID: 2, Role: "member", Status: 1}, "test-session", nil
}

func TestLoginCaptchaRequiredBeforeAuthenticationInEveryEnvironment(t *testing.T) {
	previous := config.Config
	t.Cleanup(func() { config.Config = previous; _ = cluster.Init(context.Background(), cluster.Options{}) })
	gin.SetMode(gin.TestMode)
	for _, mode := range []string{"debug", "test", "release"} {
		config.Config = &config.Configuration{Server: config.ServerConfig{Mode: mode}}
		for _, purpose := range []string{captcha.Management, captcha.Member} {
			t.Run(mode+"/"+purpose, func(t *testing.T) {
				// A nil service/DB deliberately panics if captcha protection is
				// bypassed. Missing/malformed challenges must never reach either.
				handler := (&authHandler{}).Login
				if purpose == captcha.Member {
					handler = (&memberHandler{}).Login
				}
				for _, suffix := range []string{"", `,"captcha_id":"invalid","captcha_code":"123456"`} {
					engine := gin.New()
					engine.POST("/login", handler)
					request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"username":"someone","password":"Password#2026"`+suffix+`}`))
					request.Header.Set("Content-Type", "application/json")
					response := httptest.NewRecorder()
					engine.ServeHTTP(response, request)
					if response.Code != 400 || !strings.Contains(response.Body.String(), "验证码") {
						t.Fatalf("captcha not enforced: %d %s", response.Code, response.Body.String())
					}
					if response.Header().Get("Cache-Control") != "no-store" {
						t.Fatal("login error is cacheable")
					}
				}
			})
		}
	}
}

func TestLoginCaptchaSuccessConsumedBeforeAuthAndErrorsFailClosed(t *testing.T) {
	previous := config.Config
	config.Config = &config.Configuration{Server: config.ServerConfig{Mode: "release"}, JWT: config.JWTConfig{Expire: 3600}}
	t.Cleanup(func() { config.Config = previous; _ = cluster.Init(context.Background(), cluster.Options{}) })
	server := miniredis.RunT(t)
	if err := cluster.Init(context.Background(), cluster.Options{Addr: server.Addr(), Prefix: "login-captcha-handler", Required: true}); err != nil {
		t.Fatal(err)
	}
	for _, purpose := range []string{captcha.Management, captcha.Member} {
		t.Run(purpose, func(t *testing.T) {
			const id, ip, code = "0123456789abcdef0123456789abcdef", "192.0.2.1", "012345"
			seed := func() {
				sum := sha256.Sum256([]byte(id + "\x00" + purpose + "\x00" + ip + "\x00" + code))
				server.Set(cluster.Key("captcha", id), hex.EncodeToString(sum[:]))
				server.SetTTL(cluster.Key("captcha", id), captcha.Lifetime)
			}
			probe := &captchaAuthProbe{}
			handler := (&authHandler{authService: probe}).Login
			if purpose == captcha.Member {
				handler = (&memberHandler{auth: probe}).Login
			}
			engine := gin.New()
			engine.POST("/login", handler)
			request := func(answer string) *httptest.ResponseRecorder {
				payload, _ := json.Marshal(map[string]string{"username": "someone", "password": "Password#2026", "captcha_id": id, "captcha_code": answer})
				r := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(string(payload)))
				r.RemoteAddr = ip + ":1234"
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				engine.ServeHTTP(w, r)
				return w
			}
			seed()
			if response := request("999999"); response.Code != 400 || probe.calls != 0 {
				t.Fatal("wrong captcha reached password check")
			}
			if response := request(code); response.Code != 400 || probe.calls != 0 {
				t.Fatal("wrong-answer challenge was not consumed")
			}
			seed()
			if response := request(code); response.Code != 200 || probe.calls != 1 || len(response.Result().Cookies()) != 1 {
				t.Fatalf("valid captcha failed: %d %s", response.Code, response.Body.String())
			}
			if response := request(code); response.Code != 400 || probe.calls != 1 {
				t.Fatal("successful captcha replay reached auth")
			}
			seed()
			server.SetError("ERR unavailable")
			if response := request(code); response.Code != 503 || probe.calls != 1 {
				t.Fatal("Redis failure did not fail closed")
			}
			server.SetError("")
		})
	}
}

func TestLoginCaptchaGETReturnsUncacheablePNG(t *testing.T) {
	previous := config.Config
	config.Config = &config.Configuration{Server: config.ServerConfig{Mode: "test"}}
	_ = cluster.Init(context.Background(), cluster.Options{})
	t.Cleanup(func() { config.Config = previous; _ = cluster.Init(context.Background(), cluster.Options{}) })
	engine := gin.New()
	engine.GET("/captcha", LoginCaptcha(captcha.Member))
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/captcha", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "data:image/png;base64,") || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Pragma") != "no-cache" {
		t.Fatalf("invalid challenge response: %d", w.Code)
	}
	config.Config.Server.Mode = "release"
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/captcha", nil))
	if w.Code != 503 {
		t.Fatal("release created challenge without shared store")
	}
}
