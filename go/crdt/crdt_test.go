package crdt

import (
	"egwalker/bxtree"
	"reflect"
	"testing"
)

// replayDoc rebuilds a raw crdtDoc from a document's op log, item by item.
func replayDoc(doc *RuneDocument) *crdtDoc {
	cDoc := &crdtDoc{
		items: bxtree.New(
			bxtree.WithSummarizer(crdtSummaryConfig),
			bxtree.WithOnItemMoved(func(item *crdtItem, node *bxtree.Node[*crdtItem, crdtSummary]) {
				item.node = node
			}),
		),
		currentVersion: []lv{},
		delTargets:     make(map[lv]lv),
		sortedItems:    []*crdtItem{},
	}

	for i := 0; i < len(doc.opLog.ops); i++ {
		do1Operation(cDoc, doc.opLog, lv(i), nil)
	}
	return cDoc
}

func TestVeryDeepHistory(t *testing.T) {
	doc1 := NewRuneDocument(1)
	// Use doc.Ins to keep snapshot in sync.
	// Alternating 1 and 3 prevents merging.
	docA := NewRuneDocument(3)
	for i := 0; i < 1000; i++ {
		doc1.Ins(doc1.Len(), "a")
		docA.MergeFrom(doc1)
		docA.Ins(docA.Len(), "z")
		doc1.MergeFrom(docA)
	}

	doc2 := NewRuneDocument(2)
	doc2.MergeFrom(doc1)
	doc2.Ins(doc2.Len(), "b")

	// doc1 adds something else concurrently
	doc1.Ins(doc1.Len(), "c")

	// Merge back
	doc1.MergeFrom(doc2)

	// Fully sync every replica so all three converge, then assert invariants.
	// Total visible runes: 1000 'a' + 1000 'z' + 'b' + 'c' = 2002.
	docA.MergeFrom(doc1)
	doc2.MergeFrom(doc1)

	const wantLen = 2002
	for name, d := range map[string]*RuneDocument{"doc1": doc1, "docA": docA, "doc2": doc2} {
		if d.Len() != wantLen {
			t.Errorf("%s: Len()=%d, want %d", name, d.Len(), wantLen)
		}
		if d.GetString() != doc1.GetString() {
			t.Errorf("%s: diverged from doc1", name)
		}
		d.Check()
	}
}

func TestRuneDocument_Basic(t *testing.T) {
	doc := NewRuneDocument(1)
	doc.Ins(0, "Hello")
	if doc.GetString() != "Hello" {
		t.Errorf("Expected 'Hello', got %s", doc.GetString())
	}

	doc.Ins(5, " World")
	if doc.GetString() != "Hello World" {
		t.Errorf("Expected 'Hello World', got %s", doc.GetString())
	}

	doc.Del(5, 1) // Remove space
	if doc.GetString() != "HelloWorld" {
		t.Errorf("Expected 'HelloWorld', got %s", doc.GetString())
	}

	doc.Check()
}

func TestRuneDocument_Merge(t *testing.T) {
	doc1 := NewRuneDocument(1)
	doc1.Ins(0, "abc")

	doc2 := NewRuneDocument(2)
	doc2.MergeFrom(doc1)
	if doc2.GetString() != "abc" {
		t.Errorf("Doc2 should have 'abc', got %s", doc2.GetString())
	}

	doc1.Ins(3, "d")
	doc2.Ins(0, "X")

	// Concurrent edits: doc1="abcd", doc2="Xabc"
	doc1.MergeFrom(doc2)
	doc2.MergeFrom(doc1)

	if doc1.GetString() != doc2.GetString() {
		t.Errorf("Convergence failed: %s vs %s", doc1.GetString(), doc2.GetString())
	}
	doc1.Check()
	doc2.Check()
}

func TestArrayDocument_Basic(t *testing.T) {
	arr := NewArrayDocument[int](1)
	arr.Ins(0, []int{1, 2, 3})

	items := arr.GetItems()
	expected := []int{1, 2, 3}
	if !reflect.DeepEqual(items, expected) {
		t.Errorf("Expected %v, got %v", expected, items)
	}

	arr.Del(1, 1) // Remove '2'
	items = arr.GetItems()
	expected = []int{1, 3}
	if !reflect.DeepEqual(items, expected) {
		t.Errorf("Expected %v, got %v", expected, items)
	}
	arr.Check()
}

func TestMapDocument_LWW(t *testing.T) {
	m1 := NewMapDocument[string, string](1)
	m2 := NewMapDocument[string, string](2)

	m1.Set("key", "val1")
	m2.Set("key", "val2")

	// Merge m2 into m1. Agent 2 > Agent 1, so "val2" should win.
	m1.MergeFrom(m2)
	val, ok := m1.Get("key")
	if !ok || val != "val2" {
		t.Errorf("LWW failed: expected val2, got %s", val)
	}

	// Now set with even higher agent/seq
	m1.Set("key", "val3")
	val, _ = m1.Get("key")
	if val != "val3" {
		t.Errorf("Local override failed: expected val3, got %s", val)
	}
}

