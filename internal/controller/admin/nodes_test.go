package admin

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
)

func TestListNodesClusterFlagAndCurrent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminSvc := service.NewTestAdminService(repository.NewMemoryAdminStore())
	if _, err := adminSvc.Create(t.Context(), "root", "password123"); err != nil {
		t.Fatal(err)
	}
	login, err := adminSvc.CompleteLogin(t.Context(), "root", "password123", "")
	if err != nil {
		t.Fatal(err)
	}
	store := repository.NewMemoryProjectStore()
	backend, err := storage.NewLocalFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projSvc := service.NewProjectService(store, backend)
	nodes := repository.NewMemoryNodeStore()
	projSvc.SetNodeStore(nodes)
	selfID := uuid.New()
	otherID := uuid.New()
	now := time.Now().UTC()
	if err := nodes.Create(t.Context(), &model.Node{ID: selfID, DisplayName: "self", LastSeenAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := nodes.Create(t.Context(), &model.Node{ID: otherID, DisplayName: "peer", LastSeenAt: now}); err != nil {
		t.Fatal(err)
	}
	projSvc.SetNodeID(selfID)
	projSvc.SetCluster(false, "")

	r := gin.New()
	Register(r.Group("/api/v1/admin"), adminSvc, projSvc, nil, nil, nil, nil, nil)

	w := doJSON(r, http.MethodGet, "/api/v1/admin/nodes", login.Token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("singleton list=%d %s", w.Code, w.Body.String())
	}
	got := decodeNodeList(t, w.Body.Bytes())
	if got.ClusterActive {
		t.Fatal("singleton cluster_active must be false")
	}
	assertCurrentNode(t, got.Nodes, selfID)

	projSvc.SetCluster(true, "")
	w = doJSON(r, http.MethodGet, "/api/v1/admin/nodes", login.Token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("cluster list=%d %s", w.Code, w.Body.String())
	}
	got = decodeNodeList(t, w.Body.Bytes())
	if !got.ClusterActive {
		t.Fatal("cluster cluster_active must be true")
	}
	assertCurrentNode(t, got.Nodes, selfID)
}

type nodeListBody struct {
	ClusterActive bool `json:"cluster_active"`
	Nodes         []struct {
		ID        string `json:"id"`
		IsCurrent bool   `json:"is_current"`
	} `json:"nodes"`
}

func decodeNodeList(t *testing.T, raw []byte) nodeListBody {
	t.Helper()
	var got nodeListBody
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode nodes: %v %s", err, raw)
	}
	return got
}

func assertCurrentNode(t *testing.T, nodes []struct {
	ID        string `json:"id"`
	IsCurrent bool   `json:"is_current"`
}, want uuid.UUID) {
	t.Helper()
	var current int
	for _, n := range nodes {
		if n.IsCurrent {
			current++
			if n.ID != want.String() {
				t.Fatalf("is_current id=%s want %s", n.ID, want)
			}
		}
	}
	if current != 1 {
		t.Fatalf("is_current count=%d want 1", current)
	}
}
