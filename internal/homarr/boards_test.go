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

func TestSaveBoard_RoundTripsV2Sections(t *testing.T) {
	// A board read from homarr v2.12.0 must be saved back in a shape its
	// boardSaveSchema accepts: empty sections with offsets, container
	// sections with options/collapsed/layouts. Pre-v2 "category" sections
	// no longer exist.
	var savedBody json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/trpc/board.getBoardByName":
			w.Write([]byte(`{"result":{"data":{"json":{
				"id":"board-1","name":"homelab",
				"layouts":[
					{"id":"mobile","name":"Mobile","columnCount":3,"breakpoint":0,"leftGutterColumnCount":0,"rightGutterColumnCount":0,"role":"mobile"},
					{"id":"base","name":"Base","columnCount":12,"breakpoint":768,"leftGutterColumnCount":0,"rightGutterColumnCount":0,"role":"base"}],
				"sections":[
					{"id":"root","kind":"empty","xOffset":0,"yOffset":0,"name":null,"options":{},"layouts":[],"collapsed":false},
					{"id":"c-infra","kind":"container","xOffset":null,"yOffset":null,"name":null,
					 "options":{"title":"Infra","customCssClasses":[],"borderColor":"","showLabel":true,"collapsible":true,"showOpenAll":true,"scrollable":false},
					 "layouts":[{"xOffset":0,"yOffset":0,"width":12,"height":1,"parentSectionId":"root","layoutId":"base"},
					            {"xOffset":0,"yOffset":0,"width":3,"height":1,"parentSectionId":"root","layoutId":"mobile"}],
					 "collapsed":false}],
				"items":[]}}}}`))
		case "/api/trpc/board.saveBoard":
			var reqBody map[string]json.RawMessage
			json.NewDecoder(r.Body).Decode(&reqBody)
			savedBody = reqBody["json"]
			w.Write([]byte(`{"result":{"data":{"json":null}}}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := homarr.NewClient(srv.URL, "test-key")
	board, err := c.GetBoardByName(context.Background(), "homelab")
	if err != nil {
		t.Fatalf("GetBoardByName: %v", err)
	}
	if len(board.Layouts) != 2 || board.Layouts[0].Role != "mobile" || board.Layouts[1].ColumnCount != 12 {
		t.Fatalf("layouts not decoded: %+v", board.Layouts)
	}

	if err := c.SaveBoard(context.Background(), homarr.BoardSave{ID: board.ID, Sections: board.Sections, Items: board.Items}); err != nil {
		t.Fatalf("SaveBoard: %v", err)
	}

	var saved struct {
		ID       string                       `json:"id"`
		Sections []map[string]json.RawMessage `json:"sections"`
		Items    json.RawMessage              `json:"items"`
	}
	if err := json.Unmarshal(savedBody, &saved); err != nil {
		t.Fatalf("unmarshal saved body: %v", err)
	}
	if saved.ID != "board-1" {
		t.Errorf("id = %q", saved.ID)
	}
	if string(saved.Items) != "[]" {
		t.Errorf("items = %s, want [] (homarr requires an array)", saved.Items)
	}
	if len(saved.Sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(saved.Sections))
	}
	for _, s := range saved.Sections {
		kind := string(s["kind"])
		if kind != `"empty"` && kind != `"container"` {
			t.Errorf("section kind %s is not accepted by homarr v2.12.0", kind)
		}
	}
	root, container := saved.Sections[0], saved.Sections[1]
	if string(root["xOffset"]) != "0" || string(root["yOffset"]) != "0" {
		t.Errorf("empty section offsets = %s/%s", root["xOffset"], root["yOffset"])
	}
	var layouts []map[string]any
	if err := json.Unmarshal(container["layouts"], &layouts); err != nil || len(layouts) != 2 {
		t.Fatalf("container layouts = %s (err %v)", container["layouts"], err)
	}
	if layouts[0]["parentSectionId"] != "root" {
		t.Errorf("container layout parentSectionId = %v, want root", layouts[0]["parentSectionId"])
	}
	var opts map[string]any
	if err := json.Unmarshal(container["options"], &opts); err != nil || opts["title"] != "Infra" {
		t.Errorf("container options = %s", container["options"])
	}
}
