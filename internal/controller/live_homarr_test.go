//go:build homarr_live

// Read-only validation of the controller's board.saveBoard payload against a
// running homarr. Run it by hand against a real instance:
//
//	HOMARR_URL=http://127.0.0.1:7575 HOMARR_API_KEY=... HOMARR_BOARD=annarchy \
//	KUBE_CONTEXT=admin@annarchy HOMARR_NAMESPACES=htpc,monitoring,... \
//	go test -tags homarr_live -run TestLiveHomarr -v ./internal/controller/
//
// It runs a full reconcile with the real Kubernetes sources (list only) and
// the real homarr API for reads. Every homarr mutation is refused, except
// board.saveBoard, which is sent with a board id that does not exist:
// homarr validates the input schema first and only then looks the board up,
// so NOT_FOUND proves the payload passed the schema while nothing is written.
// The payload is also checked against a port of homarr's layout bounds check.

package controller_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/controller"
	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/homarr"
	"github.com/adamancini/homarr-kubernetes-dashboard-controller/internal/source"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const nonexistentBoardID = "readonly-validation-nonexistent-board"

var errRefused = errors.New("refused: read-only validation run")

// readOnlyHomarr passes reads through and refuses every write.
type readOnlyHomarr struct {
	*homarr.Client
	t        *testing.T
	refused  []string
	payload  *homarr.BoardSave
	saveErr  error
	saveSent bool
}

func (c *readOnlyHomarr) refuse(op string) error {
	c.refused = append(c.refused, op)
	c.t.Logf("refused write: %s", op)
	return errRefused
}

func (c *readOnlyHomarr) CreateApp(_ context.Context, a homarr.AppCreate) (homarr.App, error) {
	return homarr.App{}, c.refuse("app.create " + a.Name)
}
func (c *readOnlyHomarr) UpdateApp(_ context.Context, id string, _ homarr.AppUpdate) error {
	return c.refuse("app.edit " + id)
}
func (c *readOnlyHomarr) DeleteApp(_ context.Context, id string) error {
	return c.refuse("app.delete " + id)
}
func (c *readOnlyHomarr) CreateBoard(_ context.Context, b homarr.BoardCreate) (homarr.Board, error) {
	return homarr.Board{}, c.refuse("board.createBoard " + b.Name)
}
func (c *readOnlyHomarr) CreateIntegration(_ context.Context, i homarr.IntegrationCreate) (homarr.Integration, error) {
	return homarr.Integration{}, c.refuse("integration.create " + i.Kind + " " + i.URL)
}
func (c *readOnlyHomarr) DeleteIntegration(_ context.Context, id string) error {
	return c.refuse("integration.delete " + id)
}
func (c *readOnlyHomarr) SaveBoard(ctx context.Context, b homarr.BoardSave) error {
	payload := b
	c.payload = &payload
	probe := b
	probe.ID = nonexistentBoardID
	c.saveSent = true
	c.saveErr = c.Client.SaveBoard(ctx, probe)
	return errRefused // never report success to the reconciler
}

func liveEnv(t *testing.T, key string) string {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		t.Fatalf("%s must be set for the live validation run", key)
	}
	return v
}

func TestLiveHomarr_OldCategoryPayloadIsRejected(t *testing.T) {
	c := homarr.NewClient(liveEnv(t, "HOMARR_URL"), liveEnv(t, "HOMARR_API_KEY"))
	// The pre-fix payload: a "category" section without layouts.
	err := c.SaveBoard(context.Background(), homarr.BoardSave{
		ID: nonexistentBoardID,
		Sections: []homarr.Section{
			{ID: "root", Kind: homarr.SectionKindEmpty},
			{ID: "cat-services", Kind: "category", Name: "Services", YOffset: 1},
		},
	})
	if err == nil || homarr.IsNotFound(err) || !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "invalid_union") {
		t.Fatalf("expected 400 invalid_union for the old section shape, got: %v", err)
	}
	t.Logf("old payload rejected as expected: %.300s", err.Error())
}

func TestLiveHomarr_ReconcilePayloadPassesSchema(t *testing.T) {
	ctx := context.Background()
	hc := homarr.NewClient(liveEnv(t, "HOMARR_URL"), liveEnv(t, "HOMARR_API_KEY"))
	boardName := liveEnv(t, "HOMARR_BOARD")
	namespaces := strings.Split(liveEnv(t, "HOMARR_NAMESPACES"), ",")

	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{CurrentContext: liveEnv(t, "KUBE_CONTEXT")},
	).ClientConfig()
	if err != nil {
		t.Fatalf("kube config: %v", err)
	}
	kc, err := client.New(cfg, client.Options{})
	if err != nil {
		t.Fatalf("kube client: %v", err)
	}
	sources := []controller.SourceLister{
		source.NewIngressSource(kc, "homarr.dev", namespaces),
		source.NewIngressRouteSource(kc, "homarr.dev", namespaces),
		source.NewHTTPRouteSource(kc, "homarr.dev", namespaces),
		source.NewServiceSource(kc, "homarr.dev", namespaces),
	}

	board, err := hc.GetBoardByName(ctx, boardName)
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	ro := &readOnlyHomarr{Client: hc, t: t}
	logs := captureLogs(t)
	r := controller.NewReconciler(ro, sources, boardName, 12, "https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/svg")
	if _, err := r.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	t.Logf("controller log:\n%s", logs.String())

	if !ro.saveSent || ro.payload == nil {
		t.Fatal("reconcile did not produce a saveBoard payload")
	}
	if !homarr.IsNotFound(ro.saveErr) {
		t.Fatalf("homarr rejected the payload (want NOT_FOUND for the probe board id, which means the schema passed): %v", ro.saveErr)
	}
	t.Logf("homarr accepted the payload schema (probe answered NOT_FOUND, nothing written)")

	assertValidV2Save(t, board.Layouts, ro.payload)

	// Same board, payload by payload: no section is added for categories that
	// already exist as containers; every stored section is kept.
	stored := map[string]bool{}
	for _, s := range board.Sections {
		stored[s.ID] = true
	}
	for _, s := range ro.payload.Sections {
		if !stored[s.ID] {
			t.Errorf("payload adds section %s (%q) that is not on the board", s.ID, s.Title())
		}
	}
	if len(ro.payload.Sections) != len(board.Sections) {
		t.Errorf("payload sections = %d, board sections = %d", len(ro.payload.Sections), len(board.Sections))
	}
	nonManaged := 0
	for _, it := range board.Items {
		if !strings.HasPrefix(it.ID, "managed-") {
			nonManaged++
			found := false
			for _, p := range ro.payload.Items {
				found = found || p.ID == it.ID
			}
			if !found {
				t.Errorf("non-managed item %s dropped from payload", it.ID)
			}
		}
	}
	t.Logf("board sections=%d payload sections=%d board items=%d payload items=%d nonManaged=%d refused writes=%v",
		len(board.Sections), len(ro.payload.Sections), len(board.Items), len(ro.payload.Items), nonManaged, ro.refused)

	if out := os.Getenv("HOMARR_PAYLOAD_OUT"); out != "" {
		b, _ := json.MarshalIndent(ro.payload, "", "  ")
		if err := os.WriteFile(out, b, 0o600); err != nil {
			t.Fatalf("write payload: %v", err)
		}
	}
}
