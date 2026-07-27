// Package main is a fixture: general API info coexisting with a health
// handler whose @Description must not clobber the API description.
//
// @title Account API
// @version 1.0
// @description Manages accounts and their lifecycle.
package main

// HealthCheck godoc
// @Summary Health check
// @Description Health status of the service.
// @Router /health [get]
func HealthCheck() {}

func main() {}
