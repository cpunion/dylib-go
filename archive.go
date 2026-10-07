package dylib

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// parseArchive handles GNU/SysV, COFF and BSD extended names. Index members are
// skipped: symbols are indexed from the objects, so stale ranlib tables do not
// determine which code is loaded. Member bytes remain owned by this file.
func parseArchive(name string, b []byte) (*file, error) {
	f := &file{info: Info{Name: name, Format: "ar", Kind: "archive"}}
	var names []byte
	for pos := 8; pos < len(b); {
		if len(b)-pos < 60 || string(b[pos+58:pos+60]) != "`\n" {
			return nil, fmt.Errorf("bad archive header at %d", pos)
		}
		h := b[pos : pos+60]
		n, err := strconv.ParseUint(strings.TrimSpace(string(h[48:58])), 10, 64)
		if err != nil || n > uint64(len(b)-pos-60) {
			return nil, fmt.Errorf("invalid archive member size at %d", pos)
		}
		member := b[pos+60 : pos+60+int(n)]
		pos += 60 + int(n)
		if pos&1 != 0 {
			if pos >= len(b) {
				return nil, fmt.Errorf("missing archive padding")
			}
			pos++
		}
		mn := strings.TrimSpace(string(h[:16]))
		switch {
		case mn == "//":
			names = member
			continue
		case mn == "/" || mn == "/SYM64/" || strings.HasPrefix(mn, "__.SYMDEF"):
			continue
		case strings.HasPrefix(mn, "#1/"):
			l, e := strconv.Atoi(mn[3:])
			if e != nil || l < 0 || l > len(member) {
				return nil, fmt.Errorf("invalid BSD archive name")
			}
			mn = strings.TrimRight(string(member[:l]), "\x00")
			member = member[l:]
			if strings.HasPrefix(mn, "__.SYMDEF") {
				continue
			}
		case strings.HasPrefix(mn, "/"):
			o, e := strconv.Atoi(mn[1:])
			if e != nil || o < 0 || o >= len(names) {
				return nil, fmt.Errorf("invalid archive name offset")
			}
			// GNU long names end with /\n; COFF long names end with NUL.
			// Both use the // member and /decimal-offset references.
			end := bytes.IndexAny(names[o:], "\n\x00")
			if end < 0 {
				return nil, fmt.Errorf("unterminated archive name")
			}
			mn = string(names[o : o+end])
			if names[o+end] == '\n' {
				mn = strings.TrimSuffix(mn, "/")
			}
		default:
			mn = strings.TrimSuffix(mn, "/")
		}
		if mn == "" {
			return nil, fmt.Errorf("empty archive member name")
		}
		if bytes.HasPrefix(member, []byte("!<arch>\n")) || bytes.HasPrefix(member, []byte("!<thin>\n")) {
			return nil, fmt.Errorf("nested archives are unsupported")
		}
		child, err := parse(name+"("+mn+")", member)
		if err != nil {
			return nil, err
		}
		if child.info.Kind != "object" {
			return nil, fmt.Errorf("%s: archive members must be relocatable objects", child.info.Name)
		}
		f.members = append(f.members, child)
		f.info.Members = append(f.info.Members, child.info)
	}
	return f, nil
}
