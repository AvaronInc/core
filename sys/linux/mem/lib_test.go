package mem

import (
	"fmt"
	"os"
	"testing"
)

func TestMeminfo(t *testing.T) {
	m, err := meminfo()
	fmt.Fprintf(os.Stdout, "err: %+v\n", err)
	for k, v := range m {
		fmt.Fprintf(os.Stdout, "%s: %+v\n", k, v)
	}
}
