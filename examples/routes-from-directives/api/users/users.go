// Package users serves the users of the example API.
package users

import (
	"fmt"
	"net/http"
)

// ListUsers lists every user.
//
//adder:route method=GET path=/users
func ListUsers(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "alice\nbob")
}

// GetUser returns one user by ID.
//
//adder:route method=GET path=/users/{id}
func GetUser(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, r.PathValue("id"))
}

// CreateUser adds a user.
//
//adder:route method=POST path=/users
func CreateUser(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusCreated)
}

// format is a helper, not a handler: without a directive, the scan leaves it out.
func format(name string) string { return name }