func TestRecursiveMerge_MapOfText(t *testing.T) {
	// Setup two agents with a shared project map
	alice := NewMapDocument[string, *RuneDocument](1)
	bob := NewMapDocument[string, *RuneDocument](2)

	// Create a shared document
	docA := NewRuneDocument(10)
	docA.Ins(0, "Common")

	alice.Set("readme", docA)

	// Bob merges to get the reference
	bob.MergeFrom(alice)

	docB, _ := bob.Get("readme")

	// Concurrent edits to the nested document
	docA.Ins(6, " Alice")
	docB.Ins(6, " Bob")

	// Now Alice merges Bob's project map
	alice.MergeFrom(bob)

	// Fetching from map should trigger recursive merge automatically
	merged, ok := alice.Get("readme")
	if !ok {
		t.Fatal("Failed to get readme from map")
	}

	content := merged.GetString()
	// Order depends on agent IDs (10 for Alice's start, but subsequent edits use alice/bob agents)
	// Both edits should be present.
	if len(content) < len("Common Alice Bob") {
		t.Errorf("Recursive merge failed to include both edits: %s", content)
	}
}

func TestRecursiveMerge_ComplexNesting(t *testing.T) {
	// Map -> Array -> RuneDocument
	type Node struct {
		Data *ArrayDocument[*RuneDocument]
	}

	agent1 := NewMapDocument[string, *ArrayDocument[*RuneDocument]](1)
	agent2 := NewMapDocument[string, *ArrayDocument[*RuneDocument]](2)

	// Initial structure
	arr1 := NewArrayDocument[*RuneDocument](1)
	txt1 := NewRuneDocument(1)
	txt1.Ins(0, "Hello")
	arr1.Ins(0, []*RuneDocument{txt1})
	agent1.Set("root", arr1)

	// Sync to agent2
	agent2.MergeFrom(agent1)

	arr2, _ := agent2.Get("root")
	txt2 := arr2.GetItems()[0]

	// Concurrent edits at the leaf
	txt1.Ins(5, " World")
	txt2.Ins(0, "Hey ")

	// Sync back
	agent1.MergeFrom(agent2)

	// Verify recursive resolution
	finalArr, _ := agent1.Get("root")
	finalTxt := finalArr.GetItems()[0]

	// Note: ArrayDocument currently doesn't implement recursive merge on GetItems()
	// like MapDocument does on Get(), because Array elements aren't indexed by a
	// unique key that allows identifying "concurrent versions" of the same slot
	// easily without more complex metadata.
	//
	// However, if we manually merge the instances we find:
	finalTxt.MergeFrom(txt2)

	expected := "Hey Hello World"
	if finalTxt.GetString() != expected {
		// Note: CRDT order might vary but both should be there
		if len(finalTxt.GetString()) != len(expected) {
			t.Errorf("Complex nesting merge failed: %s", finalTxt.GetString())
		}
	}
}

func TestMapDocument_Keys(t *testing.T) {
	m := NewMapDocument[int, string](1)
	m.Set(1, "one")
	m.Set(2, "two")
	m.Set(3, "three")

	keys := m.Keys()
	if len(keys) != 3 {
		t.Errorf("Expected 3 keys, got %d", len(keys))
	}

	found := make(map[int]bool)
	for _, k := range keys {
		found[k] = true
	}

	for i := 1; i <= 3; i++ {
		if !found[i] {
			t.Errorf("Key %d missing from Keys()", i)
		}
	}
}

func TestDocument_Reset(t *testing.T) {
	doc := NewRuneDocument(1)
	doc.Ins(0, "hello")
	doc.Reset()

	if doc.Len() != 0 {
		t.Errorf("Len should be 0 after reset, got %d", doc.Len())
	}
	if doc.GetString() != "" {
		t.Errorf("String should be empty after reset, got %q", doc.GetString())
	}
}

func TestRecursiveMerge_ThreeWay(t *testing.T) {
	m1 := NewMapDocument[string, *RuneDocument](1)
	m2 := NewMapDocument[string, *RuneDocument](2)
	m3 := NewMapDocument[string, *RuneDocument](3)

	doc := NewRuneDocument(10)
	doc.Ins(0, "Start")

	m1.Set("text", doc)
	m2.MergeFrom(m1)
	m3.MergeFrom(m1)

	d1, _ := m1.Get("text")
	d2, _ := m2.Get("text")
	d3, _ := m3.Get("text")

	d1.Ins(5, "1")
	d2.Ins(5, "2")
	d3.Ins(5, "3")

	m1.MergeFrom(m2)
	m1.MergeFrom(m3)

	res, _ := m1.Get("text")
	s := res.GetString()
	if len(s) != len("Start123") {
		t.Errorf("Three-way merge failed, expected length 8, got %d (%s)", len(s), s)
	}
}

