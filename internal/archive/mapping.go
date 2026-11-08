package archive

import (
	"crypto/sha1"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// TitleBasename uses the filename encoding of the original Racket archiver.
// A '#' denotes a wiki subpage separator, whereas a literal '#' is %23.
func TitleBasename(title string) string {
	parts := strings.Split(strings.ReplaceAll(title, " ", "_"), "/")
	for i, part := range parts {
		parts[i] = encodeFilename(part)
	}
	return strings.Join(parts, "#")
}

func encodeFilename(s string) string {
	var b strings.Builder
	for _, ch := range []byte(s) {
		if ch <= 31 || ch > 126 || strings.ContainsRune("<>:\"/\\|?*#~&", rune(ch)) {
			fmt.Fprintf(&b, "%%%02X", ch)
		} else {
			b.WriteByte(ch)
		}
	}
	return b.String()
}

// BasenameTitle reverses filename encoding, retaining underscores as the
// API's standard title spelling.
func BasenameTitle(basename string) (string, error) {
	return url.PathUnescape(strings.ReplaceAll(basename, "#", "/"))
}

// LocalURLBasename handles an already URL-encoded local /wiki/... path.
func LocalURLBasename(local string) (string, error) {
	u, err := url.Parse(local)
	if err != nil {
		return "", err
	}
	path, err := url.PathUnescape(u.EscapedPath())
	if err != nil {
		return "", err
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 2 || parts[0] != "wiki" {
		return "", fmt.Errorf("not a wiki URL: %q", local)
	}
	// Racket's URI path parser retains a semicolon and its parameters.
	for i, part := range parts[1:] {
		parts[i+1] = encodeFilename(part)
	}
	return strings.Join(parts[1:], "#"), nil
}

// PageKey is a SHA-1 filename only for basenames longer than 240 bytes.
// The mapping to the full basename remains in the special_page table.
func PageKey(basename string) string {
	if len(basename) > 240 {
		return sha1Hex(basename)
	}
	return basename
}

func sha1Hex(s string) string { return fmt.Sprintf("%x", sha1.Sum([]byte(s))) }

func PagePath(root, wiki, basename, suffix string) string {
	return filepath.Join(root, "archive", wiki, PageKey(basename)+suffix)
}

// ImageKey strips the meaningless numeric cb cache-busting parameter before
// hashing the URL, as the original archiver does.
func ImageKey(raw string) (cleanURL, hash string) {
	// Keep every other byte as supplied; re-encoding changes the persisted hash.
	s := raw
	for offset := 0; offset < len(s); {
		idx := strings.Index(s[offset:], "cb=")
		if idx < 0 {
			break
		}
		idx += offset
		if idx == 0 || (s[idx-1] != '?' && s[idx-1] != '&') {
			offset = idx + 3
			continue
		}
		end := idx + 3
		for end < len(s) && s[end] >= '0' && s[end] <= '9' {
			end++
		}
		if end == idx+3 {
			offset = end
			continue
		}
		sep := s[idx-1 : idx]
		start := idx - 1
		if end < len(s) && s[end] == '&' {
			// Removing a middle parameter leaves its original separator.
			s = s[:start] + sep + s[end+1:]
		} else {
			s = s[:start] + s[end:]
		}
		offset = start
	}
	s = strings.TrimRight(s, "?&")
	return s, sha1Hex(s)
}

func ImageDir(root, wiki, hash string) string {
	return filepath.Join(root, "archive", wiki, "images", hash[:1], hash[:2])
}
