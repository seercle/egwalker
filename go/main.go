package main

import "fmt"

func main() {
	doc1 := NewCRDTDocument(1)
	doc1.Ins(0, "hi")

	doc2 := NewCRDTDocument(2)
	doc2.Ins(0, "yo")

	fmt.Println("Doc1:", doc1.GetString())
	fmt.Println("Doc2:", doc2.GetString())

	doc1.MergeFrom(doc2)
	doc2.MergeFrom(doc1)

	fmt.Println("After Merge 1:")
	fmt.Println("Doc1:", doc1.GetString())
	fmt.Println("Doc2:", doc2.GetString())

	doc2.Ins(4, "x")
	fmt.Println("Doc2 insert 'x' at 4:", doc2.GetString())

	doc1.MergeFrom(doc2)
	fmt.Println("Doc1 after merge:", doc1.GetString())

	doc1.Check()
	fmt.Println("Check passed")
}
