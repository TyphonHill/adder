// Package order describes orders and where they are in their lifecycle.
package order

// Status is where an order is in its lifecycle.
type Status int

const (
	StatusPending Status = iota
	StatusPaid
	StatusShipped
	StatusCancelled
)

// Priority is how soon an order ships.
type Priority int

const (
	PriorityLow Priority = iota
	PriorityNormal
	PriorityHigh
)
