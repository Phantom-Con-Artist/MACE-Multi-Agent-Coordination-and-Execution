package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	// MaxFrameSize limits the maximum allowable frame payload to 16MB to prevent memory exhaustion.
	MaxFrameSize = 16 * 1024 * 1024
)

var (
	ErrFrameTooLarge = errors.New("protocol: frame exceeds maximum allowable size")
)

// ReadFrame reads a 4-byte big-endian length-prefixed payload from the reader.
func ReadFrame(r io.Reader) ([]byte, error) {
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return nil, err
	}

	if length > MaxFrameSize {
		return nil, fmt.Errorf("%w: %d > %d", ErrFrameTooLarge, length, MaxFrameSize)
	}

	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}

	return buf, nil
}

// WriteFrame writes a payload prefixed with a 4-byte big-endian length to the writer.
func WriteFrame(w io.Writer, payload []byte) error {
	length := uint32(len(payload))
	if length > MaxFrameSize {
		return fmt.Errorf("%w: %d > %d", ErrFrameTooLarge, length, MaxFrameSize)
	}

	if err := binary.Write(w, binary.BigEndian, length); err != nil {
		return err
	}

	_, err := w.Write(payload)
	return err
}
