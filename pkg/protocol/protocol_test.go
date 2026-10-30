package protocol

import (
	"bytes"
	"errors"
	"testing"
)

func TestAgentStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		status   AgentStatus
		expected bool
	}{
		{AgentStatusPending, false},
		{AgentStatusStarting, false},
		{AgentStatusRunning, false},
		{AgentStatusStopped, true},
		{AgentStatusFailed, true},
		{AgentStatusKilled, true},
	}

	for _, tc := range tests {
		if got := tc.status.IsTerminal(); got != tc.expected {
			t.Errorf("status %s: expected IsTerminal() = %v, got %v", tc.status, tc.expected, got)
		}
	}
}

func TestAgentLaunchRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     AgentLaunchRequest
		wantErr bool
	}{
		{
			name:    "empty command",
			req:     AgentLaunchRequest{Command: ""},
			wantErr: true,
		},
		{
			name:    "negative timeout",
			req:     AgentLaunchRequest{Command: "ls", TimeoutSeconds: -1},
			wantErr: true,
		},
		{
			name:    "valid request with default name",
			req:     AgentLaunchRequest{Command: "python3"},
			wantErr: false,
		},
		{
			name:    "valid full request",
			req:     AgentLaunchRequest{Name: "worker", Command: "node", TimeoutSeconds: 60},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() err = %v, wantErr = %v", err, tc.wantErr)
			}
			if !tc.wantErr && tc.req.Name == "" {
				t.Errorf("expected default name to be set, got empty")
			}
		})
	}
}

func TestFrame_ReadWriteRoundtrip(t *testing.T) {
	payloads := [][]byte{
		[]byte("hello, mace protocol!"),
		[]byte("{}"),
		bytes.Repeat([]byte("A"), 65536), // 64 KB
	}

	for _, payload := range payloads {
		var buf bytes.Buffer
		if err := WriteFrame(&buf, payload); err != nil {
			t.Fatalf("WriteFrame failed: %v", err)
		}

		decoded, err := ReadFrame(&buf)
		if err != nil {
			t.Fatalf("ReadFrame failed: %v", err)
		}

		if !bytes.Equal(decoded, payload) {
			t.Fatalf("payload mismatch: got len %d, want len %d", len(decoded), len(payload))
		}
	}
}

func TestFrame_ExceedsMaxSize(t *testing.T) {
	var buf bytes.Buffer
	// Craft a header with length > MaxFrameSize
	tooLarge := uint32(MaxFrameSize + 1)
	buf.Write([]byte{
		byte(tooLarge >> 24),
		byte(tooLarge >> 16),
		byte(tooLarge >> 8),
		byte(tooLarge),
	})

	_, err := ReadFrame(&buf)
	if err == nil {
		t.Fatal("expected ErrFrameTooLarge error, got nil")
	}
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("expected ErrFrameTooLarge, got %v", err)
	}
}
