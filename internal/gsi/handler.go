package gsi
import (
    "encoding/json"
    "log"
    "net/http"
)
type GameStatePayload struct {
    Provider interface{} `json:"provider"`
    Player   interface{} `json:"player"`
    Round    interface{} `json:"round"`
}
func Handler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        w.WriteHeader(http.StatusMethodNotAllowed)
        return
    }
    var payload GameStatePayload
    if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
        log.Println("Error decoding GSI data:", err)
        return
    }
    log.Println("Received Game State Data via HTTP")
    w.WriteHeader(http.StatusOK)
}
