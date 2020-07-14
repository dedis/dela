package controllers

import (
	"net/http"
	"text/template"
)

// renderHTTPError is a utility function to render a user-friendly error
func (c Ctrl) renderHTTPError(w http.ResponseWriter, message string, code int) {
	var viewData = struct {
		Title   string
		Message string
		Code    int
	}{
		"An error happened",
		message,
		code,
	}

	t, err := template.ParseFiles(c.abs("gui/views/layout.gohtml"),
		c.abs("gui/views/error.gohtml"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = t.ExecuteTemplate(w, "layout", viewData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	if code > 0 {
		w.WriteHeader(code)
	}
}
