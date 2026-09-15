package ui

import (
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"

	"github.com/miroslav-matejovsky/ais-testbench/internal/urlpath"
)

// Config configures the public URLs and logger of one UI instance. Every URL is
// the address a browser uses, including an external prefix added by a reverse
// proxy. The UI never derives URLs from requests or forwarded headers.
//
// Base paths are absolute and end with "/", for example "/tools/ais/api/". They
// contain only letters, digits, "-._~" and separators, without empty or dot
// segments. Links are absolute paths or absolute http(s) URLs without user info.
type Config struct {
	// ManagerAPIBase is the public simulator API base path. Empty disables the
	// manager component and page.
	ManagerAPIBase string
	// DisplayAPIBase is the public display API base path. Empty disables the
	// display component and page.
	DisplayAPIBase string
	// AssetsBase is the required public base path where Assets is mounted.
	AssetsBase string
	// HomeURL, ManagerURL, and DisplayURL are optional page links shown in the
	// standalone page header and on the home page. The display component also
	// links ManagerURL when no station is configured. Empty omits a link.
	HomeURL    string
	ManagerURL string
	DisplayURL string
	// StatusURL is an optional status page link. When set, the standalone manager
	// page offers an htmx status check that loads its fragment.
	StatusURL string
	// Logger receives page render failures with component=ui. Nil means
	// slog.Default(), resolved in New.
	Logger *slog.Logger
}

// ComponentConfig configures one rendered component.
type ComponentConfig struct {
	// ID is the component root element ID, unique within the host page: an ASCII
	// letter followed by at most 63 ASCII letters, digits, "-", or "_".
	ID string
}

// Resource is one stylesheet or script a host page loads.
type Resource struct {
	URL string
	// Integrity is the subresource integrity value of a third-party resource,
	// loaded with crossorigin="". It is empty for first-party assets.
	Integrity string
}

// Resources lists what a host page loads once, in order, for a component:
// stylesheets in the head and classic scripts with defer or after the markup.
type Resources struct {
	Stylesheets []Resource
	Scripts     []Resource
}

// componentID matches valid ComponentConfig.ID values.
var componentID = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

// validate checks every configured base path and link.
func validate(config Config) error {
	if err := urlpath.CheckBase(config.AssetsBase); err != nil {
		return fmt.Errorf("AssetsBase: %w", err)
	}
	for _, base := range []struct{ name, value string }{
		{"ManagerAPIBase", config.ManagerAPIBase},
		{"DisplayAPIBase", config.DisplayAPIBase},
	} {
		if base.value == "" {
			continue
		}
		if err := urlpath.CheckBase(base.value); err != nil {
			return fmt.Errorf("%s: %w", base.name, err)
		}
	}
	for _, link := range []struct{ name, value string }{
		{"HomeURL", config.HomeURL},
		{"ManagerURL", config.ManagerURL},
		{"DisplayURL", config.DisplayURL},
		{"StatusURL", config.StatusURL},
	} {
		if link.value == "" {
			continue
		}
		if err := checkLink(link.value); err != nil {
			return fmt.Errorf("%s: %w", link.name, err)
		}
	}
	return nil
}

// checkLink accepts an absolute path, excluding a scheme-relative "//" URL, or
// an absolute http(s) URL with a host and without user info. Other schemes, such
// as javascript: and data:, are rejected.
func checkLink(link string) error {
	for _, r := range link {
		if r < 0x20 || r == 0x7f || r == '\\' {
			return fmt.Errorf("link %q contains a control character or backslash", link)
		}
	}
	u, err := url.Parse(link)
	if err != nil {
		return fmt.Errorf("parse link: %w", err)
	}
	switch {
	case u.Scheme == "" && strings.HasPrefix(link, "/") && !strings.HasPrefix(link, "//"):
		return nil
	case (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil:
		return nil
	}
	return fmt.Errorf("link %q must be an absolute path or an http(s) URL without user info", link)
}
