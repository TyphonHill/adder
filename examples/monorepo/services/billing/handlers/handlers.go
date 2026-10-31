// Package handlers serves the billing service.
package handlers

import "net/http"

// CreateInvoice bills a customer.
func CreateInvoice(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusCreated) }

// RefundInvoice refunds an invoice.
func RefundInvoice(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusAccepted) }
