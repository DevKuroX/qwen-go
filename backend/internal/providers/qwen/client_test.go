package qwen

import (
	"context"
	"io"
	"testing"

	"go.uber.org/zap"
)

type chunkedReader struct {
	chunks []string
	index  int
}

func (r *chunkedReader) Read(p []byte) (int, error) {
	if r.index >= len(r.chunks) {
		return 0, io.EOF
	}
	n := copy(p, r.chunks[r.index])
	r.index++
	return n, nil
}

func collectStream(t *testing.T, reader io.Reader) []StreamChunk {
	t.Helper()
	client := &QwenClient{logger: zap.NewNop()}
	ch := make(chan StreamChunk, 16)
	if err := client.readSSEStream(context.Background(), reader, ch); err != nil && err != io.EOF {
		t.Fatalf("readSSEStream() error = %v", err)
	}
	close(ch)

	var chunks []StreamChunk
	for chunk := range ch {
		chunks = append(chunks, chunk)
	}
	return chunks
}

func TestStreamChatHandlesFragmentedSSE(t *testing.T) {
	chunks := collectStream(t, &chunkedReader{chunks: []string{
		"data: {\"choices\":[{\"delta\":{\"con",
		"tent\":\"hel",
		"lo\"}}]}\n\n",
	}})

	if len(chunks) != 1 || chunks[0].Content != "hello" || chunks[0].Done {
		t.Fatalf("chunks = %+v, want single hello chunk", chunks)
	}
}

func TestStreamChatHandlesMultipleEventsPerRead(t *testing.T) {
	chunks := collectStream(t, &chunkedReader{chunks: []string{
		"data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"b\"}}]}\n\n",
	}})

	if len(chunks) != 2 || chunks[0].Content != "a" || chunks[1].Content != "b" {
		t.Fatalf("chunks = %+v, want a,b", chunks)
	}
}

func TestStreamChatHandlesDoneMarker(t *testing.T) {
	chunks := collectStream(t, &chunkedReader{chunks: []string{"data: [DONE]\n\n"}})

	if len(chunks) != 1 || !chunks[0].Done {
		t.Fatalf("chunks = %+v, want single done chunk", chunks)
	}
}

func TestStreamChatSkipsMalformedEventAndContinues(t *testing.T) {
	chunks := collectStream(t, &chunkedReader{chunks: []string{
		"data: {bad json}\n\n",
		"data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n",
	}})

	if len(chunks) != 1 || chunks[0].Content != "ok" {
		t.Fatalf("chunks = %+v, want single ok chunk", chunks)
	}
}
