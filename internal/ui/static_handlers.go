package ui

import (
	"io"
	"net/http"
)

func (a *App) serveRobots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, "User-agent: *\nDisallow: /room/\n")
}

func (a *App) serveSecurityTxt(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, "Contact: mailto:jli@cparta.se\nExpires: 2027-05-19T00:00:00Z\nPreferred-Languages: en, sv\n")
}

func (a *App) serveNotFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_, _ = io.WriteString(w, "<!doctype html><title>Not Found</title><h1>Not Found</h1>\n")
}
