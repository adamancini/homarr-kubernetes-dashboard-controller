package homarr_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/homarr"
)

// integrationServer answers integration.create with the given tRPC result
// JSON and records the request input.
func integrationServer(t *testing.T, result string, gotInput *map[string]json.RawMessage) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/trpc/integration.create" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var body struct {
			JSON map[string]json.RawMessage `json:"json"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if gotInput != nil {
			*gotInput = body.JSON
		}
		w.Write([]byte(`{"result":{"data":{"json":` + result + `}}}`))
	}))
}

func TestCreateIntegration(t *testing.T) {
	// homarr v2.12.0 integration.create returns {integration: {...}, appId}.
	var input map[string]json.RawMessage
	srv := integrationServer(t,
		`{"integration":{"id":"intg-1","name":"Sonarr","kind":"sonarr","url":"http://sonarr:8989"},"appId":null}`,
		&input)
	defer srv.Close()

	c := homarr.NewClient(srv.URL, "test-key")
	intg, err := c.CreateIntegration(context.Background(), homarr.IntegrationCreate{
		Name: "Sonarr",
		URL:  "http://sonarr:8989",
		Kind: "sonarr",
		Secrets: []homarr.IntegrationSecret{
			{Kind: "apiKey", Value: "secret123"},
		},
	})
	if err != nil {
		t.Fatalf("CreateIntegration: %v", err)
	}
	if intg.ID != "intg-1" {
		t.Errorf("unexpected ID: %q", intg.ID)
	}
	if intg.Kind != "sonarr" || intg.URL != "http://sonarr:8989" {
		t.Errorf("unexpected integration: %+v", intg)
	}

	// Required fields of integrationCreateSchema must be on the wire.
	for _, k := range []string{"name", "url", "kind", "secrets", "attemptSearchEngineCreation"} {
		if _, ok := input[k]; !ok {
			t.Errorf("request is missing required field %q", k)
		}
	}
	if string(input["attemptSearchEngineCreation"]) != "false" {
		t.Errorf("attemptSearchEngineCreation = %s, want false", input["attemptSearchEngineCreation"])
	}
}

func TestCreateIntegration_SecretsNeverNull(t *testing.T) {
	// zod rejects secrets: null; a nil slice must go out as [].
	var input map[string]json.RawMessage
	srv := integrationServer(t, `{"integration":{"id":"i","name":"x","kind":"plex","url":"http://x"},"appId":null}`, &input)
	defer srv.Close()

	c := homarr.NewClient(srv.URL, "test-key")
	if _, err := c.CreateIntegration(context.Background(), homarr.IntegrationCreate{Name: "x", URL: "http://x", Kind: "plex"}); err != nil {
		t.Fatalf("CreateIntegration: %v", err)
	}
	if string(input["secrets"]) != "[]" {
		t.Errorf("secrets = %s, want []", input["secrets"])
	}
}

func TestCreateIntegration_ConnectionTestFailureIsAnError(t *testing.T) {
	// When homarr's connection test fails it answers HTTP 200 with
	// {error: {...}} and persists nothing. That must surface as an error
	// carrying homarr's response, not as a success with an empty id.
	srv := integrationServer(t,
		`{"error":{"type":"request","data":{"type":"timeout","reason":"aborted"},"cause":{"name":"Error","message":"Connect Timeout Error","metadata":[]}}}`,
		nil)
	defer srv.Close()

	c := homarr.NewClient(srv.URL, "test-key")
	intg, err := c.CreateIntegration(context.Background(), homarr.IntegrationCreate{
		Name: "Plex", URL: "http://plex", Kind: "plex",
		Secrets: []homarr.IntegrationSecret{{Kind: "apiKey", Value: "t"}},
	})
	if err == nil {
		t.Fatalf("expected error, got integration %+v", intg)
	}
	if intg.ID != "" {
		t.Errorf("failed create returned id %q", intg.ID)
	}
	msg := err.Error()
	for _, want := range []string{"plex", "Connect Timeout Error", "timeout"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not include %q from homarr's response", msg, want)
		}
	}
}

func TestCreateIntegration_EmptyIDIsAnError(t *testing.T) {
	for name, result := range map[string]string{
		"legacy flat shape": `{"id":"","name":"x","kind":"plex","url":"http://x"}`,
		"null integration":  `{"integration":null,"appId":null}`,
		"empty id":          `{"integration":{"id":"","name":"x","kind":"plex","url":"http://x"},"appId":null}`,
		"null result":       `null`,
	} {
		t.Run(name, func(t *testing.T) {
			srv := integrationServer(t, result, nil)
			defer srv.Close()

			c := homarr.NewClient(srv.URL, "test-key")
			intg, err := c.CreateIntegration(context.Background(), homarr.IntegrationCreate{Name: "x", URL: "http://x", Kind: "plex"})
			if err == nil {
				t.Fatalf("expected error for response %s, got %+v", result, intg)
			}
		})
	}
}

func TestListIntegrations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"result": map[string]any{
				"data": map[string]any{
					"json": []any{
						map[string]any{"id": "intg-1", "name": "Sonarr", "url": "http://sonarr:8989", "kind": "sonarr"},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := homarr.NewClient(srv.URL, "test-key")
	intgs, err := c.ListIntegrations(context.Background())
	if err != nil {
		t.Fatalf("ListIntegrations: %v", err)
	}
	if len(intgs) != 1 {
		t.Errorf("expected 1 integration, got %d", len(intgs))
	}
}
