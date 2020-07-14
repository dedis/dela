package controllers

import (
	"net/http"
	"text/template"
)

// HomeHandler handles the home page
func (c Ctrl) HomeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			c.homeGET(w, r)
		default:
			c.renderHTTPError(w, "only GET request allowed", http.StatusBadRequest)
		}
	}
}

func (c Ctrl) homeGET(w http.ResponseWriter, r *http.Request) {

	if r.URL.Path != "/" {
		c.renderHTTPError(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	t, err := template.ParseFiles(c.abs("gui/views/layout.gohtml"),
		c.abs("gui/views/home.gohtml"))
	if err != nil {
		c.renderHTTPError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	viewData := struct {
		Title string
	}{
		Title: "Welcome",
	}

	err = t.ExecuteTemplate(w, "layout", viewData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
