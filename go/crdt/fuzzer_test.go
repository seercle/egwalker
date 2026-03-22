package crdt

import (
	"fmt"
	"log"
	"math/rand"
	"reflect"
	"testing"
)

func TestTest(t *testing.T) {
	doc := NewRuneDocument(0)
	doc.Ins(0, "a")
	doc.Ins(1, "b")
	doc.Ins(2, "c")
	println(doc.GetString())
}

func TestFuzzerMerge(t *testing.T) {
	// Initialize deterministic random source
	seedCount := 100
	for seed := range seedCount {
		fmt.Printf("Fuzzing with merge on seed %d/%d\n", seed, seedCount-1)
		src := rand.NewSource(int64(seed))
		r := rand.New(src)

		// Helper functions
		randInt := func(n int) int {
			if n == 0 {
				return 0
			}
			return r.Intn(n)
		}

		randBool := func(weight float64) bool {
			return r.Float64() < weight
		}

		alphabet := []rune(" abcdefghijklmnopqrstuvwxyz")
		randChar := func() rune {
			return alphabet[randInt(len(alphabet))]
		}

		// Initialize documents
		docs := []*RuneDocument{
			NewRuneDocument(0),
			NewRuneDocument(1),
			NewRuneDocument(2),
		}

		randDoc := func() *RuneDocument {
			return docs[randInt(3)]
		}

		for i := range 100 {
			for range 3 {
				// 1. Pick a random document
				// 2. Make a random change to that document
				doc := randDoc()

				length := doc.Len()

				insertWeight := 0.35
				if length < 100 {
					insertWeight = 0.65
				}

				if length == 0 || randBool(insertWeight) {
					// Insert
					content := randChar()
					pos := randInt(length + 1)
					doc.Ins(pos, string(content))
				} else {
					// Delete
					pos := randInt(length)

					// Calculate max delete length: Math.min(len - pos, 3)
					remaining := length - pos
					maxDel := min(remaining, 3)

					delLen := randInt(maxDel)
					doc.Del(pos, delLen)
				}
			}

			// pick 2 documents and merge them
			a := randDoc()
			b := randDoc()

			if a == b {
				continue
			}

			a.MergeFrom(b)
			b.MergeFrom(a)

			// Assert equality
			if a.GetString() != b.GetString() {
				log.Fatalf("Assertion Failed at seed %d, iteration %d: Documents are not equal", seed, i)
			}
		}
	}
}

func TestFuzzerSlice(t *testing.T) {
	// Initialize deterministic random source
	seedCount := 100
	for seed := range seedCount {
		fmt.Printf("Fuzzing with slice on seed %d/%d\n", seed, seedCount-1)
		src := rand.NewSource(int64(seed))
		r := rand.New(src)

		// Helper functions
		randInt := func(n int) int {
			if n == 0 {
				return 0
			}
			return r.Intn(n)
		}

		randBool := func(weight float64) bool {
			return r.Float64() < weight
		}

		alphabet := []rune(" abcdefghijklmnopqrstuvwxyz")
		randChar := func() rune {
			return alphabet[randInt(len(alphabet))]
		}

		document := NewRuneDocument(0)
		slice := []rune{}

		for i := range 10000 {
			// Accessing the snapshot length.
			length := len(slice)
			insertWeight := 0.35
			if length < 100 {
				insertWeight = 0.65
			}
			if length == 0 || randBool(insertWeight) {
				// Insert
				content := randChar()
				pos := randInt(length + 1)
				document.Ins(pos, string(content))
				slice = append(slice[:pos], append([]rune{content}, slice[pos:]...)...)
			} else {
				// Delete
				pos := randInt(length)
				// Calculate max delete length: Math.min(len - pos, 3)
				remaining := length - pos
				maxDel := min(remaining, 3)
				delLen := randInt(maxDel)
				document.Del(pos, delLen)
				slice = append(slice[:pos], slice[pos+delLen:]...)
			}

			// Assert equality
			if document.GetString() != string(slice) {
				log.Fatalf("Assertion Failed at seed %d, iteration %d: Documents are not equal", seed, i)
			}
		}
	}
}

func TestFuzzerMap(t *testing.T) {
	seedCount := 50
	for seed := range seedCount {
		fmt.Printf("Fuzzing MapDocument on seed %d/%d\n", seed, seedCount-1)
		r := rand.New(rand.NewSource(int64(seed)))

		docs := []*MapDocument[string, int]{
			NewMapDocument[string, int](0),
			NewMapDocument[string, int](1),
			NewMapDocument[string, int](2),
		}

		for range 200 {
			// Random edit
			d := docs[r.Intn(len(docs))]
			key := fmt.Sprintf("key-%d", r.Intn(10))
			d.Set(key, r.Intn(1000))

			// Random merge
			if r.Float64() < 0.2 {
				a := docs[r.Intn(len(docs))]
				b := docs[r.Intn(len(docs))]
				if a != b {
					a.MergeFrom(b)
					b.MergeFrom(a)
				}
			}
		}

		// Final sync
		for i := 1; i < len(docs); i++ {
			docs[0].MergeFrom(docs[i])
		}
		for i := 1; i < len(docs); i++ {
			docs[i].MergeFrom(docs[0])
		}

		// Verify consistency
		for i := 1; i < len(docs); i++ {
			keys0 := docs[0].Keys()
			keysI := docs[i].Keys()
			if len(keys0) != len(keysI) {
				log.Fatalf("Seed %d: Key count mismatch", seed)
			}
			for _, k := range keys0 {
				v0, _ := docs[0].Get(k)
				vI, _ := docs[i].Get(k)
				if v0 != vI {
					log.Fatalf("Seed %d: Value mismatch for key %s", seed, k)
				}
			}
		}
	}
}

func TestFuzzerArray(t *testing.T) {
	seedCount := 50
	for seed := range seedCount {
		fmt.Printf("Fuzzing ArrayDocument on seed %d/%d\n", seed, seedCount-1)
		r := rand.New(rand.NewSource(int64(seed)))

		docs := []*ArrayDocument[int]{
			NewArrayDocument[int](0),
			NewArrayDocument[int](1),
			NewArrayDocument[int](2),
		}

		for range 200 {
			d := docs[r.Intn(len(docs))]
			length := d.Len()

			if length == 0 || r.Float64() < 0.6 {
				// Insert
				pos := r.Intn(length + 1)
				d.Ins(pos, []int{r.Intn(100)})
			} else {
				// Delete
				pos := r.Intn(length)
				d.Del(pos, 1)
			}

			if r.Float64() < 0.2 {
				a := docs[r.Intn(len(docs))]
				b := docs[r.Intn(len(docs))]
				if a != b {
					a.MergeFrom(b)
					b.MergeFrom(a)
				}
			}
		}

		// Final sync
		for i := 1; i < len(docs); i++ {
			docs[0].MergeFrom(docs[i])
		}
		for i := 1; i < len(docs); i++ {
			docs[i].MergeFrom(docs[0])
		}

		// Verify consistency
		items0 := docs[0].GetItems()
		for i := 1; i < len(docs); i++ {
			itemsI := docs[i].GetItems()
			if !reflect.DeepEqual(items0, itemsI) {
				log.Fatalf("Seed %d: Array mismatch", seed)
			}
		}
	}
}
