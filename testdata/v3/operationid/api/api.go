package api

import "net/http"

// @Router /bare [get]
// @Success 200 {string} string "ok"
func ListWidgets(w http.ResponseWriter, r *http.Request) {}

// @ID custom-explicit-id
// @Router /explicit [get]
// @Success 200 {string} string "ok"
func GetWidget(w http.ResponseWriter, r *http.Request) {}
