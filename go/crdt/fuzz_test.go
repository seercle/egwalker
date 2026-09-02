package crdt

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"
)

const textAlphabet = " abcdefghijklmnopqrstuvwxyz"

func textChar(b byte) byte {
	return textAlphabet[int(b)%len(textAlphabet)]
}

type byteReader struct {
	data []byte
	i    int
}

func (r *byteReader) next() (byte, bool) {
	if r.i >= len(r.data) {
		return 0, false
	}
	v := r.data[r.i]
	r.i++
	return v, true
}

// randBytes deterministically produces pseudo-random fuzz corpus input.
func randBytes(seed int64, n int) []byte {
	r := rand.New(rand.NewSource(seed))
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(r.Intn(256))
	}
	return b
}

func addSeeds(f *testing.F, seeds [][]byte) {
	for _, s := range seeds {
		f.Add(s)
	}
}

type textDelta struct {
	insert bool
	ch     byte
	pos    int
	del    int
}

// nextTextDelta decodes one insert/delete against a document of length L.
func nextTextDelta(L int, r *byteReader) (textDelta, bool) {
	if L == 0 {
		c, ok := r.next()
		if !ok {
			return textDelta{}, false
		}
		return textDelta{insert: true, ch: textChar(c), pos: 0}, true
	}

	op, ok := r.next()
	if !ok {
		return textDelta{}, false
	}
	if op%3 != 0 {
		// Insert a character.
		c, ok := r.next()
		if !ok {
			return textDelta{}, false
		}
		p, ok := r.next()
		if !ok {
			return textDelta{}, false
		}
		return textDelta{insert: true, ch: textChar(c), pos: int(p) % (L + 1)}, true
	}

	// Delete between 1 and 3 characters.
	p, ok := r.next()
	if !ok {
		return textDelta{}, false
	}
	n, ok := r.next()
	if !ok {
		return textDelta{}, false
	}
	pos := int(p) % L
	del := int(n)%3 + 1
	if pos+del > L {
		del = L - pos
	}
	return textDelta{insert: false, pos: pos, del: del}, true
}

func applyTextDelta(doc *RuneDocument, d textDelta) {
	if d.insert {
		doc.Ins(d.pos, string(d.ch))
	} else {
		doc.Del(d.pos, d.del)
	}
}

func textDocSeeds() [][]byte {
	return [][]byte{
		{},
		[]byte("hello world"),
		randBytes(1, 128),
		randBytes(2, 256),
		randBytes(3, 512),
		randBytes(4, 1024),
	}
}

// FuzzDocumentOps checks a single RuneDocument against a []rune reference
// model after every insert/delete.
func FuzzDocumentOps(f *testing.F) {
	addSeeds(f, textDocSeeds())

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) == 0 {
			return
		}
		doc := NewRuneDocument(1)
		var mirror []rune
		r := &byteReader{data: data}

		for {
			L := len(mirror)
			d, ok := nextTextDelta(L, r)
			if !ok {
				break
			}

			if d.insert {
				doc.Ins(d.pos, string(d.ch))
				mirror = append(mirror, 0)
				copy(mirror[d.pos+1:], mirror[d.pos:])
				mirror[d.pos] = rune(d.ch)
			} else {
				doc.Del(d.pos, d.del)
				mirror = append(mirror[:d.pos], mirror[d.pos+d.del:]...)
			}

			if got := doc.GetString(); got != string(mirror) {
				t.Fatalf("document diverged: got %q, mirror %q", got, string(mirror))
			}
		}
	})
}

// FuzzMergeConvergence checks that three replicas of a RuneDocument converge
// after interleaved edits and pairwise merges.
func FuzzMergeConvergence(f *testing.F) {
	addSeeds(f, textDocSeeds())

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) == 0 {
			return
		}
		docs := []*RuneDocument{
			NewRuneDocument(0),
			NewRuneDocument(1),
			NewRuneDocument(2),
		}
		r := &byteReader{data: data}
		target := 0

		for {
			d, ok := nextTextDelta(docs[target].Len(), r)
			if !ok {
				break
			}
			applyTextDelta(docs[target], d)
			target = (target + 1) % len(docs)

			// Occasionally sync two replicas and check convergence.
			m, ok := r.next()
			if !ok || m%3 != 0 {
				continue
			}
			a := int(m) % len(docs)
			b := (a + 1) % len(docs)
			docs[a].MergeFrom(docs[b])
			docs[b].MergeFrom(docs[a])
			if ga, gb := docs[a].GetString(), docs[b].GetString(); ga != gb {
				t.Fatalf("replicas %d and %d diverged: %q vs %q", a, b, ga, gb)
			}
		}

		// Final full sync: all replicas must agree.
		for i := 1; i < len(docs); i++ {
			docs[0].MergeFrom(docs[i])
		}
		for i := 1; i < len(docs); i++ {
			docs[i].MergeFrom(docs[0])
			if docs[i].GetString() != docs[0].GetString() {
				t.Fatalf("replica %d failed to converge: %q vs %q", i, docs[i].GetString(), docs[0].GetString())
			}
			docs[i].Check()
		}
		docs[0].Check()
	})
}

