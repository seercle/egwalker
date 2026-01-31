package main

import (
	"fmt"
	"log"
	"math/rand"
	"reflect"
	"testing"
)

func TestTest(t *testing.T) {
	doc := NewCRDTDocument(0)
	doc.Ins(0, "a")
	doc.Ins(1, "b")
	doc.Ins(2, "c")
	println(doc.GetString())
}

func TestFuzzerMerge(t *testing.T) {
	// Initialize deterministic random source
	seed_count := 100
	for seed := range seed_count {
		fmt.Printf("Fuzzing with merge on seed %d/%d\n", seed, seed_count-1)
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
		docs := []*CRDTDocument{
			NewCRDTDocument(0),
			NewCRDTDocument(1),
			NewCRDTDocument(2),
		}

		randDoc := func() *CRDTDocument {
			return docs[randInt(3)]
		}

		for i := range 100 {
			// console.log('ii', i)
			for range 3 {
				// 1. Pick a random document
				// 2. Make a random change to that document
				doc := randDoc()

				// Accessing the snapshot length.
				length := len(doc.Branch.Snapshot)
				//length := doc.Branch.Snapshot.Size()

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

				// doc.Check()
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
	seed_count := 100
	for seed := range seed_count {
		fmt.Printf("Fuzzing with slice on seed %d/%d\n", seed, seed_count-1)
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

		document := NewCRDTDocument(0)
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

			// Assert deep equality
			if !reflect.DeepEqual(document.Branch.Snapshot, slice) {
				log.Fatalf("Assertion Failed at seed %d, iteration %d: Documents are not equal", seed, i)
			}
			//j := 0
			//document.Branch.Snapshot.ForEach(func(r rune) {
			//	if r != slice[j] {
			//		log.Fatalf("Assertion Failed at seed %d, iteration %d: Documents are not equal", seed, i)
			//	}
			//	j++
			//})
		}
	}
}