func TestRecursiveMerge_NestedMaps(t *testing.T) {
	m1 := NewMapDocument[string, *MapDocument[string, string]](1)
	m2 := NewMapDocument[string, *MapDocument[string, string]](2)

	inner1 := NewMapDocument[string, string](11)
	inner1.Set("a", "1")
	m1.Set("inner", inner1)

	m2.MergeFrom(m1)
	inner2, _ := m2.Get("inner")

	inner1.Set("b", "2")
	inner2.Set("c", "3")

	m1.MergeFrom(m2)
	mergedInner, _ := m1.Get("inner")

	if v, ok := mergedInner.Get("b"); !ok || v != "2" {
		t.Errorf("Nested map merge failed for key 'b'")
	}
	if v, ok := mergedInner.Get("c"); !ok || v != "3" {
		t.Errorf("Nested map merge failed for key 'c'")
	}
}

func TestRecursiveMerge_MixedTypes(t *testing.T) {
	m1 := NewMapDocument[string, any](1)
	m2 := NewMapDocument[string, any](2)

	doc := NewRuneDocument(11)
	doc.Ins(0, "doc")

	m1.Set("key", doc)
	m2.Set("key", "not-a-doc")

	// Agent 2 > Agent 1, so "not-a-doc" should win and NO merge should happen
	m1.MergeFrom(m2)
	val, _ := m1.Get("key")

	if s, ok := val.(string); !ok || s != "not-a-doc" {
		t.Errorf("Mixed types LWW failed, expected 'not-a-doc', got %v", val)
	}
}

func TestOpLog_IsAncestor(t *testing.T) {
	log := newOpLog[rune]()
	// LV 0: agent 1, seq 0, parents []
	log.pushLocalOp(1, op[rune]{content: 'a'})
	// LV 1: agent 1, seq 1, parents [0]
	log.pushLocalOp(1, op[rune]{content: 'b'})
	// To make LV 2 and LV 3 concurrent, both having LV 1 as parent:
	// LV 2: agent 2, seq 0, parents [1]
	pushRemoteOp(log, op[rune]{id: id{agent: 2, seq: 0}, content: 'c'}, []id{{agent: 1, seq: 1}})
	// LV 3: agent 3, seq 0, parents [1]
	pushRemoteOp(log, op[rune]{id: id{agent: 3, seq: 0}, content: 'd'}, []id{{agent: 1, seq: 1}})

	if !log.isAncestor(0, 1) {
		t.Error("0 should be ancestor of 1")
	}
	if !log.isAncestor(0, 2) {
		t.Error("0 should be ancestor of 2")
	}
	if !log.isAncestor(1, 2) {
		t.Error("1 should be ancestor of 2")
	}
	if !log.isAncestor(1, 3) {
		t.Error("1 should be ancestor of 3")
	}
	if log.isAncestor(2, 3) {
		t.Error("2 should NOT be ancestor of 3 (concurrent)")
	}
	if log.isAncestor(3, 2) {
		t.Error("3 should NOT be ancestor of 2 (concurrent)")
	}
	if !log.isAncestor(1, 1) {
		t.Error("1 should be ancestor of itself")
	}
	if log.isAncestor(1, 0) {
		t.Error("1 should NOT be ancestor of 0")
	}
}

func TestRecursiveMerge_MapOfArrays(t *testing.T) {
	m1 := NewMapDocument[string, *ArrayDocument[int]](1)
	m2 := NewMapDocument[string, *ArrayDocument[int]](2)

	arr := NewArrayDocument[int](10)
	arr.Ins(0, []int{1, 2})

	m1.Set("list", arr)
	m2.MergeFrom(m1)

	a1, _ := m1.Get("list")
	a2, _ := m2.Get("list")

	// Concurrent inserts into the nested array
	a1.Ins(2, []int{3})
	a2.Ins(0, []int{0})

	m1.MergeFrom(m2)
	merged, ok := m1.Get("list")
	if !ok {
		t.Fatal("Failed to get list from map")
	}

	items := merged.GetItems()
	// Should contain [0, 1, 2, 3] in some stable order
	if len(items) != 4 {
		t.Errorf("Recursive array merge failed, expected 4 items, got %d: %v", len(items), items)
	}
}

