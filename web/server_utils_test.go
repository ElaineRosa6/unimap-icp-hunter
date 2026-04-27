package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseEnginesParam_MultipleValues(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?engines=fofa&engines=hunter", nil)
	got := parseEnginesParam(req)
	if len(got) != 2 {
		t.Fatalf("expected 2 engines, got %d: %v", len(got), got)
	}
}

func TestParseEnginesParam_CommaSeparated(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?engines=fofa,hunter,zoomeye", nil)
	got := parseEnginesParam(req)
	if len(got) != 3 {
		t.Fatalf("expected 3 engines, got %d: %v", len(got), got)
	}
}

func TestParseWSStringList_String(t *testing.T) {
	got := parseWSStringList("a, b, c")
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("expected [a b c], got %v", got)
	}
}

func TestParseWSStringList_StringSlice(t *testing.T) {
	got := parseWSStringList([]string{"a, b", "c"})
	if len(got) != 3 {
		t.Fatalf("expected 3 items, got %d: %v", len(got), got)
	}
}

func TestParseWSStringList_InterfaceSlice(t *testing.T) {
	got := parseWSStringList([]interface{}{"a, b", 123})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("expected [a b], got %v", got)
	}
}

func TestParseWSStringList_Nil(t *testing.T) {
	got := parseWSStringList(nil)
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestParseWSStringList_UnknownType(t *testing.T) {
	got := parseWSStringList(42)
	if got != nil {
		t.Fatalf("expected nil for int, got %v", got)
	}
}

func TestParseWSInt_Float64(t *testing.T) {
	got := parseWSInt(float64(42), 0)
	if got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
}

func TestParseWSInt_Int(t *testing.T) {
	got := parseWSInt(42, 0)
	if got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
}

func TestParseWSInt_String(t *testing.T) {
	got := parseWSInt("42", 0)
	if got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
}

func TestParseWSInt_Nil(t *testing.T) {
	got := parseWSInt(nil, 10)
	if got != 10 {
		t.Fatalf("expected 10, got %d", got)
	}
}

func TestParseWSInt_NegativeFloat(t *testing.T) {
	got := parseWSInt(float64(-1), 5)
	if got != 5 {
		t.Fatalf("expected default 5, got %d", got)
	}
}

func TestParseWSInt_EmptyString(t *testing.T) {
	got := parseWSInt("", 7)
	if got != 7 {
		t.Fatalf("expected default 7, got %d", got)
	}
}

func TestParseWSInt_InvalidString(t *testing.T) {
	got := parseWSInt("abc", 3)
	if got != 3 {
		t.Fatalf("expected default 3, got %d", got)
	}
}

func TestValidateQueryInput_AllowedControlChars(t *testing.T) {
	if err := validateQueryInput("test\tquery\n"); err != nil {
		t.Fatalf("expected no error for tab/newline, got %v", err)
	}
}

func TestParseBoolValue_TrueVariants(t *testing.T) {
	for _, v := range []string{"1", "true", "True", "TRUE", "yes", "on"} {
		if !parseBoolValue(v) {
			t.Fatalf("expected true for %q", v)
		}
	}
}

func TestParseBoolValue_FalseVariants(t *testing.T) {
	for _, v := range []string{"0", "false", "no", "off", "", "anything"} {
		if parseBoolValue(v) {
			t.Fatalf("expected false for %q", v)
		}
	}
}

func TestParseWSBool_Bool(t *testing.T) {
	if !parseWSBool(true) {
		t.Fatal("expected true for true")
	}
	if parseWSBool(false) {
		t.Fatal("expected false for false")
	}
}

func TestParseWSBool_String(t *testing.T) {
	if !parseWSBool("true") {
		t.Fatal("expected true for 'true'")
	}
	if parseWSBool("false") {
		t.Fatal("expected false for 'false'")
	}
}

func TestParseWSBool_Float64(t *testing.T) {
	if !parseWSBool(float64(1)) {
		t.Fatal("expected true for 1.0")
	}
	if parseWSBool(float64(0)) {
		t.Fatal("expected false for 0.0")
	}
}

func TestParseWSBool_Int(t *testing.T) {
	if !parseWSBool(1) {
		t.Fatal("expected true for int 1")
	}
	if parseWSBool(0) {
		t.Fatal("expected false for int 0")
	}
}

func TestParseWSBool_UnknownType(t *testing.T) {
	if parseWSBool(struct{}{}) {
		t.Fatal("expected false for unknown type")
	}
}

func TestAppendUniqueStrings_BaseOnly(t *testing.T) {
	got := appendUniqueStrings([]string{"a", "b"}, nil)
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("expected [a b], got %v", got)
	}
}

