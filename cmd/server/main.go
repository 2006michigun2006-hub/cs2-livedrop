package main

import (
    "log"
    "net/http"
    "[github.com/2006michigun2006-hub/cs2-livedrop/internal/auth](https://github.com/2006michigun2006-hub/cs2-livedrop/internal/auth)"
    "[github.com/2006michigun2006-hub/cs2-livedrop/internal/gsi](https://github.com/2006michigun2006-hub/cs2-livedrop/internal/gsi)"
)
func main() {
    log.Println("Starting CS2 LiveDrop Server...")
    authService := auth.NewService()
    _ = authService 
    http.HandleFunc("/api/gsi", gsi.Handler)
    http.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("Login Endpoint"))
    })
    log.Println("Server running on port 8080")
    if err := http.ListenAndServe(":8080", nil); err != nil {
        log.Fatal(err)
    }
}
