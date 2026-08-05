package sfu

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/pion/rtcp"
)

type recordingWriter struct {
	mu      sync.Mutex
	packets [][]rtcp.Packet
	err     error
}

func (w *recordingWriter) WriteRTCP(packets []rtcp.Packet) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err != nil {
		return w.err
	}
	w.packets = append(w.packets, packets)
	return nil
}

func (w *recordingWriter) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.packets)
}

func TestKeyframeRequesterCoalescesRequests(t *testing.T) {
	requester := NewKeyframeRequester(time.Second)
	now := time.Unix(0, 0)
	requester.now = func() time.Time { return now }
	writer := &recordingWriter{}

	sent, err := requester.Request(writer, 42)
	if err != nil || !sent {
		t.Fatalf("expected first request to be sent, sent=%v err=%v", sent, err)
	}
	sent, err = requester.Request(writer, 42)
	if err != nil || sent {
		t.Fatalf("expected second request to be coalesced, sent=%v err=%v", sent, err)
	}
	if writer.count() != 1 {
		t.Fatalf("expected exactly 1 rtcp write, got %d", writer.count())
	}

	// A different source is independent of the throttled one.
	if sent, _ := requester.Request(writer, 43); !sent {
		t.Fatal("expected a different ssrc to be requested immediately")
	}

	now = now.Add(2 * time.Second)
	if sent, _ := requester.Request(writer, 42); !sent {
		t.Fatal("expected request to be allowed once the interval elapsed")
	}

	emitted, throttled := requester.Stats()
	if emitted != 3 || throttled != 1 {
		t.Fatalf("expected 3 emitted / 1 throttled, got %d / %d", emitted, throttled)
	}
}

func TestKeyframeRequesterWritesPictureLossIndication(t *testing.T) {
	requester := NewKeyframeRequester(0)
	writer := &recordingWriter{}

	if _, err := requester.Request(writer, 7); err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if writer.count() != 1 {
		t.Fatalf("expected 1 write, got %d", writer.count())
	}
	pli, ok := writer.packets[0][0].(*rtcp.PictureLossIndication)
	if !ok {
		t.Fatalf("expected a PictureLossIndication, got %T", writer.packets[0][0])
	}
	if pli.MediaSSRC != 7 {
		t.Fatalf("expected ssrc 7, got %d", pli.MediaSSRC)
	}
}

func TestKeyframeRequesterIgnoresInvalidTargets(t *testing.T) {
	requester := NewKeyframeRequester(time.Second)

	if sent, err := requester.Request(nil, 1); sent || err != nil {
		t.Fatalf("expected nil writer to be a no-op, sent=%v err=%v", sent, err)
	}
	if sent, err := requester.Request(&recordingWriter{}, 0); sent || err != nil {
		t.Fatalf("expected zero ssrc to be a no-op, sent=%v err=%v", sent, err)
	}
	if emitted, throttled := requester.Stats(); emitted != 0 || throttled != 0 {
		t.Fatalf("expected no counters to move, got %d / %d", emitted, throttled)
	}
}

func TestKeyframeRequesterPropagatesWriteError(t *testing.T) {
	requester := NewKeyframeRequester(0)
	writer := &recordingWriter{err: errors.New("closed")}

	sent, err := requester.Request(writer, 9)
	if sent {
		t.Fatal("expected sent=false when the write fails")
	}
	if err == nil {
		t.Fatal("expected the write error to be propagated")
	}
}

func TestKeyframeRequesterForget(t *testing.T) {
	requester := NewKeyframeRequester(time.Hour)
	writer := &recordingWriter{}

	if _, err := requester.Request(writer, 11); err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if requester.Tracked() != 1 {
		t.Fatalf("expected 1 tracked ssrc, got %d", requester.Tracked())
	}

	requester.Forget(11)
	if requester.Tracked() != 0 {
		t.Fatalf("expected tracking to be cleared, got %d", requester.Tracked())
	}
	if sent, _ := requester.Request(writer, 11); !sent {
		t.Fatal("expected a forgotten ssrc to be requestable again")
	}
}

func TestKeyframeRequestDetection(t *testing.T) {
	cases := []struct {
		name   string
		packet rtcp.Packet
		want   bool
	}{
		{"pli", &rtcp.PictureLossIndication{}, true},
		{"fir", &rtcp.FullIntraRequest{}, true},
		{"receiver report", &rtcp.ReceiverReport{}, false},
		{"nack", &rtcp.TransportLayerNack{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsKeyframeRequest(tc.packet); got != tc.want {
				t.Fatalf("IsKeyframeRequest(%T) = %v, want %v", tc.packet, got, tc.want)
			}
		})
	}

	if ContainsKeyframeRequest([]rtcp.Packet{&rtcp.ReceiverReport{}, &rtcp.SenderReport{}}) {
		t.Fatal("expected no keyframe request in a report only compound")
	}
	if !ContainsKeyframeRequest([]rtcp.Packet{&rtcp.ReceiverReport{}, &rtcp.FullIntraRequest{}}) {
		t.Fatal("expected the FIR in the compound to be detected")
	}
}

func TestKeyframeRequesterConcurrentRequests(t *testing.T) {
	requester := NewKeyframeRequester(time.Hour)
	writer := &recordingWriter{}

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = requester.Request(writer, 100)
		}()
	}
	wg.Wait()

	if writer.count() != 1 {
		t.Fatalf("expected exactly one request to win the race, got %d", writer.count())
	}
	emitted, throttled := requester.Stats()
	if emitted != 1 || throttled != 31 {
		t.Fatalf("expected 1 emitted / 31 throttled, got %d / %d", emitted, throttled)
	}
}
