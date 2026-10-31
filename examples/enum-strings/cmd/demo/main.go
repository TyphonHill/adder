// Command demo prints a status and a priority by name.
package main

import (
	"fmt"

	"example.com/enum-strings/order"
)

func main() {
	fmt.Println(order.StatusShipped, order.PriorityHigh)
}
