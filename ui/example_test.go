package ui_test

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/miroslav-matejovsky/ais-testbench/ui"
)

// hostPage is an application template. The component is trusted HTML produced
// by RenderManager; resources come from the same UI object.
var hostPage = template.Must(template.New("host").Parse(`<!doctype html>
<title>Operations</title>
{{range .Resources.Stylesheets}}<link rel="stylesheet" href="{{.URL}}">{{end}}
<nav><a href="/">Operations home</a></nav>
<main>{{.Manager}}</main>
{{range .Resources.Scripts}}<script defer src="{{.URL}}"></script>{{end}}
`))

// Example renders the manager component into a host page mounted below
// /tools/ais. The simulator API (see simulator.Simulator.API) would be mounted at
// /tools/ais/api/ on the same mux.
func Example() {
	u, err := ui.New(ui.Config{
		ManagerAPIBase: "/tools/ais/api/",
		AssetsBase:     "/tools/ais/assets/",
	})
	if err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/tools/ais/assets/", http.StripPrefix("/tools/ais/assets", u.Assets()))
	mux.HandleFunc("GET /operations", func(w http.ResponseWriter, r *http.Request) {
		// Render into buffers so a failure can still change the response status.
		var component, page bytes.Buffer
		if err := u.RenderManager(&component, ui.ComponentConfig{ID: "fleet-manager"}); err != nil {
			http.Error(w, "render failed", http.StatusInternalServerError)
			return
		}
		data := struct {
			Resources ui.Resources
			Manager   template.HTML
		}{u.ManagerResources(), template.HTML(component.String())} // #nosec G203 -- RenderManager escapes its output.
		if err := hostPage.Execute(&page, data); err != nil {
			http.Error(w, "render failed", http.StatusInternalServerError)
			return
		}
		if _, err := page.WriteTo(w); err != nil {
			return // The client went away.
		}
	})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/operations", nil))
	body := rec.Body.String()
	fmt.Println(rec.Code)
	fmt.Println(strings.Contains(body, `<div id="fleet-manager" class="ais-manager" data-ais-manager data-api-base="/tools/ais/api/">`))
	fmt.Println(strings.Contains(body, `<script defer src="/tools/ais/assets/js/manager.js"></script>`))
	// Output:
	// 200
	// true
	// true
}
