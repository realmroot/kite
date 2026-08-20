package cluster

import (
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/zxh326/kite/pkg/model"
	"k8s.io/client-go/rest"
)

type closeTrackingTransport struct {
	closed atomic.Bool
}

func (transport *closeTrackingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	panic("not used")
}

func (transport *closeTrackingTransport) CloseIdleConnections() {
	transport.closed.Store(true)
}

func TestClusterRuntimeIsSharedAndRebuiltWhenConnectionChanges(t *testing.T) {
	var creations atomic.Int32
	var createdMu sync.Mutex
	var created []*closeTrackingTransport
	manager := &ClusterManager{
		runtimes: make(map[uint]*clusterRuntime),
		transportFor: func(*rest.Config) (http.RoundTripper, error) {
			transport := &closeTrackingTransport{}
			creations.Add(1)
			createdMu.Lock()
			created = append(created, transport)
			createdMu.Unlock()
			return transport, nil
		},
	}
	clusterRecord := &model.Cluster{Model: model.Model{ID: 42}, Name: "prod", APIServerURL: "https://api.example.test"}

	const callers = 32
	var waitGroup sync.WaitGroup
	waitGroup.Add(callers)
	runtimes := make(chan *clusterRuntime, callers)
	for range callers {
		go func() {
			defer waitGroup.Done()
			runtime, err := manager.runtimeForCluster(clusterRecord)
			if err != nil {
				t.Errorf("runtimeForCluster() error = %v", err)
				return
			}
			runtimes <- runtime
		}()
	}
	waitGroup.Wait()
	close(runtimes)

	var first *clusterRuntime
	for runtime := range runtimes {
		if first == nil {
			first = runtime
			continue
		}
		if runtime != first {
			t.Fatal("concurrent callers received different cluster runtimes")
		}
	}
	if got := creations.Load(); got != 1 {
		t.Fatalf("transport creations = %d, want 1", got)
	}

	clusterRecord.TLSServerName = "changed.example.test"
	if _, err := manager.runtimeForCluster(clusterRecord); err != nil {
		t.Fatalf("rebuilding runtime: %v", err)
	}
	if got := creations.Load(); got != 2 {
		t.Fatalf("transport creations after metadata change = %d, want 2", got)
	}
	createdMu.Lock()
	firstTransport := created[0]
	createdMu.Unlock()
	if !firstTransport.closed.Load() {
		t.Fatal("replaced transport did not close idle connections")
	}
}

func TestInvalidateRuntimeClosesAndRemovesTransport(t *testing.T) {
	transport := &closeTrackingTransport{}
	manager := &ClusterManager{runtimes: map[uint]*clusterRuntime{
		7: {transport: transport},
	}}

	manager.invalidateRuntime(7)

	if !transport.closed.Load() {
		t.Fatal("invalidated transport did not close idle connections")
	}
	if manager.runtimes[7] != nil {
		t.Fatal("invalidated runtime remains cached")
	}
}
