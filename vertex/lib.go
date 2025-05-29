package vertex

import (
	"bytes"
	"encoding/base64"
	"io"
	"log"
	"net"
	"strings"
)

type Key [32]byte

func (k Key) String() string {
	return base64.StdEncoding.EncodeToString(k[:])
}

func (k *Key) UnmarshalText(buf []byte) (int64, error) {
	if len(buf) < 44 {
		return 0, io.ErrShortBuffer
	}

	log.Printf("decoding buf: '%s'\n", buf[:])
	_, err := base64.StdEncoding.Decode(k[:], bytes.TrimSpace(buf[:]))
	return int64(len(buf)), err
}

func (k Key) MarshalText() ([]byte, error) {
	buf := make([]byte, 44)
	base64.StdEncoding.Encode(buf[:], k[:])
	return buf, nil
}

func (k Key) Path() string {
	return strings.Replace(k.String(), "/", "-", -1)
}

func (k Key) GlobalAddress() *net.IPNet {
	var n net.IPNet
	n.IP = make([]byte, net.IPv6len)
	n.Mask = make([]byte, net.IPv6len)

	if len(k) < net.IPv6len {
		panic("key should be longer than IPv6 address")
	}

	var (
		prefix = []byte{0xfc, 0x00, 0xa7, 0xa0}
		mask   = []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	)

	copy(n.IP, prefix)
	copy(n.Mask, mask)

	for i := 0; i < net.IPv6len-len(prefix); i++ {
		n.IP[i+len(prefix)] = k[i]
	}

	return &n
}
