package urlpath_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-testbench/internal/urlpath"
)

func TestCheckBase(t *testing.T) {
	for base, valid := range map[string]bool{
		"/":                       true,
		"/api/":                   true,
		"/tools/ais/display/api/": true,
		"/a-b_c.d~e/":             true,
		"":                        false,
		"api/":                    false,
		"/api":                    false,
		"//":                      false,
		"/tools//api/":            false,
		"/./":                     false,
		"/tools/../api/":          false,
		"/tools%2Fapi/":           false,
		"/tools\\api/":            false,
		"/api/?x=1/":              false,
		"/api/#top/":              false,
		"/a b/":                   false,
		"/café/":                  false,
	} {
		t.Run(base, func(t *testing.T) {
			err := urlpath.CheckBase(base)
			if valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