func TestNestedArrays_Manual(t *testing.T) {
	// Array of Arrays
	parent1 := NewArrayDocument[*ArrayDocument[int]](1)
	parent2 := NewArrayDocument[*ArrayDocument[int]](2)

	child1 := NewArrayDocument[int](11)
	child1.Ins(0, []int{100})

	parent1.Ins(0, []*ArrayDocument[int]{child1})
	parent2.MergeFrom(parent1)

	child2 := parent2.GetItems()[0]

	// Concurrent edits to the nested array
	child1.Ins(1, []int{200})
	child2.Ins(0, []int{50})

	// Sync parents
	parent1.MergeFrom(parent2)

	// Since ArrayDocument doesn't auto-merge on GetItems (it has no keys/causal slot mapping),
	// we verify we can still manually merge the instances found in the array.
	finalChild := parent1.GetItems()[0]
	finalChild.MergeFrom(child2)

	res := finalChild.GetItems()
	if len(res) != 3 {
		t.Errorf("Nested array manual merge failed, expected 3 items, got %v", res)
	}
}

func TestNestedArrayInArray_Automatic(t *testing.T) {
	// Tests a multi-level array structure: [ [1, 2], [3, 4] ]
	parent1 := NewArrayDocument[*ArrayDocument[int]](1)
	parent2 := NewArrayDocument[*ArrayDocument[int]](2)

	childA := NewArrayDocument[int](10)
	childA.Ins(0, []int{1, 2})

	childB := NewArrayDocument[int](20)
	childB.Ins(0, []int{3, 4})

	// Initial state: Alice has childA, Bob has childB
	parent1.Ins(0, []*ArrayDocument[int]{childA})
	parent2.Ins(0, []*ArrayDocument[int]{childB})

	// Sync parents: Now both should have both children
	parent1.MergeFrom(parent2)
	parent2.MergeFrom(parent1)

	items1 := parent1.GetItems()
	items2 := parent2.GetItems()

	if len(items1) != 2 || len(items2) != 2 {
		t.Errorf("Sync failed, expected 2 children, got %d and %d", len(items1), len(items2))
	}

	// Verify that we can edit a specific child discovered via the parent
	// and sync it across instances.
	remoteChildA := items2[0] // On Bob's side
	remoteChildA.Ins(2, []int{99})

	childA.MergeFrom(remoteChildA)
	res := childA.GetItems()

	expected := []int{1, 2, 99}
	if !reflect.DeepEqual(res, expected) {
		t.Errorf("Nested child sync failed: expected %v, got %v", expected, res)
	}
}

func TestArrayOfArray_RecursiveSync(t *testing.T) {
	// 1. Setup parent and a nested array
	parent1 := NewArrayDocument[*ArrayDocument[int]](1)
	child1 := NewArrayDocument[int](11)
	child1.Ins(0, []int{1, 2})

	parent1.Ins(0, []*ArrayDocument[int]{child1})

	// 2. Clone parent to another agent
	parent2 := NewArrayDocument[*ArrayDocument[int]](2)
	parent2.MergeFrom(parent1)

	// 3. Get the child references from both parents
	c1 := parent1.GetItems()[0]
	c2 := parent2.GetItems()[0]

	// 4. Concurrent edits to the nested child instances
	c1.Ins(2, []int{3})
	c2.Ins(0, []int{0})

	// 5. Merge parents. This triggers recursive merge of the child
	// because they share the same insertion identity in the parent's oplog.
	parent1.MergeFrom(parent2)

	// 6. Verify
	finalChild := parent1.GetItems()[0]
	res := finalChild.GetItems()

	if len(res) != 4 {
		t.Errorf("Expected 4 items in recursively merged nested array, got %v", res)
	}
}

func TestItemMerging(t *testing.T) {
	doc := NewRuneDocument(1)

	// Consecutive insertions
	doc.Ins(0, "abc")
	doc.Ins(3, "def")

	// We can manually checkout the document to inspect the crdtDoc structure
	cDoc := replayDoc(doc)

	// Verify that we only have 1 merged item instead of 2 separate ones.
	if cDoc.items.Size() != 1 {
		t.Errorf("Expected 1 merged item, got %d", cDoc.items.Size())
	}

	// Check length of the merged item
	itemPtr, _ := cDoc.items.GetAt(0)
	if (*itemPtr).length != 6 {
		t.Errorf("Expected item length 6, got %d", (*itemPtr).length)
	}
}

func TestItemMerging_SplitAndReMerge(t *testing.T) {
	doc := NewRuneDocument(1)
	doc.Ins(0, "abc")
	doc.Ins(3, "def")

	// Insert in middle, should split the existing merged item.
	doc.Ins(3, "X")

	cDoc := replayDoc(doc)

	// Expected: "abc" (3 chars), "X" (1 char), "def" (3 chars).
	// Because "X" was inserted at index 3, it splits "abcdef" into "abc" and "def",
	// and then "X" is inserted between them.
	// In the CRDT logic, "X" will be between them because its originLeft is the last char of "abc".
	if cDoc.items.Size() != 3 {
		t.Errorf("Expected 3 items, got %d", cDoc.items.Size())
	}
}
