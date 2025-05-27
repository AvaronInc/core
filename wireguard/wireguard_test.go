package wireguard

import (
	"fmt"
	"testing"
	//"context"
)

func Test(t *testing.T) {
	interfaces, err := Interfaces(t.Context())
	if err != nil {
		panic(err)
	}
	for name, i := range interfaces {
		for k, peer := range i.Peers {
			fmt.Printf("%s - %s: %+v\n", name, k, peer)
		}
	}
}
