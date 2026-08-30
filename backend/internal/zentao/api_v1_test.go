package zentao

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"
)

func TestAPICreateTaskPrefersExecutionRoute(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		if r.Header.Get("Token") != "tok" {
			t.Fatalf("unexpected token header: %q", r.Header.Get("Token"))
		}

		switch r.URL.Path {
		case "/api.php/v1/executions/396468/tasks":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method: %s", r.Method)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if body["name"] != "团队管理事务-20260615" {
				t.Fatalf("unexpected request body name: %#v", body["name"])
			}
			estStarted, _ := body["estStarted"].(string)
			if estStarted == "" {
				t.Fatalf("expected estStarted in request body, got %#v", body["estStarted"])
			}
			if _, err := time.Parse("2006-01-02", estStarted); err != nil {
				t.Fatalf("invalid estStarted format: %q", estStarted)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 101})
		case "/api.php/v1/tasks":
			t.Fatalf("generic /tasks should not be called when execution route succeeds")
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	cli := NewAPIClient(srv.URL)
	out, err := cli.APICreateTask(context.Background(), "tok", map[string]any{
		"name":      "团队管理事务-20260615",
		"execution": 396468,
	})
	if err != nil {
		t.Fatalf("APICreateTask returned error: %v", err)
	}
	if id := out["id"]; id != float64(101) {
		t.Fatalf("unexpected task id: %#v", id)
	}
	if !slices.Equal(calls, []string{"/api.php/v1/executions/396468/tasks"}) {
		t.Fatalf("unexpected call order: %#v", calls)
	}
}

func TestAPICreateTaskFallsBackToGenericRoute(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		switch r.URL.Path {
		case "/api.php/v1/executions/396468/tasks":
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		case "/api.php/v1/tasks":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 202})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	cli := NewAPIClient(srv.URL)
	out, err := cli.APICreateTask(context.Background(), "tok", map[string]any{
		"name":      "团队管理事务-20260616",
		"execution": 396468,
	})
	if err != nil {
		t.Fatalf("APICreateTask returned error: %v", err)
	}
	if id := out["id"]; id != float64(202) {
		t.Fatalf("unexpected task id: %#v", id)
	}
	if !slices.Equal(calls, []string{"/api.php/v1/executions/396468/tasks", "/api.php/v1/tasks"}) {
		t.Fatalf("unexpected call order: %#v", calls)
	}
}

func TestAPICreateTaskContinuesAfterTasksEntryArgumentError(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		switch r.URL.Path {
		case "/api.php/v1/projects/123/tasks":
			http.Error(w, "ArgumentCountError: Too few arguments to function tasksEntry::post()", http.StatusInternalServerError)
		case "/api.php/v1/tasks":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 303})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	cli := NewAPIClient(srv.URL)
	out, err := cli.APICreateTask(context.Background(), "tok", map[string]any{
		"name":    "团队管理事务-20260617",
		"project": 123,
	})
	if err != nil {
		t.Fatalf("APICreateTask returned error: %v", err)
	}
	if id := out["id"]; id != float64(303) {
		t.Fatalf("unexpected task id: %#v", id)
	}
	if !slices.Equal(calls, []string{"/api.php/v1/projects/123/tasks", "/api.php/v1/tasks"}) {
		t.Fatalf("unexpected call order: %#v", calls)
	}
}

func TestAPICreateTaskCopiesSnakeCaseEstStarted(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		switch r.URL.Path {
		case "/api.php/v1/tasks":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if body["estStarted"] != "2026-06-18" {
				t.Fatalf("expected estStarted to be copied from est_started, got %#v", body["estStarted"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 404})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	cli := NewAPIClient(srv.URL)
	out, err := cli.APICreateTask(context.Background(), "tok", map[string]any{
		"name":        "团队管理事务-20260618",
		"est_started": "2026-06-18",
	})
	if err != nil {
		t.Fatalf("APICreateTask returned error: %v", err)
	}
	if id := out["id"]; id != float64(404) {
		t.Fatalf("unexpected task id: %#v", id)
	}
	if !slices.Equal(calls, []string{"/api.php/v1/tasks"}) {
		t.Fatalf("unexpected call order: %#v", calls)
	}
}

func TestAPIUpdateTaskUsesPut(t *testing.T) {
	var method string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api.php/v1/tasks/364029" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		method = r.Method
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["estStarted"] != "2026-08-01" || body["deadline"] != "2026-08-05" {
			t.Fatalf("unexpected body: %#v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 364029, "estStarted": "2026-08-01", "deadline": "2026-08-05"})
	}))
	defer srv.Close()

	cli := NewAPIClient(srv.URL)
	_, err := cli.APIUpdateTask(context.Background(), "tok", 364029, map[string]any{
		"estStarted": "2026-08-01",
		"deadline":   "2026-08-05",
	})
	if err != nil {
		t.Fatalf("APIUpdateTask returned error: %v", err)
	}
	if method != http.MethodPut {
		t.Fatalf("expected PUT, got %s", method)
	}
}

func TestParsePlanIDs(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want []int64
	}{
		{name: "int", in: 12, want: []int64{12}},
		{name: "float", in: float64(34), want: []int64{34}},
		{name: "csv", in: "1, 2,3", want: []int64{1, 2, 3}},
		{name: "object", in: map[string]any{"id": float64(9)}, want: []int64{9}},
		{name: "list", in: []any{float64(4), "5"}, want: []int64{4, 5}},
		{name: "empty", in: "0", want: nil},
	}
	for _, tc := range cases {
		got := ParsePlanIDs(tc.in)
		if !slices.Equal(got, tc.want) {
			t.Fatalf("%s: got %#v want %#v", tc.name, got, tc.want)
		}
	}
}

func TestExtractStoryAndBugPlan(t *testing.T) {
	storyID, ok := ExtractStoryPlanID(map[string]any{
		"story": map[string]any{"plan": "8,9"},
	})
	if !ok || storyID != 8 {
		t.Fatalf("ExtractStoryPlanID: got %d ok=%v", storyID, ok)
	}
	if !ContainsPlanID(ExtractStoryPlanIDs(map[string]any{"plan": "8,9"}), 9) {
		t.Fatal("expected story plan 9 to be present")
	}
	bugID, ok := ExtractBugPlanID(map[string]any{
		"bug": map[string]any{"plan": float64(21)},
	})
	if !ok || bugID != 21 {
		t.Fatalf("ExtractBugPlanID: got %d ok=%v", bugID, ok)
	}
}
