// Package urlpath validates the public base paths shared by the UI and remote
// display sources, so both follow one canonical rule.
//
// A base path is absolute and ends with "/": "/", "/api/", or
// "/tools/ais/display/api/". Endpoint paths are appended without a leading slash,
// so a base never loses its prefix. Segments contain only RFC 3986 unreserved
// ASCII characters (letters, digits, "-._~"). Empty, "." and ".." segments are
// rejected. Percent signs, backslashes, queries, and fragments cannot appear, which
// excludes encoded separators and any spelling that decodes to another path.
//
// A path prefix is a base path without its trailing "/", for example
// "/tools/ais", or empty for the root. Standalone servers and the testbench
// facade append their routes, such as "/manager" and "/api/", to it.
package urlpath
