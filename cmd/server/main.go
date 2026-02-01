package main
import (
	"log"
	"net/http"
	"github.com/2006michigun2006-hub/cs2-livedrop/internal/auth"
	"github.com/2006michigun2006-hub/cs2-livedrop/internal/gsi"
)
func main() {
	log.Println("Starting CS2 LiveDrop Server...")
	_ = auth.NewService()
	go gsi.BackgroundWorker()
	http.HandleFunc("/api/gsi", gsi.Handler)
	http.HandleFunc("/api/awards", gsi.GetAwardsHandler)
	http.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Server is active. Monitoring CS2 events..."))
	})
	log.Println("Server is running on port 8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
