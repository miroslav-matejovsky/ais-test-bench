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
// by RenderManager; the stylesheet and module URLs come from the same UI object.
// The host's own module script, /static/operations.js, mounts the component
// without inline scripts:
//
//	const { mountManager } = await import(document.querySelector("[data-ais-module]").dataset.aisModule);
//	const manager = mountManager(document.getElementById("fleet-manager"), {
//	    fetch: (url, init) => fetch(url, { ...init, headers: { ...init.headers, "X-CSRF-Token": token } }),
//	});
//	// On removal: manager.destroy();
var hostPage = template.Must(template.New("host").Parse(`<!doctype html>
<title>Operations</title>
<link rel="stylesheet" href="{{.Stylesheet}}">
<script type="module" src="/static/operations.js"></script>
<nav><a href="/">Operations home</a></nav>
<main data-ais-module="{{.Module}}">{{.Manager}}</main>
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
			Stylesheet, Module string
			Manager            template.HTML
		}{u.StylesheetURL(), u.ModuleURL(), template.HTML(component.String())} // #nosec G203 -- RenderManager escapes its output.
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
	fmt.Println(strings.Contains(body, `<main data-ais-module="/tools/ais/assets/js/ui.js">`))
	// Output:
	// 200
	// true
	// true
}
