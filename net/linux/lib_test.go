package netlink

import (
	"fmt"
	"os"
	"testing"
	//"context"
)

func TestList(t *testing.T) {
	list, err := List(t.Context())
	fmt.Fprintf(os.Stdout, "err: %+v\n", err)
	for _, i := range list {
		fmt.Fprintf(os.Stdout, "interface: %+v\n", i)
	}
}
