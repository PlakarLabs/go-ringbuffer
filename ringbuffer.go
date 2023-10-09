/*
 * Copyright (c) 2023 Gilles Chehade <gilles@poolp.org>
 *
 * Permission to use, copy, modify, and distribute this software for any
 * purpose with or without fee is hereby granted, provided that the above
 * copyright notice and this permission notice appear in all copies.
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

package ringbuffer

import (
	"io"
)

type RingBuffer struct {
	rd    io.Reader
	rdErr error

	buffer []byte
	head   int
	tail   int

	filled bool
}

func New(size int) *RingBuffer {
	return &RingBuffer{
		buffer: make([]byte, size),
	}
}

func NewReaderSize(rd io.Reader, size int) *RingBuffer {
	rb := New(size)
	rb.rd = rd
	return rb
}

func (rb *RingBuffer) prefillBuffer() int {
	totalCapacity := rb.unlockedCapacity()
	totalLen := rb.unlockedLen()

	if rb.rd == nil || rb.rdErr != nil || totalCapacity == 0 {
		return totalLen
	}

	var rCapacity int
	if rb.tail < rb.head {
		rCapacity = rb.head - rb.tail
	} else {
		rCapacity = cap(rb.buffer) - rb.tail
	}

	n, err := rb.rd.Read(rb.buffer[rb.tail : rb.tail+rCapacity])
	if err != nil && err != io.EOF {
		rb.rd = nil
		rb.rdErr = err
		return totalLen
	}
	if n != 0 {
		rb.tail = (rb.tail + n) % cap(rb.buffer)
		totalLen += n
	}

	if rCapacity < totalCapacity && err != io.EOF {
		lCapacity := totalCapacity - rCapacity
		n, err := rb.rd.Read(rb.buffer[rb.tail : rb.tail+lCapacity])
		if err != nil && err != io.EOF {
			rb.rd = nil
			rb.rdErr = err
			return totalLen
		}
		if n != 0 {
			rb.tail = (rb.tail + n) % cap(rb.buffer)
			totalLen += n
		}
	}

	if rb.head == rb.tail {
		rb.filled = true
	}

	if err == io.EOF {
		rb.rd = nil
		rb.rdErr = io.EOF
	}
	return totalLen
}

func (rb *RingBuffer) unlockedCapacity() int {
	if rb.filled {
		return 0
	}

	delta := rb.tail - rb.head
	if delta < 0 {
		return -delta
	} else {
		return cap(rb.buffer) - delta
	}
}

func (rb *RingBuffer) unlockedLen() int {
	return cap(rb.buffer) - rb.unlockedCapacity()
}

func (rb *RingBuffer) Discard(n int) (int, error) {
	if n > rb.unlockedLen() {
		n = rb.unlockedLen()
	}
	rb.head = (int(rb.head) + n) % cap(rb.buffer)
	rb.filled = false
	return n, nil
}

func (rb *RingBuffer) copyToBuffer(data []byte, start int) {
	end := start + len(data)
	if end <= cap(rb.buffer) {
		copy(data, rb.buffer[start:end])
	} else {
		pivot := cap(rb.buffer) - start
		copy(data[:pivot], rb.buffer[start:])
		copy(data[pivot:], rb.buffer[:end%cap(rb.buffer)])
	}
}

func (rb *RingBuffer) Peek(p []byte) (int, error) {
	size := len(p)
	rblen := rb.unlockedLen()
	if size > rblen && rb.rd != nil {
		rblen = rb.prefillBuffer()
	}
	if rblen < size {
		size = rblen
	} else if rblen > size {
		rblen = size
	}

	rb.copyToBuffer(p[:rblen], int(rb.head))
	return rblen, rb.rdErr
}

func (rb *RingBuffer) Read(p []byte) (int, error) {
	size := len(p)
	rblen := rb.unlockedLen()
	if size > rblen && rb.rd != nil {
		rblen = rb.prefillBuffer()
	}
	if rblen < size {
		size = rblen
	} else if rblen > size {
		rblen = size
	}

	rb.copyToBuffer(p[:rblen], int(rb.head))
	rb.Discard(rblen)
	return rblen, rb.rdErr
}
