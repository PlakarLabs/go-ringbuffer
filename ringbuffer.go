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
)

type RingBuffer struct {
	rd     io.Reader
	buffer []byte
	rdErr  error

	head   int
	tail   int
	filled bool

	canFill chan bool
	canRead chan bool
}

var ErrBufferFull = errors.New("ringbuffer: buffer is full")

func NewReaderSize(rd io.Reader, size int) *RingBuffer {
	rb := &RingBuffer{
		rd:      rd,
		buffer:  make([]byte, size),
		canFill: make(chan bool, 1),
		canRead: make(chan bool, 1),
	}
	go func() {
		rb.canFill <- true
		fillbuf := make([]byte, size)

		for {
			<-rb.canFill
			capacity := rb.Capacity()

			n, err := rb.rd.Read(fillbuf[:capacity])
			if err != nil && err != io.EOF {
				rb.rdErr = err
				break
			}

			rb.Write(fillbuf[:n])
			if err == io.EOF {
				rb.rdErr = io.EOF
				close(rb.canFill)
				break
			}
		}

	}()

	return rb
}

func (rb *RingBuffer) IsEmpty() bool {
	return rb.head == rb.tail && !rb.filled
}

func (rb *RingBuffer) IsFull() bool {
	return rb.filled
}

func (rb *RingBuffer) Capacity() int {
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

func (rb *RingBuffer) Len() int {
	return cap(rb.buffer) - rb.Capacity()
}

func (rb *RingBuffer) Write(data []byte) (int, error) {
	if rb.filled {
		return 0, ErrBufferFull
	}

	nbytes := len(data)

	var err error
	delta := rb.tail - rb.head
	if delta < 0 {
		delta = -delta
		if delta < len(data) {
			nbytes = delta
		}
		copy(rb.buffer[rb.tail:], data[:nbytes])
	} else {
		// tail is after head ... BUT may overlap buffer boundary
		delta = cap(rb.buffer) - delta
		if delta < len(data) {
			nbytes = delta
		}

		if rb.tail+nbytes <= cap(rb.buffer) {
			copy(rb.buffer[rb.tail:], data[:nbytes])
		} else {
			pivot := cap(rb.buffer) - rb.tail
			copy(rb.buffer[rb.tail:], data[:pivot])
			copy(rb.buffer[0:], data[pivot:])
		}
	}

	rb.tail = (rb.tail + nbytes) % cap(rb.buffer)
	if rb.head == rb.tail {
		rb.filled = true
	}

	if nbytes != len(data) {
		err = io.ErrShortWrite
	}

	if !rb.IsEmpty() {
		rb.canRead <- true
	}

	if rb.Capacity() != 0 {
		rb.canFill <- true
	}

	return nbytes, err
}

func (rb *RingBuffer) Read(p []byte) (int, error) {
	if rb.rdErr != nil && rb.rdErr != io.EOF {
		return 0, rb.rdErr
	}

	if rb.rdErr == io.EOF && rb.IsEmpty() {
		return 0, rb.rdErr
	}

	<-rb.canRead

	if !rb.filled && rb.head == rb.tail {
		return 0, io.EOF
	}

	nbytes := len(p)
	if nbytes > rb.Len() {
		nbytes = rb.Len()
	}

	if rb.head <= rb.tail {
		copy(p[:nbytes], rb.buffer[rb.head:rb.head+nbytes])
		rb.head = (rb.head + nbytes) % cap(rb.buffer)
	} else {
		// Tail is before head, so we need to read in two parts
		available := cap(rb.buffer) - rb.head
		if nbytes <= available {
			copy(p, rb.buffer[rb.head:rb.head+nbytes])
			rb.head = (rb.head + nbytes) % cap(rb.buffer)
		} else {
			copy(p, rb.buffer[rb.head:])
			copy(p[available:], rb.buffer[:nbytes-available])
			rb.head = nbytes - available
		}
	}

	rb.filled = false
	if rb.rdErr != io.EOF {
		rb.canFill <- true
	}

	if !rb.IsEmpty() {
		rb.canRead <- true
	}

	return nbytes, nil
}
