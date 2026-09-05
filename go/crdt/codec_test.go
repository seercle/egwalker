package crdt

import "testing"

func TestRuneTextCodecRoundTrip(t *testing.T) {
	cases := []runeText{
		"",
		"hello",
		"héllo wörld — ünïcode ✓",
		"\xff\xfe raw invalid bytes", // preserved as raw bytes in memory
		"line1\nline2\ttabbed\r\n",
	}
	c := ContentCodec[runeText](RuneTextCodec{})
	for _, in := range cases {
		blob, err := c.Encode(in)
		if err != nil {
			t.Fatalf("Encode(%q): %v", in, err)
		}
		got, err := c.Decode(blob)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if got != in {
			t.Errorf("round trip = %q, want %q", got, in)
		}
	}
}
