package urlpath

import (
	"fmt"
	"strings"
)

// CheckBase returns an error unless base is a canonical absolute base path.
func CheckBase(base string) error {
	if base == "" || base[0] != '/' || base[len(base)-1] != '/' {
		return fmt.Errorf("base path %q must start and end with /", base)
	}
	for _, r := range base {
		if r != '/' && !unreserved(r) {
			return fmt.Errorf("base path %q contains %q; use only letters, digits, and -._~", base, r)
		}
	}
	if base == "/" {
		return nil
	}
	for segment := range strings.SplitSeq(base[1:len(base)-1], "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("base path %q has an empty, . or .. segment", base)
		}
	}
	return nil
}

// unreserved reports whether r is an RFC 3986 unreserved ASCII character.
func unreserved(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("-._~", r)
}
