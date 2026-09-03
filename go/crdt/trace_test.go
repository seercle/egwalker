package crdt

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
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

func TestTrace(t *testing.T) {
	jsonFile, err := os.Open("../../resources/editing-trace.json")
	if err != nil {
		t.Fatalf("Failed to open JSON file: %v", err)
	}
	defer jsonFile.Close()

	var trace Trace
	decoder := json.NewDecoder(jsonFile)
	err = decoder.Decode(&trace)
	if err != nil {
		t.Fatalf("Failed to decode JSON file: %v", err)
	}

	csv, err := os.Create("trace-data.csv")
	if err != nil {
		t.Fatalf("Failed to create CSV file: %v", err)
	}
	defer csv.Close()
	csv.WriteString("id,position,is_insert,char,avg_time_ms\n")

	document := NewRuneDocument(0)
	timeSum := time.Duration(0)
	plotEvery := 500

	start := time.Now()
	for i, edit := range trace.Edits {
		opStart := time.Now()
		if edit.IsInsert {
			document.Ins(edit.Position, edit.Char)
		} else {
			document.Del(edit.Position, 1)
		}
		timeSum += time.Since(opStart)
		if i%plotEvery == 0 {
			avgTime := float64(timeSum.Nanoseconds()) / float64(plotEvery)
			fmt.Fprintf(csv, "%d,%d,%t,%q,%.5f\n", i, edit.Position, edit.IsInsert, edit.Char, avgTime)
			timeSum = 0
		}
	}
	elapsed := time.Since(start)

	fmt.Printf("Applied %d edits in %dms\n", len(trace.Edits), elapsed.Milliseconds())

	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Printf("Final memory usage: %.2f MB\n", float64(m.Alloc)/1024.0/1024.0)

	if trace.FinalText != document.GetString() {
		t.Fatalf("Mismatch, got '%q'", document.GetString())
	}
}

func BenchmarkTrace(b *testing.B) {
	jsonFile, err := os.Open("../../resources/editing-trace.json")
	if err != nil {
		b.Fatalf("Failed to open JSON file: %v", err)
	}
	defer jsonFile.Close()

	var trace Trace
	decoder := json.NewDecoder(jsonFile)
	err = decoder.Decode(&trace)
	if err != nil {
		b.Fatalf("Failed to decode JSON file: %v", err)
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

	if trace.FinalText != document.GetString() {
		b.Fatalf("Mismatch, got '%q'", document.GetString())
	}
}
