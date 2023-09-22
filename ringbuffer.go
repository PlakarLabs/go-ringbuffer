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
	"errors"
	"fmt"
	"io"
	"sync"
)

type RingBuffer struct {
	rd      io.Reader
	rdErr   error
	prefill []byte

	buffer      []byte
	bufferMutex sync.Mutex
	head        int
	tail        int
	filled      bool

	callMutex sync.Mutex
}

var ErrBufferFull = errors.New("ringbuffer: buffer is full")

func New(size int) *RingBuffer {
	return &RingBuffer{
		buffer: make([]byte, size),
	}
}

func NewReaderSize(rd io.Reader, size int) *RingBuffer {
	rb := New(size)
	rb.rd = rd
	rb.prefill = make([]byte, size)
	rb.prefillBuffer()
	return rb
}

func (rb *RingBuffer) prefillBuffer() {
	if rb.rd != nil && rb.Capacity() != 0 && rb.rdErr != io.EOF {
		n, err := rb.rd.Read(rb.prefill[0:rb.Capacity()])
		if err != nil && err != io.EOF {
			rb.rdErr = err
			return
		}
		rb.writeBytes(rb.prefill[:n])
		if err == io.EOF {
			rb.rd = nil
			rb.rdErr = io.EOF
		}
	}
}

func (rb *RingBuffer) Slice(start, end int) []byte {
	rb.bufferMutex.Lock()
	defer rb.bufferMutex.Unlock()
	return rb.buffer[start:end]
}

func (rb *RingBuffer) IsEmpty() bool {
	rb.bufferMutex.Lock()
	defer rb.bufferMutex.Unlock()
	return rb.head == rb.tail && !rb.filled
}

func (rb *RingBuffer) IsFull() bool {
	rb.bufferMutex.Lock()
	defer rb.bufferMutex.Unlock()
	return rb.filled
}

func (rb *RingBuffer) capacity() int {
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

func (rb *RingBuffer) Capacity() int {
	rb.bufferMutex.Lock()
	defer rb.bufferMutex.Unlock()
	return rb.capacity()
}

func (rb *RingBuffer) len() int {
	return cap(rb.buffer) - rb.capacity()
}

func (rb *RingBuffer) Len() int {
	rb.bufferMutex.Lock()
	defer rb.bufferMutex.Unlock()

	return rb.len()
}

func (rb *RingBuffer) writeBytes(data []byte) {
	rb.bufferMutex.Lock()
	defer rb.bufferMutex.Unlock()

	delta := rb.tail - rb.head
	if delta < 0 {
		delta = -delta
	} else {
		delta = cap(rb.buffer) - delta
	}

	if delta < len(data) {
		panic("RingBuffer overflow")
	}

	if rb.tail+len(data) <= cap(rb.buffer) {
		copy(rb.buffer[rb.tail:], data)
	} else {
		pivot := cap(rb.buffer) - rb.tail
		copy(rb.buffer[rb.tail:], data[:pivot])
		copy(rb.buffer[0:], data[pivot:])
	}

	rb.tail = (rb.tail + len(data)) % cap(rb.buffer)
	if rb.head == rb.tail {
		rb.filled = true
	}
}

func (rb *RingBuffer) Write(data []byte) (int, error) {
	rb.callMutex.Lock()
	defer rb.callMutex.Unlock()

	rb.bufferMutex.Lock()
	filled := rb.filled
	rb.bufferMutex.Unlock()
	if filled {
		return 0, ErrBufferFull
	}

	nbytes := len(data)
	capacity := rb.Capacity()
	if nbytes > capacity {
		nbytes = capacity
	}

	fmt.Println("will attempt to write", nbytes)

	rb.writeBytes(data[:nbytes])

	//fmt.Println("write:", "head", rb.head, "tail", rb.tail, "filled", rb.filled, "len", rb.Len(), "cap", rb.Capacity())
	if nbytes != len(data) {
		return nbytes, io.ErrShortWrite
	}
	return nbytes, nil
}

func (rb *RingBuffer) readBytes(data []byte) {
	rb.bufferMutex.Lock()
	defer rb.bufferMutex.Unlock()

	n := len(data)
	if n > rb.len() {
		panic("RingBuffer underflow")
	}

	if rb.head+n <= cap(rb.buffer) {
		copy(data, rb.buffer[rb.head:rb.head+n])
	} else {
		pivot := cap(rb.buffer) - rb.head
		copy(data[:pivot], rb.buffer[rb.head:])
		copy(data[pivot:], rb.buffer[:n-pivot])
	}

	rb.head = (rb.head + n) % cap(rb.buffer)
	rb.filled = false
}

func (rb *RingBuffer) Read(p []byte) (int, error) {
	rb.callMutex.Lock()
	defer rb.callMutex.Unlock()

	nbytes := 0
	for nbytes < len(p) {
		rb.prefillBuffer()

		if rb.len() == 0 {
			if rb.rdErr != nil {
				if rb.rdErr == io.EOF {
					return nbytes, io.EOF
				}
				return 0, rb.rdErr
			}
		}

		// 3 cases:
		// 1. first one, there's more data in the ring buffer than we can handle
		rblen := rb.len()
		if rblen > len(p)-nbytes {
			rblen = len(p) - nbytes
		}

		// 2. second one, there's less data in the ring buffer than we can handle
		rb.readBytes(p[nbytes : nbytes+rblen])
		nbytes += rblen

		if rb.len() == 0 {
			return nbytes, nil
		}
	}
	if nbytes != 0 {
		return nbytes, nil
	}

	return 0, rb.rdErr
}

func (rb *RingBuffer) peekBytes(data []byte) {
	rb.bufferMutex.Lock()
	defer rb.bufferMutex.Unlock()

	n := len(data)
	if n > rb.len() {
		panic("RingBuffer underflow")
	}

	if rb.head+n <= cap(rb.buffer) {
		copy(data, rb.buffer[rb.head:rb.head+n])
	} else {
		pivot := cap(rb.buffer) - rb.head
		copy(data[:pivot], rb.buffer[rb.head:])
		copy(data[pivot:], rb.buffer[:n-pivot])
	}
}

func (rb *RingBuffer) Peek(p []byte) (int, error) {
	rb.callMutex.Lock()
	defer rb.callMutex.Unlock()

	rb.prefillBuffer()
	rblen := rb.len()
	if rblen > len(p) {
		rblen = len(p)
	}
	rb.peekBytes(p[:rblen])
	return rblen, rb.rdErr
}
