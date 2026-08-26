package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mushroomyuan/vpp-backend/alarm/application"
	"github.com/mushroomyuan/vpp-backend/alarm/application/command"
	"github.com/mushroomyuan/vpp-backend/alarm/application/query"
	"github.com/mushroomyuan/vpp-backend/alarm/domain"
	"github.com/mushroomyuan/vpp-backend/alarm/domain/model"
	"github.com/mushroomyuan/vpp-backend/platform/authn/casdoor"
)

type stubList struct {
	last query.ListAlarms
	res  *query.ListAlarmsResult
	err  error
}

func (s *stubList) Handle(_ context.Context, q query.ListAlarms) (*query.ListAlarmsResult, error) {
	s.last = q
	if s.res == nil {
		return &query.ListAlarmsResult{}, s.err
	}
	return s.res, s.err
}

type stubGet struct {
	res *query.GetAlarmResult
	err error
}

func (s *stubGet) Handle(_ context.Context, _ query.GetAlarm) (*query.GetAlarmResult, error) {
	return s.res, s.err
}

type stubAck struct {
	last command.Acknowledge
	res  *command.AcknowledgeResult
	err  error
}

func (s *stubAck) Handle(_ context.Context, cmd command.Acknowledge) (*command.AcknowledgeResult, error) {
	s.last = cmd
	return s.res, s.err
}

type stubClose struct {
	last command.Close
	res  *command.CloseResult
	err  error
}

func (s *stubClose) Handle(_ context.Context, cmd command.Close) (*command.CloseResult, error) {
	s.last = cmd
	return s.res, s.err
}

func sampleAlarm(id, tenant string) *model.Alarm {
	a, err := model.NewOpenAlarm(id, model.Decision{
		TenantID: tenant, Source: model.SourceSOE, Severity: model.SeverityWarning,
		RuleID: model.RuleSOEDiscreteChange, Title: "cu/brk 变位", Fingerprint: "v1:fp",
		EventID: "e1", SourceRef: "cu/brk", AttributesSchema: model.AttributesSchemaV1,
		OccurredAt: time.Unix(10, 0).UTC(),
	})
	if err != nil {
		panic(err)
	}
	return a
}

func testApp(list *stubList, get *stubGet, ack *stubAck, clo *stubClose) application.Application {
	if list == nil {
		list = &stubList{}
	}
	if get == nil {
		get = &stubGet{}
	}
	if ack == nil {
		ack = &stubAck{}
	}
	if clo == nil {
		clo = &stubClose{}
	}
	return application.Application{
		Commands: application.Commands{Acknowledge: ack, Close: clo},
		Queries:  application.Queries{ListAlarms: list, GetAlarm: get},
	}
}

func TestListAlarms_SnakeCaseAndTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	list := &stubList{res: &query.ListAlarmsResult{
		Alarms: []*model.Alarm{sampleAlarm("a1", "t1")},
		Total:  1,
	}}
	r := gin.New()
	RegisterRoutes(r, testApp(list, nil, nil, nil), nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/t1/alarms?status=open&limit=10", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	if list.last.TenantID != "t1" || list.last.Status != "open" || list.last.Limit != 10 {
		t.Fatalf("%+v", list.last)
	}
	if !strings.Contains(w.Body.String(), `"tenant_id":"t1"`) || !strings.Contains(w.Body.String(), `"rule_id"`) {
		t.Fatalf("body=%s", w.Body.String())
	}
}

func TestGetAlarm_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	get := &stubGet{err: domain.ErrNotFound}
	r := gin.New()
	RegisterRoutes(r, testApp(nil, get, nil, nil), nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/t1/alarms/missing", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestAck_ConflictIncludesVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cur := sampleAlarm("a1", "t1")
	cur.Version = 7
	ack := &stubAck{err: domain.ErrConflict}
	get := &stubGet{res: &query.GetAlarmResult{Alarm: cur}}
	r := gin.New()
	RegisterRoutes(r, testApp(nil, get, ack, nil), nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/t1/alarms/a1/ack", strings.NewReader(`{"version":1}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["version"] != float64(7) {
		t.Fatalf("%v", body)
	}
}

func TestAck_ActorFromPrincipalNotBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ack := &stubAck{res: &command.AcknowledgeResult{Alarm: sampleAlarm("a1", "default")}}
	r := gin.New()
	RegisterRoutes(r, testApp(nil, nil, ack, nil), AuthMiddleware(
		AuthConfig{TrustProxyHeaders: true},
		casdoor.ParseUserinfo,
		mustHealthyChecker(t),
	))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/default/alarms/a1/ack",
		bytes.NewReader([]byte(`{"version":1,"actor":"mallory"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Userinfo", encodeUserinfo(t, "default", "operator"))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	if ack.last.Actor != "operator" {
		t.Fatalf("actor=%q", ack.last.Actor)
	}
	if ack.last.Version != 1 {
		t.Fatalf("version=%d", ack.last.Version)
	}
}

func TestNoCreateAlarmRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterRoutes(r, testApp(nil, nil, nil, nil), nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/t1/alarms", strings.NewReader(`{}`))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("create must not exist, code=%d", w.Code)
	}
}

func TestMapHTTPError(t *testing.T) {
	t.Parallel()
	status, _ := mapHTTPError(domain.ErrNotFound)
	if status != http.StatusNotFound {
		t.Fatal(status)
	}
	status, _ = mapHTTPError(domain.ErrInvalidFilter)
	if status != http.StatusBadRequest {
		t.Fatal(status)
	}
}
