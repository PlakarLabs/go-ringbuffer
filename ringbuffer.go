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
	"io"
	"sync"
	"sync/atomic"
)

type RingBuffer struct {
	rd      io.Reader
	rdErr   error
	prefill []byte

	buffer      []byte
	bufferMutex sync.Mutex
	head        int32
	tail        int32
	filled      int32

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
	if rb.rdErr != nil {
		return
	}
	if rb.rd != nil && rb.unlockedCapacity() != 0 && rb.rdErr != io.EOF {
		n, err := rb.rd.Read(rb.prefill[0:rb.unlockedCapacity()])
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

func (rb *RingBuffer) IsFull() bool {
	return atomic.LoadInt32(&rb.filled) == 1
}

func (rb *RingBuffer) IsEmpty() bool {
	return atomic.LoadInt32(&rb.head) == atomic.LoadInt32(&rb.tail) && atomic.LoadInt32(&rb.filled) == 0
}

func (rb *RingBuffer) unlockedCapacity() int {
	if rb.IsFull() {
		return 0
	}

	delta := int(rb.tail) - int(rb.head)
	if delta < 0 {
		return -delta
	} else {
		return cap(rb.buffer) - delta
	}
}

func (rb *RingBuffer) Capacity() int {
	rb.bufferMutex.Lock()
	defer rb.bufferMutex.Unlock()
	return rb.unlockedCapacity()
}

func (rb *RingBuffer) unlockedLen() int {
	return cap(rb.buffer) - rb.unlockedCapacity()
}

func (rb *RingBuffer) Len() int {
	rb.bufferMutex.Lock()
	defer rb.bufferMutex.Unlock()

	return rb.unlockedLen()
}

func (rb *RingBuffer) writeBytes(data []byte) {
	rb.bufferMutex.Lock()
	defer rb.bufferMutex.Unlock()

	delta := int(rb.tail) - int(rb.head)
	if delta < 0 {
		delta = -delta
	} else {
		delta = cap(rb.buffer) - delta
	}

	if delta < len(data) {
		panic("RingBuffer overflow")
	}

	end := int(rb.tail) + len(data)
	if end <= cap(rb.buffer) {
		copy(rb.buffer[rb.tail:], data)
	} else {
		pivot := int32(cap(rb.buffer) - int(rb.tail))
		if pivot > 0 {
			copy(rb.buffer[rb.tail:], data[:pivot])
		}
		copy(rb.buffer, data[pivot:])
	}

	/*
		if int(rb.tail)+len(data) <= cap(rb.buffer) {
			copy(rb.buffer[rb.tail:], data)
		} else {
			pivot := cap(rb.buffer) - int(rb.tail)
			copy(rb.buffer[rb.tail:], data[:pivot])
			copy(rb.buffer[0:], data[pivot:])
		}
	*/

	rb.tail = int32((int(rb.tail) + len(data)) % cap(rb.buffer))
	if rb.head == rb.tail {
		rb.filled = 1
	}
}

func (rb *RingBuffer) skip(n int) {
	if n > rb.unlockedLen() {
		panic("RingBuffer overflow")
	}

	rb.head = int32((int(rb.head) + n) % cap(rb.buffer))
	rb.filled = 0
}

func (rb *RingBuffer) Skip(n int) {
	rb.callMutex.Lock()
	defer rb.callMutex.Unlock()

	if n > rb.unlockedLen() {
		n = rb.unlockedLen()
	}
	rb.skip(n)
}

func (rb *RingBuffer) Write(data []byte) (int, error) {
	rb.callMutex.Lock()
	defer rb.callMutex.Unlock()

	rb.bufferMutex.Lock()
	filled := rb.filled
	rb.bufferMutex.Unlock()
	if filled != 0 {
		return 0, ErrBufferFull
	}

	nbytes := len(data)
	capacity := rb.Capacity()
	if nbytes > capacity {
		nbytes = capacity
	}

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
	if n > rb.unlockedLen() {
		panic("RingBuffer underflow")
	}

	if int(rb.head)+n <= cap(rb.buffer) {
		copy(data, rb.buffer[rb.head:rb.head+int32(n)])
	} else {
		pivot := cap(rb.buffer) - int(rb.head)
		copy(data[:pivot], rb.buffer[rb.head:])
		copy(data[pivot:], rb.buffer[:n-pivot])
	}

	rb.head = int32((int(rb.head) + n) % cap(rb.buffer))
	rb.filled = 0
}

func (rb *RingBuffer) Read(p []byte) (int, error) {
	rb.callMutex.Lock()
	defer rb.callMutex.Unlock()

	nbytes := 0
	for nbytes < len(p) {
		rb.prefillBuffer()

		rblen := rb.unlockedLen()
		if rblen == 0 {
			if rb.rdErr != nil {
				if rb.rdErr == io.EOF {
					return nbytes, io.EOF
				}
				return 0, rb.rdErr
			}
		}

		// 3 cases:
		// 1. first one, there's more data in the ring buffer than we can handle
		if rblen > len(p)-nbytes {
			rblen = len(p) - nbytes
		}

		// 2. second one, there's less data in the ring buffer than we can handle
		rb.readBytes(p[nbytes : nbytes+rblen])
		nbytes += rblen

		if rb.unlockedLen() == 0 {
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
	if n > rb.unlockedLen() {
		panic("RingBuffer underflow")
	}

	if int(rb.head)+n <= cap(rb.buffer) {
		copy(data, rb.buffer[rb.head:rb.head+int32(n)])
	} else {
		pivot := cap(rb.buffer) - int(rb.head)
		copy(data[:pivot], rb.buffer[rb.head:])
		copy(data[pivot:], rb.buffer[:n-pivot])
	}
}

func (rb *RingBuffer) Peek(p []byte) (int, error) {
	rb.callMutex.Lock()
	defer rb.callMutex.Unlock()

	rb.prefillBuffer()
	rblen := rb.unlockedLen()
	if rblen > len(p) {
		rblen = len(p)
	}
	rb.peekBytes(p[:rblen])
	return rblen, rb.rdErr
}
