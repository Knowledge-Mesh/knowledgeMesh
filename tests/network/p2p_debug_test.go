package network_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/network"
	"github.com/libp2p/go-libp2p/core/host"
	peer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

func resetP2PDebug(t *testing.T) {
	t.Helper()
	network.SetP2PDebug(false)
	t.Cleanup(func() { network.SetP2PDebug(false) })
}

func TestParseP2PDebugEnv(t *testing.T) {
	cases := []struct {
		env    string
		wantOn bool
	}{
		{"", false},
		{"0", false},
		{"false", false},
		{"1", true},
		{"true", true},
		{"TRUE", true},
		{"yes", true},
		{"on", true},
	}
	for _, tc := range cases {
		t.Run(tc.env, func(t *testing.T) {
			t.Setenv(network.EnvP2PDebug, tc.env)
			if got := network.ParseP2PDebugEnv(); got != tc.wantOn {
				t.Fatalf("ParseP2PDebugEnv() = %v, want %v", got, tc.wantOn)
			}
		})
	}
}

func TestParseP2PDebugHTTPAddr(t *testing.T) {
	want := " 127.0.0.1:9091 "
	t.Setenv(network.EnvP2PDebugHTTP, want)
	if got := network.ParseP2PDebugHTTPAddr(); got != "127.0.0.1:9091" {
		t.Fatalf("got %q, want trimmed addr", got)
	}
}

func TestApplyP2PDebugForHost(t *testing.T) {
	resetP2PDebug(t)

	t.Run("env enables", func(t *testing.T) {
		t.Setenv(network.EnvP2PDebug, "1")
		network.ApplyP2PDebugForHost(network.HostConfig{})
		if !network.P2PDebug() {
			t.Fatal("expected P2P debug on from env")
		}
	})

	resetP2PDebug(t)
	t.Run("config overrides env off", func(t *testing.T) {
		t.Setenv(network.EnvP2PDebug, "1")
		off := false
		network.ApplyP2PDebugForHost(network.HostConfig{EnableP2PDebug: &off})
		if network.P2PDebug() {
			t.Fatal("expected P2P debug off from HostConfig")
		}
	})

	resetP2PDebug(t)
	t.Run("config overrides env on", func(t *testing.T) {
		t.Setenv(network.EnvP2PDebug, "")
		on := true
		network.ApplyP2PDebugForHost(network.HostConfig{EnableP2PDebug: &on})
		if !network.P2PDebug() {
			t.Fatal("expected P2P debug on from HostConfig")
		}
	})
}

func TestParsePeerIDFromArg(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h, err := network.NewHost(ctx, network.DefaultQUICListenAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	id := h.ID()

	if _, err := network.ParsePeerIDFromArg("not-a-peer-id"); err == nil {
		t.Fatal("expected error for invalid id")
	}
	got, err := network.ParsePeerIDFromArg("/" + id.String())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != id {
		t.Fatal("peer id mismatch")
	}
}

// connectTwoHostsForDebug dials h1 -> h2 and returns h1, h2's peer ID, tracker, cleanup.
func connectTwoHostsForDebug(t *testing.T, ctx context.Context) (host.Host, peer.ID, *network.ConnectionTypeTracker, func()) {
	t.Helper()

	h1, err := network.NewHost(ctx, network.DefaultQUICListenAddr)
	if err != nil {
		t.Fatalf("host1: %v", err)
	}
	h2, err := network.NewHost(ctx, network.DefaultQUICListenAddr)
	if err != nil {
		_ = h1.Close()
		t.Fatalf("host2: %v", err)
	}

	addr := h2.Addrs()[0].Encapsulate(ma.StringCast("/p2p/" + h2.ID().String()))
	info, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		_ = h2.Close()
		_ = h1.Close()
		t.Fatalf("addr info: %v", err)
	}
	if err := h1.Connect(ctx, *info); err != nil {
		_ = h2.Close()
		_ = h1.Close()
		t.Fatalf("connect: %v", err)
	}

	tracker := network.NewConnectionTypeTracker(h1)
	tracker.Start()

	cleanup := func() {
		tracker.Close()
		_ = h2.Close()
		_ = h1.Close()
	}
	return h1, h2.ID(), tracker, cleanup
}

func TestBuildPeerDebugReportDirect(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h1, peerB, tracker, cleanup := connectTwoHostsForDebug(t, ctx)
	defer cleanup()

	deadline := time.Now().Add(2 * time.Second)
	for !tracker.IsDirectConnection(peerB) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if !tracker.IsDirectConnection(peerB) {
		t.Fatal("expected direct connection for report")
	}
	tracker.HandleNetworkChange()

	rep := network.BuildPeerDebugReport(h1, tracker, nil, peerB)
	if rep.LocalPeerID != h1.ID().String() || rep.PeerID != peerB.String() {
		t.Fatalf("peer ids: local=%q peer=%q", rep.LocalPeerID, rep.PeerID)
	}
	if !rep.Connected {
		t.Fatal("expected connected")
	}
	if rep.ConnectionType != "direct" {
		t.Fatalf("connectionType = %q, want direct", rep.ConnectionType)
	}
	if rep.TaggedConnectionType != "direct" {
		t.Fatalf("taggedConnectionType = %q, want direct", rep.TaggedConnectionType)
	}
}

func TestNewP2PDebugHTTPMuxReachabilityAndPeer(t *testing.T) {
	t.Parallel()
	resetP2PDebug(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h1, peerB, tracker, cleanup := connectTwoHostsForDebug(t, ctx)
	defer cleanup()

	network.SetP2PDebug(true)

	deadline := time.Now().Add(2 * time.Second)
	for !tracker.IsDirectConnection(peerB) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}

	srv := httptest.NewServer(network.NewP2PDebugHTTPMux(h1, tracker, nil))
	defer srv.Close()

	t.Run("reachability", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/debug/p2p/reachability")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status %d: %s", resp.StatusCode, body)
		}
		var out map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		if _, ok := out["natReachability"]; !ok {
			t.Fatalf("missing natReachability: %v", out)
		}
	})

	t.Run("peer report", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/debug/p2p/peer/" + peerB.String())
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status %d: %s", resp.StatusCode, body)
		}
		var rep network.PeerDebugReport
		if err := json.NewDecoder(resp.Body).Decode(&rep); err != nil {
			t.Fatal(err)
		}
		if rep.PeerID != peerB.String() || !rep.Connected {
			t.Fatalf("unexpected report: %+v", rep)
		}
	})

	t.Run("invalid peer id", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/debug/p2p/peer/notvalid")
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("want 400, got %d", resp.StatusCode)
		}
	})

	t.Run("debug off returns 503", func(t *testing.T) {
		network.SetP2PDebug(false)
		resp, err := http.Get(srv.URL + "/debug/p2p/reachability")
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("want 503, got %d", resp.StatusCode)
		}
		network.SetP2PDebug(true)
	})
}
