package ui

import (
	"errors"
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
	// Tiles configures the display map tile source. The zero value uses
	// OpenStreetMap tiles with their attribution.
	Tiles MapTiles
	// Logger receives page render failures with component=ui. Nil means
	// slog.Default(), resolved in New.
	Logger *slog.Logger
}

// MapTiles configures the raster tile source of display maps. The browser loads
// tiles directly, so a host Content-Security-Policy must allow the tile origin in
// img-src.
type MapTiles struct {
	// URL is a Leaflet tile URL template: an absolute path or http(s) URL
	// containing {z}, {x}, and {y}, for example
	// "https://tiles.example/{z}/{x}/{y}.png".
	URL string
	// Attribution is the plain-text credit shown on the map. It is required with
	// URL and is never interpreted as HTML.
	Attribution string
	// AttributionURL optionally links the credit. It follows the link rules of
	// Config.
	AttributionURL string
}

// defaultTiles is used when Config.Tiles is the zero value.
var defaultTiles = MapTiles{
	URL:            "https://tile.openstreetmap.org/{z}/{x}/{y}.png",
	Attribution:    "© OpenStreetMap contributors",
	AttributionURL: "https://www.openstreetmap.org/copyright",
}

// ComponentConfig configures one rendered component.
type ComponentConfig struct {
	// ID is the component root element ID, unique within the host page: an ASCII
	// letter followed by at most 63 ASCII letters, digits, "-", or "_". Element IDs
	// inside the component start with ID followed by "-".
	ID string
}

// componentID matches valid ComponentConfig.ID values.
var componentID = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

// validate checks every configured base path, link, and the tile source.
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
	return checkTiles(config.Tiles)
}

// checkTiles accepts the zero value or a complete tile source.
func checkTiles(tiles MapTiles) error {
	if tiles == (MapTiles{}) {
		return nil
	}
	if tiles.URL == "" || tiles.Attribution == "" {
		return errors.New("Tiles.URL and Tiles.Attribution are required together")
	}
	if err := checkLink(tiles.URL); err != nil {
		return fmt.Errorf("Tiles.URL: %w", err)
	}
	for _, placeholder := range []string{"{z}", "{x}", "{y}"} {
		if !strings.Contains(tiles.URL, placeholder) {
			return fmt.Errorf("Tiles.URL %q has no %s placeholder", tiles.URL, placeholder)
		}
	}
	if tiles.AttributionURL != "" {
		if err := checkLink(tiles.AttributionURL); err != nil {
			return fmt.Errorf("Tiles.AttributionURL: %w", err)
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
