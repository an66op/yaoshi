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
}

func (s *recordingMemberPlanService) ActivateStream(_ context.Context, room uint64, position int, key string, historyLimits ...int) (services.PlanStreamDetail, error) {
	s.calls++
	s.room, s.position, s.key = room, position, key
	if len(historyLimits) > 0 {
		s.limit = historyLimits[0]
	}
	return services.PlanStreamDetail{}, nil
}

func (s *recordingMemberPlanService) ActivateGame(_ context.Context, room uint64, game string, limits ...int) (services.PlanDetail, error) {
	s.calls++
	s.room, s.game = room, game
	if len(limits) > 0 {
		s.limit = limits[0]
	}
	return services.PlanDetail{}, nil
}

func (s *recordingMemberPlanService) Detail(room uint64, game string, limits ...int) (services.PlanDetail, error) {
	s.reads++
	s.room, s.game = room, game
	if len(limits) > 0 {
		s.limit = limits[0]
	}
	return services.PlanDetail{}, nil
}

func (s *recordingMemberPlanService) StreamDetail(room uint64, position int, key string, limits ...int) (services.PlanStreamDetail, error) {
	s.reads++
	s.room, s.position, s.key = room, position, key
	if len(limits) > 0 {
		s.limit = limits[0]
	}
	return services.PlanStreamDetail{}, nil
}

func TestMemberPlanVisitAndHistoryContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		method, path, body string
		want, limit        int
	}{
		{"POST", "/plans/speed-fly/activate", `{}`, 200, 6},
		{"POST", "/plans/speed-fly/activate?history_limit=10", ``, 200, 10},
		{"POST", "/plans/speed-racing/activate", `{}`, 200, 6},
		{"POST", "/plans/speed-racing/activate?history_limit=11", `{}`, 400, 0},
		{"GET", "/plans/speed-racing?history_limit=10", ``, 200, 10},
		{"GET", "/plans/speed-fly", ``, 200, 6},
		{"GET", "/plans/speed-fly?history_limit=0", ``, 400, 0},
		{"GET", "/plans/speed-fly?history_limit=bad", ``, 400, 0},
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
			if test.want == 200 && strings.Contains(test.path, "speed-racing") && (service.position != 1 || service.key != services.DefaultPlanKey) {
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
		{name: "unsupported game selection", user: uint64(42), room: uint64(17), game: "speed-fly", body: `{"position":2}`, want: http.StatusBadRequest},
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