func TestAppendUniqueStrings_WithExtra(t *testing.T) {
	got := appendUniqueStrings([]string{"a"}, []string{"b", "c"})
	if len(got) != 3 {
		t.Fatalf("expected 3 items, got %d: %v", len(got), got)
	}
}

func TestAppendUniqueStrings_Dedup(t *testing.T) {
	got := appendUniqueStrings([]string{"a"}, []string{"a", "b"})
	if len(got) != 2 {
		t.Fatalf("expected 2 items (dedup), got %d: %v", len(got), got)
	}
}

func TestAppendUniqueStrings_SkipsEmpty(t *testing.T) {
	got := appendUniqueStrings([]string{"a", ""}, []string{"", "b"})
	if len(got) != 2 {
		t.Fatalf("expected 2 items (skip empty), got %d: %v", len(got), got)
	}
}

func TestAppendUniqueStrings_EmptyInputs(t *testing.T) {
	got := appendUniqueStrings(nil, nil)
	if len(got) != 0 {
		t.Fatalf("expected 0 items, got %d", len(got))
	}
}

func TestMaskAPIKey_Short(t *testing.T) {
	got := maskAPIKey("abc")
	if got != "****" {
		t.Fatalf("expected ****, got %q", got)
	}
}

func TestMaskAPIKey_Exact8(t *testing.T) {
	got := maskAPIKey("12345678")
	if got != "****" {
		t.Fatalf("expected ****, got %q", got)
	}
}

func TestMaskAPIKey_Long(t *testing.T) {
	got := maskAPIKey("abcdef1234567890")
	if got != "abcd****7890" {
		t.Fatalf("expected abcd****7890, got %q", got)
	}
}

func TestMaskAPIKey_Empty(t *testing.T) {
	got := maskAPIKey("")
	if got != "****" {
		t.Fatalf("expected ****, got %q", got)
	}
}

func TestSecurityMiddleware_SetsHeaders(t *testing.T) {
	handler := securityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	expectedHeaders := map[string]string{
		"X-Frame-Options":        "DENY",
		"X-Content-Type-Options": "nosniff",
		"X-XSS-Protection":       "1; mode=block",
	}
	for header, expected := range expectedHeaders {
		if rec.Header().Get(header) != expected {
			t.Fatalf("expected %s=%q, got %q", header, expected, rec.Header().Get(header))
		}
	}
}

func TestBindAddr_Default(t *testing.T) {
	s := &Server{config: nil}
	if got := s.bindAddr(); got != "127.0.0.1" {
		t.Fatalf("expected 127.0.0.1, got %q", got)
	}
}

func TestNewRouter_CreatesRouter(t *testing.T) {
	s := &Server{}
	r := NewRouter(s)
	if r == nil {
		t.Fatal("expected non-nil router")
	}
	if r.server != s {
		t.Fatal("expected router.server to match")
	}
}

func TestRouter_GetRoutes_Empty(t *testing.T) {
	r := &Router{routes: []Route{}}
	got := r.GetRoutes()
	if len(got) != 0 {
		t.Fatalf("expected 0 routes, got %d", len(got))
	}
}

func TestRouter_AddRoute(t *testing.T) {
	r := &Router{}
	handler := func(w http.ResponseWriter, r *http.Request) {}
	r.addRoute("test", "GET", "/test", handler, true)
	routes := r.GetRoutes()
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	if routes[0].Name != "test" || routes[0].Method != "GET" || routes[0].Pattern != "/test" {
		t.Fatalf("unexpected route: %+v", routes[0])
	}
	if !routes[0].RateLimited {
		t.Fatal("expected route to be rate limited")
	}
}
