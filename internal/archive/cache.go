package archive

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The Racket indexer writes cache.rkt as a list of #hasheq documents. Keep
// that format by default so -c can reuse caches from either implementation.
func readIndexCache(path string) ([]Document, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(contents)) == 0 {
		return nil, fmt.Errorf("empty cache")
	}
	if bytes.HasPrefix(bytes.TrimSpace(contents), []byte("(")) {
		return readRacketCache(string(contents))
	}
	var docs []Document
	if err := json.Unmarshal(contents, &docs); err != nil {
		return nil, err
	}
	return docs, nil
}

func writeIndexCache(path string, docs []Document) error {
	if strings.HasSuffix(path, ".json") {
		body, err := json.Marshal(docs)
		if err != nil {
			return err
		}
		return os.WriteFile(path, body, 0644)
	}
	var b strings.Builder
	b.WriteByte('(')
	for i, doc := range docs {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString("#hasheq((id . ")
		writeRacketString(&b, doc.ID)
		b.WriteString(") (title . ")
		writeRacketString(&b, doc.Title)
		b.WriteString(") (body . ")
		writeRacketString(&b, doc.Body)
		b.WriteString(") (table . ")
		writeRacketString(&b, doc.Table)
		fmt.Fprintf(&b, ") (len . %d))", doc.Len)
	}
	b.WriteByte(')')
	return os.WriteFile(path, []byte(b.String()), 0644)
}

func writeRacketString(b *strings.Builder, s string) {
	b.WriteByte('"')
	for _, ch := range s {
		switch ch {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if ch < ' ' || ch == 127 {
				fmt.Fprintf(b, `\x%X;`, ch)
			} else {
				b.WriteRune(ch)
			}
		}
	}
	b.WriteByte('"')
}

type racketCacheReader struct {
	source string
	offset int
}

func (r *racketCacheReader) whitespace() {
	for r.offset < len(r.source) {
		switch r.source[r.offset] {
		case ' ', '\t', '\r', '\n':
			r.offset++
		default:
			return
		}
	}
}
func (r *racketCacheReader) consume(s string) bool {
	r.whitespace()
	if strings.HasPrefix(r.source[r.offset:], s) {
		r.offset += len(s)
		return true
	}
	return false
}
func (r *racketCacheReader) require(s string) error {
	if r.consume(s) {
		return nil
	}
	return fmt.Errorf("expected %q near cache byte %d", s, r.offset)
}
func (r *racketCacheReader) atom() string {
	r.whitespace()
	start := r.offset
	for r.offset < len(r.source) && !strings.ContainsRune("() \r\n\t", rune(r.source[r.offset])) {
		r.offset++
	}
	return r.source[start:r.offset]
}
func (r *racketCacheReader) quoted() (string, error) {
	if err := r.require(`"`); err != nil {
		return "", err
	}
	var b strings.Builder
	for r.offset < len(r.source) {
		ch := r.source[r.offset]
		r.offset++
		if ch == '"' {
			return b.String(), nil
		}
		if ch != '\\' {
			b.WriteByte(ch)
			continue
		}
		if r.offset == len(r.source) {
			break
		}
		escape := r.source[r.offset]
		r.offset++
		switch escape {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'a':
			b.WriteByte('\a')
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'v':
			b.WriteByte('\v')
		case '\\', '"', '\'':
			b.WriteByte(escape)
		case 'x', 'u', 'U':
			start := r.offset
			if escape == 'x' {
				for r.offset < len(r.source) && isHex(r.source[r.offset]) {
					r.offset++
				}
				if r.offset < len(r.source) && r.source[r.offset] == ';' {
					r.offset++
				}
			} else {
				n := 4
				if escape == 'U' {
					n = 8
				}
				if r.offset+n > len(r.source) {
					return "", fmt.Errorf("invalid unicode cache escape")
				}
				r.offset += n
			}
			digits := strings.TrimSuffix(r.source[start:r.offset], ";")
			u, err := strconv.ParseUint(digits, 16, 32)
			if err != nil || !utf8.ValidRune(rune(u)) {
				return "", fmt.Errorf("invalid cache string escape %q", digits)
			}
			b.WriteRune(rune(u))
		default:
			return "", fmt.Errorf("unknown cache escape %q", escape)
		}
	}
	return "", fmt.Errorf("unterminated cache string")
}
func isHex(b byte) bool { return b >= '0' && b <= '9' || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F' }

func readRacketCache(source string) ([]Document, error) {
	r := &racketCacheReader{source: source}
	if err := r.require("("); err != nil {
		return nil, err
	}
	var docs []Document
	for !r.consume(")") {
		if err := r.require("#hasheq("); err != nil {
			return nil, err
		}
		var doc Document
		for !r.consume(")") {
			if err := r.require("("); err != nil {
				return nil, err
			}
			field := r.atom()
			if err := r.require("."); err != nil {
				return nil, err
			}
			switch field {
			case "id":
				var err error
				doc.ID, err = r.quoted()
				if err != nil {
					return nil, err
				}
			case "title":
				var err error
				doc.Title, err = r.quoted()
				if err != nil {
					return nil, err
				}
			case "body":
				var err error
				doc.Body, err = r.quoted()
				if err != nil {
					return nil, err
				}
			case "table":
				var err error
				doc.Table, err = r.quoted()
				if err != nil {
					return nil, err
				}
			case "len":
				var err error
				doc.Len, err = strconv.Atoi(r.atom())
				if err != nil {
					return nil, err
				}
			default:
				return nil, fmt.Errorf("unknown index cache field %q", field)
			}
			if err := r.require(")"); err != nil {
				return nil, err
			}
		}
		docs = append(docs, doc)
	}
	r.whitespace()
	if r.offset != len(r.source) {
		return nil, fmt.Errorf("unexpected cache trailing data")
	}
	return docs, nil
}