func mapDocEqual(a, b *MapDocument[string, int]) bool {
	ka, kb := a.Keys(), b.Keys()
	if len(ka) != len(kb) {
		return false
	}
	for _, k := range ka {
		va, oka := a.Get(k)
		vb, okb := b.Get(k)
		if !oka || !okb || va != vb {
			return false
		}
	}
	return true
}

// FuzzMapDocument checks convergence of replicated MapDocuments under
// random Set calls and pairwise merges.
func FuzzMapDocument(f *testing.F) {
	addSeeds(f, [][]byte{
		{},
		randBytes(10, 64),
		randBytes(11, 128),
		randBytes(12, 256),
		randBytes(13, 512),
	})

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) == 0 {
			return
		}
		docs := []*MapDocument[string, int]{
			NewMapDocument[string, int](0),
			NewMapDocument[string, int](1),
			NewMapDocument[string, int](2),
		}
		r := &byteReader{data: data}

		for {
			x, ok := r.next()
			if !ok {
				break
			}
			y, ok := r.next()
			if !ok {
				break
			}
			target := int(x) % len(docs)

			if y%3 != 0 {
				// Set a value on one replica.
				k, ok := r.next()
				if !ok {
					break
				}
				v, ok := r.next()
				if !ok {
					break
				}
				docs[target].Set(fmt.Sprintf("k-%d", int(k)%8), int(v))
				continue
			}

			// Sync two replicas and check convergence.
			a := target
			b := (a + 1) % len(docs)
			docs[a].MergeFrom(docs[b])
			docs[b].MergeFrom(docs[a])
			if !mapDocEqual(docs[a], docs[b]) {
				t.Fatalf("map replicas %d and %d diverged", a, b)
			}
		}

		// Final full sync.
		for i := 1; i < len(docs); i++ {
			docs[0].MergeFrom(docs[i])
		}
		for i := 1; i < len(docs); i++ {
			docs[i].MergeFrom(docs[0])
			if !mapDocEqual(docs[0], docs[i]) {
				t.Fatalf("map replica %d failed to converge", i)
			}
		}
	})
}

// FuzzArrayDocument checks convergence of replicated ArrayDocuments under
// random element inserts/deletes and pairwise merges.
func FuzzArrayDocument(f *testing.F) {
	addSeeds(f, [][]byte{
		{},
		randBytes(20, 64),
		randBytes(21, 128),
		randBytes(22, 256),
		randBytes(23, 512),
	})

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) == 0 {
			return
		}
		docs := []*ArrayDocument[int]{
			NewArrayDocument[int](0),
			NewArrayDocument[int](1),
			NewArrayDocument[int](2),
		}
		r := &byteReader{data: data}

		for {
			x, ok := r.next()
			if !ok {
				break
			}
			y, ok := r.next()
			if !ok {
				break
			}
			target := int(x) % len(docs)
			L := docs[target].Len()

			if L == 0 || y%2 == 0 {
				// Insert one element.
				v, ok := r.next()
				if !ok {
					break
				}
				pos := 0
				if L > 0 {
					p, ok := r.next()
					if !ok {
						break
					}
					pos = int(p) % (L + 1)
				}
				docs[target].Ins(pos, []int{int(v)})
			} else {
				// Delete one element.
				p, ok := r.next()
				if !ok {
					break
				}
				docs[target].Del(int(p)%L, 1)
			}

			// Occasionally sync two replicas and check convergence.
			m, ok := r.next()
			if !ok || m%5 != 0 {
				continue
			}
			a := int(m) % len(docs)
			b := (a + 1) % len(docs)
			docs[a].MergeFrom(docs[b])
			docs[b].MergeFrom(docs[a])
			if ia, ib := docs[a].GetItems(), docs[b].GetItems(); !reflect.DeepEqual(ia, ib) {
				t.Fatalf("array replicas %d and %d diverged: %v vs %v", a, b, ia, ib)
			}
		}

		// Final full sync.
		for i := 1; i < len(docs); i++ {
			docs[0].MergeFrom(docs[i])
		}
		for i := 1; i < len(docs); i++ {
			docs[i].MergeFrom(docs[0])
			if !reflect.DeepEqual(docs[0].GetItems(), docs[i].GetItems()) {
				t.Fatalf("array replica %d failed to converge", i)
			}
		}
	})
}
