package routinghelpers

import (
	"context"
	"testing"

	peert "github.com/dms3-p2p/go-p2p-peer/test"
	routing "github.com/dms3-p2p/go-p2p-routing"
)

func TestGetPublicKey(t *testing.T) {
	d := Parallel{
		Parallel{
			&Compose{
				ValueStore: &LimitedValueStore{
					ValueStore: new(dummyValueStore),
					Namespaces: []string{"other"},
				},
			},
		},
		Tiered{
			&Compose{
				ValueStore: &LimitedValueStore{
					ValueStore: new(dummyValueStore),
					Namespaces: []string{"pk"},
				},
			},
		},
		&Compose{
			ValueStore: &LimitedValueStore{
				ValueStore: new(dummyValueStore),
				Namespaces: []string{"other", "pk"},
			},
		},
		&struct{ Compose }{Compose{ValueStore: &LimitedValueStore{ValueStore: Null{}}}},
		&struct{ Compose }{},
	}

	pid, _ := peert.RandPeerID()

	ctx := context.Background()
	if _, err := d.GetPublicKey(ctx, pid); err != routing.ErrNotFound {
		t.Fatal(err)
	}
}
