package mem

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

type info struct {
	value int64
	unit  string
}

func (info info) String() string {
	return fmt.Sprintf("%d %s", info.value, info.unit)
}

func meminfo() (map[string]info, error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return nil, err
	}

	m := make(map[string]info)

	scanner := bufio.NewScanner(file)

	var (
		line string
		f    []string
		info info
	)

	for scanner.Scan() {
		line = scanner.Text()
		f = strings.Fields(line)
		switch len(f) {
		case 3:
			info.unit = f[2]
			fallthrough
		case 2:
			if info.value, err = strconv.ParseInt(f[1], 10, 64); err == nil && len(f[0]) > 0 {
				break
			}
			fallthrough
		default:
			log.Printf("junk line from %s: '%s'\n", file.Name(), line)
			continue
		}

		if f[0][len(f[0])-1] == ':' {
			f[0] = f[0][:len(f[0])-1]
		}

		m[f[0]] = info
	}
	return m, nil
}

func GetTotal() (int64, error) {
	m, err := meminfo()
	if err != nil {
		return 0, err
	}
	info, ok := m["MemTotal"]
	if !ok {
		return 0, fmt.Errorf("MemTotal missing from /proc/meminfo")
	}
	var total int64
	switch info.unit {
	case "kB":
		total = info.value * 1000
	default:
		panic("unknown unit given by /proc/meminfo")
	}

	return total, nil
}
