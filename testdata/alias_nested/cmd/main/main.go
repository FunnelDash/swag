package main

import "github.com/FunnelDash/swag/v2/testdata/alias_nested/pkg/good"

// @Success 200 {object} good.Gen
// @Router /api [get].
func main() {
	var _ good.Gen
}
