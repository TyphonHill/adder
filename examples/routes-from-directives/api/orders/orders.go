// Package orders serves the orders of the example API.
package orders

import (
	"fmt"
	"net/http"
)

// ListOrders lists every order.
//
//adder:route method=GET path=/orders
func ListOrders(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "1001\n1002")
}

// CancelOrder cancels an order that has not shipped.
//
//adder:route method=DELETE path=/orders/{id}
func CancelOrder(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}
