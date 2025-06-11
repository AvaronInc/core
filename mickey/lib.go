package mickey

import (
	"io"
	"sync"
	"log"
)

type Muxer struct {
	lock sync.RWMutex
	r io.Reader // underlying reader
	b []byte    // underlying buffer
	err error   // typically eof
}

type reader struct {
	m *Muxer // pointer to parent muxer
	n int    // bytes read to output buffer 
}

func (m *Muxer) EOF() bool {
	m.lock.RLock()
	b := m.err != nil
	m.lock.RUnlock()
	return b
}

func (r *reader) Read(p []byte) (n int, err error) {
	defer log.Println("mickey muxer returning", n, err)
	m := r.m
	var mn int // muxer number of bytes read


	// if there's stuff in the left muxer to read
	m.lock.Lock()
	defer m.lock.Unlock()
	mn = len(m.b)
	err = m.err // (while we still have the lock)
	if r.n < mn {
		n = copy(p, m.b[r.n:])
	}

	r.n += n
	if n == len(p) {
		// we filled the whole buffer
		// probably no use trying to read from underlying
		return
	}

	// conditions
	// - we HAVE NOT filled the dest buffer
	// - BUT we have fully consumed the bytes we've read from underlying
	// need to check for EOF
	if r.n == mn && err != nil {
		// err is typically EOF 
		return
	} else if r.n > mn {
		// sanity check
		panic("mickey muxer client read more bytes than underlying muxer")
	}

	// conditions
	// - we HAVE NOT filled the dest buffer(still have space left)
	// - we have FULLY CONSUMED the bytes which were read from underlying
	// - we HAVE NOT hit EOF
	// we wil use the rest of the destination buffer to read
	// this will tell us how much we need to alloc to the underlying buffer
	mn, err = m.r.Read(p[n:]) // mn is now the bytes read from underlying buffer
	m.b = append(m.b, p[n:n+mn]...)
	m.err = err

	if n > 0 {
		err = nil
	}

	n += mn
	r.n += mn
	return
}

// Primary constructor for this package
func New(r io.Reader) *Muxer {
	return &Muxer{
		// no need to alloc byte buffer
		// the end user allocates the destination buffer,
		// from which we copy & realloc internal buffer
		r: r,
	}
}

func (m *Muxer) NewReader() *reader {
	return &reader{
		m: m,
		n: 0,
	}
}

