package routinghelpers

import (
	"context"
	"testing"

	errwrap "github.com/hashicorp/errwrap"
	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-peer"
	routing "github.com/libp2p/go-libp2p-routing"
	mh "github.com/multiformats/go-multihash"
)

func TestParallelPutGet(t *testing.T) {
	d := Parallel{
		&Compose{
			ValueStore: &LimitedValueStore{
				ValueStore: new(dummyValueStore),
				Namespaces: []string{"allow1", "allow2", "notsupported"},
			},
		},
		&Compose{
			ValueStore: &LimitedValueStore{
				ValueStore: new(dummyValueStore),
				Namespaces: []string{"allow1", "allow2", "notsupported", "error"},
			},
		},
		&Compose{
			ValueStore: &LimitedValueStore{
				ValueStore: new(dummyValueStore),
				Namespaces: []string{"allow1", "error", "solo"},
			},
		},
	}

	ctx := context.Background()

	if err := d.PutValue(ctx, "/allow1/hello", []byte("world")); err != nil {
		t.Fatal(err)
	}
	for _, di := range append([]routing.IpfsRouting{d}, d...) {
		v, err := di.GetValue(ctx, "/allow1/hello")
		if err != nil {
			t.Fatal(err)
		}
		if string(v) != "world" {
			t.Fatal("got the wrong value")
		}
	}

	if err := d.PutValue(ctx, "/allow2/hello", []byte("world2")); err != nil {
		t.Fatal(err)
	}
	for _, di := range append([]routing.IpfsRouting{d}, d[:1]...) {
		v, err := di.GetValue(ctx, "/allow2/hello")
		if err != nil {
			t.Fatal(err)
		}
		if string(v) != "world2" {
			t.Fatal("got the wrong value")
		}
	}
	if err := d.PutValue(ctx, "/forbidden/hello", []byte("world")); err != routing.ErrNotSupported {
		t.Fatalf("expected ErrNotSupported, got: %s", err)
	}
	for _, di := range append([]routing.IpfsRouting{d}, d...) {
		_, err := di.GetValue(ctx, "/forbidden/hello")
		if err != routing.ErrNotFound {
			t.Fatalf("expected ErrNotFound, got: %s", err)
		}
	}
	// Bypass the LimitedValueStore.
	if err := d.PutValue(ctx, "/notsupported/hello", []byte("world")); err != routing.ErrNotSupported {
		t.Fatalf("expected ErrNotSupported, got: %s", err)
	}
	if err := d.PutValue(ctx, "/error/myErr", []byte("world")); !errwrap.Contains(err, "myErr") {
		t.Fatalf("expected error to contain myErr, got: %s", err)
	}
	if err := d.PutValue(ctx, "/solo/thing", []byte("value")); err != nil {
		t.Fatal(err)
	}
	v, err := d.GetValue(ctx, "/solo/thing")
	if err != nil {
		t.Fatal(err)
	}
	if string(v) != "value" {
		t.Fatalf("expected 'value', got '%s'", string(v))
	}
}

func TestBasicParallelFindProviders(t *testing.T) {
	prefix := cid.NewPrefixV1(cid.Raw, mh.SHA2_256)
	c, _ := prefix.Sum([]byte("foo"))

	ctx := context.Background()

	d := Parallel{}
	if _, ok := <-d.FindProvidersAsync(ctx, c, 10); ok {
		t.Fatal("expected no results")
	}
	d = Parallel{
		&Compose{
			ContentRouting: &dummyProvider{},
		},
	}
	if _, ok := <-d.FindProvidersAsync(ctx, c, 10); ok {
		t.Fatal("expected no results")
	}
}

func TestParallelFindProviders(t *testing.T) {
	prefix := cid.NewPrefixV1(cid.Raw, mh.SHA2_256)

	cid1, _ := prefix.Sum([]byte("foo"))
	cid2, _ := prefix.Sum([]byte("bar"))
	cid3, _ := prefix.Sum([]byte("baz"))
	cid4, _ := prefix.Sum([]byte("none"))

	d := Parallel{
		&Compose{},
		&Compose{
			ContentRouting: &dummyProvider{
				cids: map[string][]peer.ID{
					cid1.KeyString(): []peer.ID{
						"first",
						"second",
						"third",
						"fourth",
						"fifth",
						"sixth",
					},
					cid2.KeyString(): []peer.ID{
						"fourth",
						"fifth",
						"sixth",
					},
				},
			},
		},
		&Compose{
			ContentRouting: &dummyProvider{
				cids: map[string][]peer.ID{
					cid1.KeyString(): []peer.ID{
						"first",
						"second",
						"fifth",
						"sixth",
					},
					cid2.KeyString(): []peer.ID{
						"second",
						"fourth",
						"fifth",
					},
				},
			},
		},
		&Compose{
			ValueStore: &LimitedValueStore{
				ValueStore: new(dummyValueStore),
				Namespaces: []string{"allow1"},
			},
			ContentRouting: &dummyProvider{
				cids: map[string][]peer.ID{
					cid2.KeyString(): []peer.ID{
						"first",
					},
					cid3.KeyString(): []peer.ID{
						"second",
						"fourth",
						"fifth",
						"sixth",
					},
				},
			},
		},
	}

	ctx := context.Background()

	for i := 0; i < 2; i++ {

		for i, tc := range []struct {
			cid       *cid.Cid
			providers []peer.ID
		}{
			{
				cid:       cid1,
				providers: []peer.ID{"first", "second", "third", "fourth", "fifth", "sixth"},
			},
			{
				cid:       cid2,
				providers: []peer.ID{"first", "second", "fourth", "fifth", "sixth"},
			},
			{
				cid:       cid3,
				providers: []peer.ID{"second", "fourth", "fifth", "sixth"},
			},
		} {
			expecting := make(map[peer.ID]struct{}, len(tc.providers))
			for _, p := range tc.providers {
				expecting[p] = struct{}{}
			}
			for p := range d.FindProvidersAsync(ctx, tc.cid, 10) {
				if _, ok := expecting[p.ID]; !ok {
					t.Errorf("not expecting provider %s for test case %d", string(p.ID), i)
				}
				delete(expecting, p.ID)
			}
			for p := range expecting {
				t.Errorf("failed to find expected provider %s for test case %d", string(p), i)
			}
		}
		expecting := []peer.ID{"second", "fourth", "fifth"}
		for p := range d.FindProvidersAsync(ctx, cid3, 3) {
			if len(expecting) == 0 {
				t.Errorf("not expecting any more providers, got %s", string(p.ID))
				continue
			}
			if expecting[0] != p.ID {
				t.Errorf("expecting peer %s, got peer %s", string(expecting[0]), string(p.ID))
			}
			expecting = expecting[1:]
		}
		for _, e := range expecting {
			t.Errorf("didn't find expected peer: %s", string(e))
		}
		if _, ok := <-d.FindProvidersAsync(ctx, cid4, 3); ok {
			t.Fatalf("shouldn't have found this CID")
		}
		if _, ok := <-d.FindProvidersAsync(ctx, cid1, 0); ok {
			t.Fatalf("should have found no CIDs")
		}

		// Now to test many content routers
		for i := 0; i < 30; i++ {
			d = append(d, &Compose{
				ContentRouting: &dummyProvider{},
			})
		}
	}
}