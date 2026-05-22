package zon

import (
	"io"
	"reflect"
)

// An Encoder writes ZON values to an output stream.
type Encoder struct {
	w      io.Writer
	prefix string
	indent string
}

// NewEncoder returns a new encoder that writes to w. By default it pretty-prints
// with four-space indentation; call SetIndent to change this.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w, indent: "    "}
}

// SetIndent sets the prefix and per-level indent used by Encode. An empty
// indent produces compact, single-line output.
func (enc *Encoder) SetIndent(prefix, indent string) {
	enc.prefix = prefix
	enc.indent = indent
}

// Encode writes the ZON encoding of v to the stream, followed by a newline.
func (enc *Encoder) Encode(v any) error {
	e := &encodeState{prefix: enc.prefix, indent: enc.indent}
	if err := e.encode(reflect.ValueOf(v)); err != nil {
		return err
	}
	e.buf.WriteByte('\n')
	_, err := enc.w.Write(e.buf.Bytes())
	return err
}

// A Decoder reads and decodes a ZON value from an input stream.
//
// A ZON document holds a single value, so the Decoder reads the entire stream
// on the first call to Decode.
type Decoder struct {
	r    io.Reader
	data []byte
	read bool
}

// NewDecoder returns a new decoder that reads from r.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: r}
}

// Decode reads the ZON value from the stream and stores it in the value pointed
// to by v.
func (dec *Decoder) Decode(v any) error {
	if !dec.read {
		data, err := io.ReadAll(dec.r)
		if err != nil {
			return err
		}
		dec.data = data
		dec.read = true
	}
	return Unmarshal(dec.data, v)
}
