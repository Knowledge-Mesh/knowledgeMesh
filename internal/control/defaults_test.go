package control

import "testing"

func TestResolveControlURL(t *testing.T) {
	u, def := ResolveControlURL("")
	if u != DefaultControlURL || !def {
		t.Fatalf("empty: got %q def=%v", u, def)
	}
	u, def = ResolveControlURL("  http://example.com:9000/  ")
	if u != "http://example.com:9000/" || def {
		t.Fatalf("explicit: got %q def=%v", u, def)
	}
}
