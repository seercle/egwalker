package main

import (
	"egwalker/crdt"
	"fmt"
)

func main() {
	// --- RuneDocument Example ---
	fmt.Println("--- RuneDocument ---")
	doc1 := crdt.NewRuneDocument(1)
	doc1.Ins(0, "hi")

	doc2 := crdt.NewRuneDocument(2)
	doc2.Ins(0, "yo")

	fmt.Println("Doc1:", doc1.GetString())
	fmt.Println("Doc2:", doc2.GetString())

	doc1.MergeFrom(doc2)
	fmt.Println("After Merge - Doc1:", doc1.GetString())

	doc1.Check()
	fmt.Println("RuneDocument check passed")

	// --- ArrayDocument Example ---
	fmt.Println("\n--- ArrayDocument ---")
	arr1 := crdt.NewArrayDocument[int](1)
	arr1.Ins(0, []int{10, 20})

	arr2 := crdt.NewArrayDocument[int](2)
	arr2.Ins(0, []int{30, 40})

	fmt.Println("Arr1:", arr1.GetItems())
	fmt.Println("Arr2:", arr2.GetItems())

	arr1.MergeFrom(arr2)
	fmt.Println("After Merge - Arr1:", arr1.GetItems())

	arr1.Check()
	fmt.Println("ArrayDocument check passed")

	// --- MapDocument Example ---
	fmt.Println("\n--- MapDocument ---")
	m1 := crdt.NewMapDocument[string, string](1)
	m1.Set("foo", "bar")

	m2 := crdt.NewMapDocument[string, string](2)
	m2.Set("foo", "baz")
	m2.Set("hello", "world")

	fmt.Println("M1 keys:", m1.Keys())
	fmt.Println("M2 keys:", m2.Keys())

	m1.MergeFrom(m2)
	val, _ := m1.Get("foo")
	fmt.Println("M1 'foo' after merge (LWW):", val)

	// --- Complex Nested Sync Example ---
	fmt.Println("\n--- Complex Nested Sync (Recursive) ---")

	// Alice and Bob share a "Project" map
	aliceProject := crdt.NewMapDocument[string, *crdt.RuneDocument](1)
	bobProject := crdt.NewMapDocument[string, *crdt.RuneDocument](2)

	// Alice starts with a Readme
	aliceReadme := crdt.NewRuneDocument(11)
	aliceReadme.Ins(0, "Initial Content")

	// Bob starts with a copy of the same Readme
	bobReadme := crdt.NewRuneDocument(22)
	bobReadme.MergeFrom(aliceReadme)

	aliceProject.Set("readme", aliceReadme)
	bobProject.Set("readme", bobReadme)

	// Alice and Bob edit their own instances concurrently
	aliceReadme.Ins(15, " (Alice's edit)")
	bobReadme.Ins(15, " (Bob's edit)")

	fmt.Println("Alice's Readme before sync:", aliceReadme.GetString())
	fmt.Println("Bob's Readme before sync:", bobReadme.GetString())

	// Sync the Map
	aliceProject.MergeFrom(bobProject)
	bobProject.MergeFrom(aliceProject)

	// With recursive merge implemented, calling Get() on the map will
	// automatically detect concurrent versions of the nested documents
	// and merge them.
	mergedAlice, _ := aliceProject.Get("readme")
	mergedBob, _ := bobProject.Get("readme")

	fmt.Println("Merged Readme (Alice's Map.Get):", mergedAlice.GetString())
	fmt.Println("Merged Readme (Bob's Map.Get):", mergedBob.GetString())

	// --- Nested ArrayDocument (Matrix) ---
	fmt.Println("\n--- Nested ArrayDocument (Matrix) ---")

	// Create a 3x2 matrix using nested ArrayDocuments
	matrix := crdt.NewArrayDocument[*crdt.ArrayDocument[int]](1)

	for i := 0; i < 3; i++ {
		row := crdt.NewArrayDocument[int](i + 10)
		row.Ins(0, []int{i * 10, i*10 + 1})
		matrix.Ins(i, []*crdt.ArrayDocument[int]{row})
	}

	fmt.Println("Matrix state:")
	for i, row := range matrix.GetItems() {
		fmt.Printf("Row %d: %v\n", i, row.GetItems())
	}
}
