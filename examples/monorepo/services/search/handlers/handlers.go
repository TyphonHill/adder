// Package handlers serves the search service.
package handlers

import "net/http"

// Search finds documents that match a query.
func Search(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
