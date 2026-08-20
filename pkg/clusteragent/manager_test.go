package clusteragent

import "testing"

func TestDisconnectAllClearsRegisteredTunnelState(t *testing.T) {
	manager := NewManager(func() {})
	manager.registrations["7"] = registeredCluster{}
	manager.registrations["9"] = registeredCluster{}

	manager.DisconnectAll()

	if len(manager.registrations) != 0 {
		t.Fatalf("registrations = %#v, want none", manager.registrations)
	}
}
