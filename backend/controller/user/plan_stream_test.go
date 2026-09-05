package user

import (
	"backend/services"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type recordingMemberPlanService struct {
	memberPlanService
	calls    int
	room     uint64
	position int
	key      string
	game     string
	limit    int
	reads    int
	user     uint64
	detail   services.PlanDetail
	stream   services.PlanStreamDetail
}

func (s *recordingMemberPlanService) ActivateStreamForMember(_ context.Context, user, room uint64, game string, position int, key string, historyLimits ...int) (services.PlanStreamDetail, error) {
	s.calls++
	s.user, s.room, s.game, s.position, s.key = user, room, game, position, key
	if len(historyLimits) > 0 {
		s.limit = historyLimits[0]
	}
	return s.stream, nil
}

func (s *recordingMemberPlanService) ActivateGameForMember(_ context.Context, user, room uint64, game string, limits ...int) (services.PlanDetail, error) {
	s.calls++
	s.user, s.room, s.game = user, room, game
	if len(limits) > 0 {
		s.limit = limits[0]
	}
	return s.detail, nil
}

func (s *recordingMemberPlanService) Detail(room uint64, game string, limits ...int) (services.PlanDetail, error) {
	s.reads++
	s.room, s.game = room, game
	if len(limits) > 0 {
		s.limit = limits[0]
	}
	return s.detail, nil
}

func (s *recordingMemberPlanService) StreamDetailForGame(room uint64, game string, position int, key string, limits ...int) (services.PlanStreamDetail, error) {
	s.reads++
	s.room, s.game, s.position, s.key = room, game, position, key
	if len(limits) > 0 {
		s.limit = limits[0]
	}
	return s.stream, nil
}

func TestMemberPlanGETReturnsMetadataWithoutRecommendationPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := services.PlanRecommendationView{ID: 99, GameID: "speed-ssc", Issue: "secret-issue", MasterName: "未登记查看的专家", Numbers: []int{1, 3, 7}}
	for _, test := range []struct {
		name, path string
		service    *recordingMemberPlanService
	}{
		{name: "generic", path: "/plans/speed-ssc", service: &recordingMemberPlanService{detail: services.PlanDetail{GameID: "speed-ssc", CurrentIssue: "visible-metadata", Recommendations: []services.PlanRecommendationView{secret}, LatestRecommendations: []services.PlanRecommendationView{secret}, History: []services.PlanRecommendationView{secret}}}},
		{name: "racing", path: "/plans/speed-racing?position=1&plan_key=four-period-five-codes", service: &recordingMemberPlanService{stream: services.PlanStreamDetail{PlanDetail: services.PlanDetail{GameID: "speed-racing", CurrentIssue: "visible-metadata", Recommendations: []services.PlanRecommendationView{secret}, LatestRecommendations: []services.PlanRecommendationView{secret}, History: []services.PlanRecommendationView{secret}}, LegacyHistory: []services.PlanRecommendationView{secret}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := &memberHandler{plans: test.service}
			engine := gin.New()
			engine.Use(func(c *gin.Context) { c.Set("user_id", uint64(42)); c.Set("workspace_id", uint64(17)) })
			engine.GET("/plans/:gameID", handler.PlanDetail)
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			body := response.Body.String()
			if response.Code != http.StatusOK || strings.Contains(body, secret.MasterName) || strings.Contains(body, secret.Issue) || !strings.Contains(body, `"current_issue":"visible-metadata"`) {
				t.Fatalf("GET exposed recommendation content: status=%d body=%s", response.Code, body)
			}
			for _, empty := range []string{`"recommendations":[]`, `"latest_recommendations":[]`, `"history":[]`} {
				if !strings.Contains(body, empty) {
					t.Fatalf("metadata response missing redacted %s: %s", empty, body)
				}
			}
			if test.name == "racing" && !strings.Contains(body, `"legacy_history":[]`) {
				t.Fatalf("racing metadata exposed legacy content: %s", body)
			}
		})
	}
}

func TestMemberPlanPOSTReturnsOnlyAuditedRecommendationPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := services.PlanRecommendationView{ID: 99, GameID: "speed-ssc", Issue: "audited-issue", MasterName: "已登记查看的专家", Numbers: []int{1, 3, 7}}
	for _, test := range []struct {
		name, path, body string
		service          *recordingMemberPlanService
	}{
		{name: "generic", path: "/plans/speed-ssc/activate", body: `{}`, service: &recordingMemberPlanService{detail: services.PlanDetail{GameID: "speed-ssc", Recommendations: []services.PlanRecommendationView{secret}}}},
		{name: "racing", path: "/plans/speed-racing/activate", body: `{"position":1,"plan_key":"four-period-five-codes"}`, service: &recordingMemberPlanService{stream: services.PlanStreamDetail{PlanDetail: services.PlanDetail{GameID: "speed-racing", Recommendations: []services.PlanRecommendationView{secret}}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := &memberHandler{plans: test.service}
			engine := gin.New()
			engine.Use(func(c *gin.Context) { c.Set("user_id", uint64(42)); c.Set("workspace_id", uint64(17)) })
			engine.POST("/plans/:gameID/activate", handler.ActivatePlanStream)
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), secret.MasterName) || test.service.calls != 1 || test.service.user != 42 {
				t.Fatalf("audited POST response missing: status=%d calls=%d user=%d body=%s", response.Code, test.service.calls, test.service.user, response.Body.String())
			}
		})
	}
}

func TestMemberPlanVisitAndHistoryContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		method, path, body string
		want, limit        int
	}{
		{"POST", "/plans/speed-ssc/activate", `{}`, 200, 6},
		{"POST", "/plans/speed-fly/activate?history_limit=10", ``, 200, 10},
		{"POST", "/plans/speed-racing/activate", `{}`, 200, 6},
		{"POST", "/plans/speed-racing/activate?history_limit=11", `{}`, 400, 0},
		{"GET", "/plans/speed-racing?history_limit=10", ``, 200, 10},
		{"GET", "/plans/speed-ssc", ``, 200, 6},
		{"GET", "/plans/speed-ssc?history_limit=0", ``, 400, 0},
		{"GET", "/plans/speed-ssc?history_limit=bad", ``, 400, 0},
	} {
		t.Run(test.method+test.path, func(t *testing.T) {
			service := &recordingMemberPlanService{}
			handler := &memberHandler{plans: service}
			engine := gin.New()
			engine.Use(func(c *gin.Context) { c.Set("user_id", uint64(42)); c.Set("workspace_id", uint64(17)) })
			engine.GET("/plans/:gameID", handler.PlanDetail)
			engine.POST("/plans/:gameID/activate", handler.ActivatePlanStream)
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			if response.Code != test.want || service.limit != test.limit {
				t.Fatalf("status=%d limit=%d body=%s", response.Code, service.limit, response.Body.String())
			}
			if test.want != 200 && service.calls+service.reads != 0 {
				t.Fatal("invalid limit reached service")
			}
			if test.method == "GET" && service.calls != 0 {
				t.Fatal("GET generated data")
			}
			if test.want == 200 && service.room != 17 {
				t.Fatal("lost session room")
			}
			if test.want == 200 && service.calls > 0 && service.user != 42 {
				t.Fatal("lost authenticated member identity")
			}
			if test.want == 200 && (strings.Contains(test.path, "speed-racing") || strings.Contains(test.path, "speed-fly")) && (service.position != 1 || service.key != services.DefaultPlanKey) {
				t.Fatal("default selection changed")
			}
		})
	}
}

func TestActivatePlanStreamUsesAuthenticatedRoomAndValidatesBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name string
		user any
		room any
		game string
		body string
		want int
	}{
		{name: "not authenticated", room: uint64(17), game: "speed-racing", body: `{}`, want: http.StatusUnauthorized},
		{name: "room absent", user: uint64(42), game: "speed-racing", body: `{"workspace_id":999,"position":1,"plan_key":"four-period-five-codes"}`, want: http.StatusForbidden},
		{name: "room wrong type", user: uint64(42), room: "17", game: "speed-racing", body: `{}`, want: http.StatusForbidden},
		{name: "room zero", user: uint64(42), room: uint64(0), game: "speed-racing", body: `{}`, want: http.StatusForbidden},
		{name: "unsupported game selection", user: uint64(42), room: uint64(17), game: "speed-ssc", body: `{"position":2}`, want: http.StatusBadRequest},
		{name: "invalid JSON", user: uint64(42), room: uint64(17), game: "speed-racing", body: `{"position":`, want: http.StatusBadRequest},
		{name: "body cannot select room", user: uint64(42), room: uint64(17), game: "speed-racing", body: `{"workspace_id":999,"position":2,"plan_key":"three-period-six-codes"}`, want: http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			plans := &recordingMemberPlanService{}
			handler := &memberHandler{plans: plans}
			engine := gin.New()
			engine.Use(func(c *gin.Context) {
				if test.user != nil {
					c.Set("user_id", test.user)
				}
				if test.room != nil {
					c.Set("workspace_id", test.room)
				}
			})
			engine.POST("/plans/:gameID/activate", handler.ActivatePlanStream)
			request := httptest.NewRequest(http.MethodPost, "/plans/"+test.game+"/activate", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.want, response.Body.String())
			}
			if test.want == http.StatusOK {
				if plans.calls != 1 || plans.room != 17 || plans.position != 2 || plans.key != "three-period-six-codes" {
					t.Fatalf("untrusted payload changed room or selection: %#v", plans)
				}
			} else if plans.calls != 0 {
				t.Fatalf("invalid boundary reached service: %#v", plans)
			}
		})
	}
}
