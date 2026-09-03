package crdt

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"
)

type Edit struct {
	Position int
	IsInsert bool
	Char     string
}

type Trace struct {
	Edits     []Edit `json:"edits"`
	FinalText string `json:"finalText"`
}

func (e *Edit) UnmarshalJSON(buf []byte) error {
	var temp []any
	if err := json.Unmarshal(buf, &temp); err != nil {
		return err
	}
	if len(temp) >= 1 {
		if val, ok := temp[0].(float64); ok {
			e.Position = int(val)
		}
	}
	if len(temp) >= 2 {
		if val, ok := temp[1].(float64); ok {
			e.IsInsert = val == 0
		}
	}
	if len(temp) == 3 {
		if val, ok := temp[2].(string); ok {
			e.Char = val
		}
	}
	return nil
}

var (
	traceOnce    sync.Once
	loadedTrace  *Trace
	traceLoadErr error
)

// loadTrace decodes resources/editing-trace.json once.
func loadTrace() (*Trace, error) {
	traceOnce.Do(func() {
		jsonFile, err := os.Open("../../resources/editing-trace.json")
		if err != nil {
			traceLoadErr = fmt.Errorf("failed to open JSON file: %w", err)
			return
		}
		defer jsonFile.Close()

		var trace Trace
		if err := json.NewDecoder(jsonFile).Decode(&trace); err != nil {
			traceLoadErr = fmt.Errorf("failed to decode JSON file: %w", err)
			return
		}
		loadedTrace = &trace
	})
	return loadedTrace, traceLoadErr
}

// TestMain preloads the trace when a profile is requested: the testing
// harness starts -cpuprofile/-memprofile inside m.Run, so decoding before
// that keeps the JSON decode out of the profile. Flags are parsed here
// because m.Run has not parsed them yet at this point.
func TestMain(m *testing.M) {
	flag.Parse()
	for _, name := range []string{"test.cpuprofile", "test.memprofile"} {
		if f := flag.Lookup(name); f != nil && f.Value.String() != "" {
			loadTrace()
			break
		}
	}
	os.Exit(m.Run())
}

// replay applies every trace edit to a fresh document. When csv is non-nil it
// also records the running average per-op time every 500 edits (the plot
// pipeline's trace-data.csv format); timing overhead is skipped entirely for
// untimed replays.
func replay(trace *Trace, csv io.Writer) *RuneDocument {
	document := NewRuneDocument(0)
	timeSum := time.Duration(0)
	plotEvery := 500

	for i, edit := range trace.Edits {
		var opStart time.Time
		if csv != nil {
			opStart = time.Now()
		}
		if edit.IsInsert {
			document.Ins(edit.Position, edit.Char)
		} else {
			document.Del(edit.Position, 1)
		}
		if csv != nil {
			timeSum += time.Since(opStart)
			if i%plotEvery == 0 {
				avgTime := float64(timeSum.Nanoseconds()) / float64(plotEvery)
				fmt.Fprintf(csv, "%d,%d,%t,%q,%.5f\n", i, edit.Position, edit.IsInsert, edit.Char, avgTime)
				timeSum = 0
			}
		}
	}
	return document
}

// TestTrace verifies replaying the full real-world editing trace converges to
// the recorded final text.
func TestTrace(t *testing.T) {
	trace, err := loadTrace()
	if err != nil {
		t.Fatal(err)
	}
	document := replay(trace, nil)
	if trace.FinalText != document.GetString() {
		t.Fatalf("Mismatch, got '%q'", document.GetString())
	}
}

// BenchmarkTrace measures replaying the full editing trace. It also writes
// trace-data.csv (untimed, after the measured loop) and prints wall time and
// final memory for that replay.
func BenchmarkTrace(b *testing.B) {
	trace, err := loadTrace()
	if err != nil {
		b.Fatal(err)
	}

	var document *RuneDocument
	for b.Loop() {
		b.StopTimer()
		document = NewRuneDocument(0)
		b.StartTimer()
		for _, edit := range trace.Edits {
			if edit.IsInsert {
				document.Ins(edit.Position, edit.Char)
			} else {
				document.Del(edit.Position, 1)
			}
		}
	}

	// Untimed epilogue: plot CSV + wall-time/memory report, and a final
	// correctness check on the epilogue replay.
	csv, err := os.Create("trace-data.csv")
	if err != nil {
		b.Fatalf("Failed to create CSV file: %v", err)
	}
	defer csv.Close()
	csv.WriteString("id,position,is_insert,char,avg_time_ms\n")

	start := time.Now()
	document = replay(trace, csv)
	elapsed := time.Since(start)
	fmt.Printf("Applied %d edits in %dms\n", len(trace.Edits), elapsed.Milliseconds())

	// Keep the document referenced until after ReadMemStats (as the original
	// TestTrace did) so the printed figure includes the replayed document.
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Printf("Final memory usage: %.2f MB\n", float64(m.Alloc)/1024.0/1024.0)

	if trace.FinalText != document.GetString() {
		b.Fatalf("Mismatch, got '%q'", document.GetString())
	}
}
