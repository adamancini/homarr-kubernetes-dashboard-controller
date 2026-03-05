package homarr_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/homarr"
)

func TestCreateBoard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/trpc/board.createBoard" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var body struct {
			JSON homarr.BoardCreate `json:"json"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.JSON.Name != "homelab" {
			t.Errorf("unexpected name: %s", body.JSON.Name)
		}
		resp := map[string]any{
			"result": map[string]any{
				"data": map[string]any{
					"json": map[string]any{"id": "board-1", "name": "homelab", "isPublic": false},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := homarr.NewClient(srv.URL, "test-key")
	board, err := c.CreateBoard(context.Background(), homarr.BoardCreate{
		Name:        "homelab",
		ColumnCount: 12,
		IsPublic:    false,
	})
	if err != nil {
		t.Fatalf("CreateBoard: %v", err)
	}
	if board.ID != "board-1" {
		t.Errorf("unexpected ID: %s", board.ID)
	}
}

func TestGetBoardByName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/trpc/board.getBoardByName" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := map[string]any{
			"result": map[string]any{
				"data": map[string]any{
					"json": map[string]any{
						"id": "board-1", "name": "homelab",
						"sections": []any{},
						"items":    []any{},
						"layouts":  []any{},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := homarr.NewClient(srv.URL, "test-key")
	board, err := c.GetBoardByName(context.Background(), "homelab")
	if err != nil {
		t.Fatalf("GetBoardByName: %v", err)
	}
	if board.ID != "board-1" {
		t.Errorf("unexpected ID: %s", board.ID)
	}
}

func TestSaveBoard_PreservesSectionFields(t *testing.T) {
	// Homarr returns sections with collapsed, options, and layouts fields.
	// SaveBoard must round-trip these fields so the tRPC schema validates.
	var savedBody json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/trpc/board.getBoardByName":
			resp := map[string]any{
				"result": map[string]any{
					"data": map[string]any{
						"json": map[string]any{
							"id": "board-1", "name": "homelab",
							"sections": []any{
								map[string]any{
									"id": "s1", "kind": "category", "name": "Apps",
									"xOffset": 0, "yOffset": 0,
									"collapsed": false,
								},
								map[string]any{
									"id": "s2", "kind": "empty",
									"xOffset": 0, "yOffset": 1,
								},
								map[string]any{
									"id": "s3", "kind": "dynamic", "name": "Dynamic",
									"xOffset": 0, "yOffset": 2,
									"options": map[string]any{"css": ""},
									"layouts":  []any{map[string]any{"id": "l1", "columnCount": 12, "breakpoint": 0}},
								},
							},
							"items":   []any{},
							"layouts": []any{map[string]any{"id": "l1", "name": "default", "columnCount": 12, "breakpoint": 0}},
						},
					},
				},
			}
			json.NewEncoder(w).Encode(resp)
		case "/api/trpc/board.saveBoard":
			body, _ := json.Marshal(map[string]any{}) // read the request body
			_ = body
			raw, _ := json.MarshalIndent(map[string]any{}, "", "  ")
			_ = raw
			// Capture the actual request body
			var reqBody map[string]json.RawMessage
			json.NewDecoder(r.Body).Decode(&reqBody)
			savedBody = reqBody["json"]
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"result":{"data":{"json":null}}}`))
		}
	}))
	defer srv.Close()

	c := homarr.NewClient(srv.URL, "test-key")

	board, err := c.GetBoardByName(context.Background(), "homelab")
	if err != nil {
		t.Fatalf("GetBoardByName: %v", err)
	}

	err = c.SaveBoard(context.Background(), homarr.BoardSave{
		ID:       board.ID,
		Sections: board.Sections,
		Items:    board.Items,
	})
	if err != nil {
		t.Fatalf("SaveBoard: %v", err)
	}

	// Parse the saved body and verify section fields are preserved
	var saved struct {
		Sections []map[string]json.RawMessage `json:"sections"`
	}
	if err := json.Unmarshal(savedBody, &saved); err != nil {
		t.Fatalf("unmarshal saved body: %v", err)
	}

	if len(saved.Sections) != 3 {
		t.Fatalf("expected 3 sections, got %d", len(saved.Sections))
	}

	// Category section must have "collapsed"
	if _, ok := saved.Sections[0]["collapsed"]; !ok {
		t.Error("category section missing 'collapsed' field")
	}

	// Dynamic section must have "options" and "layouts"
	if _, ok := saved.Sections[2]["options"]; !ok {
		t.Error("dynamic section missing 'options' field")
	}
	if _, ok := saved.Sections[2]["layouts"]; !ok {
		t.Error("dynamic section missing 'layouts' field")
	}
}
