package netlink

import (
	"fmt"
	"os"
	"sort"
	"testing"
	//"context"
)

func TestList(t *testing.T) {
	list, err := List(t.Context())
	fmt.Fprintf(os.Stdout, "err: %+v\n", err)
	for _, i := range list {
		fmt.Fprintf(os.Stdout, "interface: %+v\n", i)
		for _, a := range i.AddrInfo {
			fmt.Fprintf(os.Stdout, "addr info: %+v\n", a)
		}
		fmt.Fprintf(os.Stdout, "sorted\n\n")
		sort.Sort(AddressMask(i.AddrInfo))
	}

	routes, err := Routes(t.Context())
	fmt.Fprintf(os.Stdout, "err: %+v\n", err)
	for _, r := range routes {
		fmt.Fprintf(os.Stdout, "%+v\n", r)
	}

	fmt.Fprintf(os.Stdout, "sorted\n")
	sort.Sort(RouteMask(routes))
	for _, r := range routes {
		fmt.Fprintf(os.Stdout, "%+v\n", r)
	}

	metrics, err := Metrics(t.Context())
	fmt.Fprintf(os.Stdout, "err: %+v\n", err)
	for _, m := range metrics {
		fmt.Fprintf(os.Stdout, "%+v\n", m)
	}
}
