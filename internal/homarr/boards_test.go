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
