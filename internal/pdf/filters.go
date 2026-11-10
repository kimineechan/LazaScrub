package pdf

import (
	"bytes"
	"compress/zlib"
	"encoding/ascii85"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

const maxDecoded = 256 << 20

// decode undoes the filters of a stream. Only the filters that structural
// and metadata streams use in practice are supported.
func decode(dict *Dict, raw []byte) ([]byte, error) {
	filters := list(dict.Get("Filter"))
	params := list(dict.Get("DecodeParms"))
	if params == nil {
		params = list(dict.Get("DP"))
	}
	data := raw
	for i, f := range filters {
		name, _ := f.(Name)
		var parm *Dict
		if i < len(params) {
			parm, _ = params[i].(*Dict)
		}
		var err error
		switch name {
		case "FlateDecode", "Fl":
			if data, err = inflate(data); err == nil {
				data, err = unpredict(data, parm)
			}
		case "ASCIIHexDecode", "AHx":
			data, err = unhexStream(data)
		case "ASCII85Decode", "A85":
			data, err = un85(data)
		default:
			err = fmt.Errorf("the stream uses the %s filter, which is not supported", name)
		}
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

// list returns a filter entry as an array, whether the file wrote one value or several.
func list(o Object) Array {
	switch v := o.(type) {
	case nil:
		return nil
	case Array:
		return v
	}
	return Array{o}
}

func inflate(b []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	out, err := io.ReadAll(io.LimitReader(r, maxDecoded))
	if len(out) == 0 && err != nil {
		return nil, err
	}
	// A stream that is cut short still yields its leading bytes.
	return out, nil
}

func unhexStream(b []byte) ([]byte, error) {
	var digits []byte
	for _, c := range b {
		if c == '>' {
			break
		}
		if isHex(c) {
			digits = append(digits, c)
		}
	}
	if len(digits)%2 == 1 {
		digits = append(digits, '0')
	}
	out := make([]byte, len(digits)/2)
	_, err := hex.Decode(out, digits)
	return out, err
}

func un85(b []byte) ([]byte, error) {
	if i := bytes.Index(b, []byte("~>")); i >= 0 {
		b = b[:i]
	}
	b = bytes.TrimPrefix(bytes.TrimSpace(b), []byte("<~"))
	out := make([]byte, len(b))
	n, _, err := ascii85.Decode(out, b, true)
	return out[:n], err
}

// unpredict reverses the row predictors that cross-reference and object
// streams are usually compressed with.
func unpredict(data []byte, parm *Dict) ([]byte, error) {
	predictor, _ := asInt(parm.Get("Predictor"))
	if predictor <= 1 {
		return data, nil
	}
	columns, colors, bits := 1, 1, 8
	if v, ok := asInt(parm.Get("Columns")); ok {
		columns = v
	}
	if v, ok := asInt(parm.Get("Colors")); ok {
		colors = v
	}
	if v, ok := asInt(parm.Get("BitsPerComponent")); ok {
		bits = v
	}
	if columns <= 0 || colors <= 0 || bits <= 0 || columns > 1<<20 || colors > 32 || bits > 16 {
		return nil, errors.New("the stream has invalid predictor settings")
	}
	pixel := max(1, colors*bits/8)
	row := (colors*bits*columns + 7) / 8
	if predictor == 2 {
		for start := 0; start+row <= len(data); start += row {
			for i := start + pixel; i < start+row; i++ {
				data[i] += data[i-pixel]
			}
		}
		return data, nil
	}
	out := make([]byte, 0, len(data))
	prev := make([]byte, row)
	for start := 0; start+row+1 <= len(data); start += row + 1 {
		kind, line := data[start], data[start+1:start+1+row]
		for i := range line {
			var left, up, corner byte
			if i >= pixel {
				left, corner = line[i-pixel], prev[i-pixel]
			}
			up = prev[i]
			switch kind {
			case 1:
				line[i] += left
			case 2:
				line[i] += up
			case 3:
				line[i] += byte((int(left) + int(up)) / 2)
			case 4:
				line[i] += paeth(left, up, corner)
			}
		}
		out = append(out, line...)
		prev = line
	}
	return out, nil
}

func paeth(a, b, c byte) byte {
	p := int(a) + int(b) - int(c)
	pa, pb, pc := abs(p-int(a)), abs(p-int(b)), abs(p-int(c))
	switch {
	case pa <= pb && pa <= pc:
		return a
	case pb <= pc:
		return b
	}
	return c
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
