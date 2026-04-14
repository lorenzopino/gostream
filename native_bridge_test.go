package main

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestStreamTorrentToPipeRecoversPanic(t *testing.T) {
	pr, pw := io.Pipe()

	go streamTorrentToPipe(pw, "test stream", func() error {
		panic("piece window mismatch")
	})

	_, err := io.ReadAll(pr)
	if err == nil {
		t.Fatal("expected pipe read to fail after recovered panic")
	}
	if !strings.Contains(err.Error(), "panic") {
		t.Fatalf("expected panic error, got %v", err)
	}
}

func TestStreamTorrentToPipePropagatesStreamError(t *testing.T) {
	pr, pw := io.Pipe()
	want := errors.New("stream failed")

	go streamTorrentToPipe(pw, "test stream", func() error {
		return want
	})

	_, err := io.ReadAll(pr)
	if !errors.Is(err, want) {
		t.Fatalf("expected stream error %v, got %v", want, err)
	}
}
