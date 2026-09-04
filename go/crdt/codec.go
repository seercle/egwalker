package crdt

// ContentCodec converts content values to and from opaque bytes for the
// binary frame. The frame treats encoded content as an opaque blob: it never
// inspects, interprets, or post-processes codec output. Each concrete content
// type owns its private byte format; a codec may version its own payload
// freely, since the frame's version header covers only frame structure.
type ContentCodec[C content[C]] interface {
	Encode(C) ([]byte, error)
	Decode([]byte) (C, error)
}

// RuneTextCodec is the codec for RuneDocument's runeText content: the raw
// UTF-8 bytes. runeText is a string, so no other representation exists;
// invalid UTF-8 bytes pass through unchanged, matching in-memory semantics.
type RuneTextCodec struct{}

func (RuneTextCodec) Encode(t runeText) ([]byte, error) { return []byte(t), nil }
func (RuneTextCodec) Decode(b []byte) (runeText, error) { return runeText(b), nil }
