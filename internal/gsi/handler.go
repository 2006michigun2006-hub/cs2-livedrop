package gsi

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
)

// Award(Database)
type Award struct {
	PlayerName string `json:"player_name"`
	Event      string `json:"event"`
	Prize      string `json:"prize"`
}

var (
	Awards      []Award
	AwardsMutex sync.Mutex
)

type GameStatePayload struct {
	Player struct {
		SteamID string `json:"steamid"`
		Name    string `json:"name"`
		State   struct {
			Health int `json:"health"`
		} `json:"state"`
		MatchStats struct {
			Kills int `json:"kills"`
		} `json:"match_stats"`
		Weapons map[string]struct {
			Name  string `json:"name"`
			State string `json:"state"`
		} `json:"weapons"`
	} `json:"player"`
	Round struct {
		Phase string `json:"phase"`
	} `json:"round"`
	Bomb struct {
		State string `json:"state"`
	} `json:"bomb"`
}

var DataChannel = make(chan GameStatePayload, 100)

func Handler(w http.ResponseWriter, r *http.Request) {
	var payload GameStatePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return
	}
	select {
	case DataChannel <- payload:
	default:
	}
	w.WriteHeader(http.StatusOK)
}

// GetAwardsHandler
func GetAwardsHandler(w http.ResponseWriter, r *http.Request) {
	AwardsMutex.Lock()
	defer AwardsMutex.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Awards)
}
func addAward(name, event, prize string) {
	AwardsMutex.Lock()
	Awards = append(Awards, Award{PlayerName: name, Event: event, Prize: prize})
	AwardsMutex.Unlock()
	log.Printf("[AWARD STORED] %s -> %s (%s)", name, prize, event)
}
func getActiveWeapon(data GameStatePayload) string {
	for _, w := range data.Player.Weapons {
		if w.State == "active" {
			return w.Name
		}
	}
	return ""
}
func BackgroundWorker() {
	var lastMatchKills int
	var roundKills int
	var fiveSevenKills int
	var lastSteamID string
	var isDead bool
	var bombAlreadyDefused bool
	log.Println("GSI Background Worker started...")
	for data := range DataChannel {
		if data.Player.SteamID != lastSteamID {
			lastSteamID = data.Player.SteamID
			lastMatchKills = data.Player.MatchStats.Kills
			roundKills = 0
			fiveSevenKills = 0
			isDead = false
			log.Printf("--- Focus on Player: %s ---", data.Player.Name)
			continue
		}
		if data.Round.Phase == "freeze" {
			if roundKills != 0 || isDead || bombAlreadyDefused || lastMatchKills != data.Player.MatchStats.Kills {
				roundKills = 0
				fiveSevenKills = 0
				isDead = false
				bombAlreadyDefused = false
				lastMatchKills = data.Player.MatchStats.Kills
				log.Println("--- NEW ROUND STARTED: Counters synced ---")
			}
		}
		if data.Bomb.State == "defused" && !bombAlreadyDefused {
			addAward(data.Player.Name, "Bomb Defused", "Consumer Grade Item")
			bombAlreadyDefused = true
		}
		if data.Player.State.Health == 0 && !isDead && data.Player.Name != "" {
			log.Printf(" %s is down!", data.Player.Name)
			isDead = true
		}
		if data.Player.MatchStats.Kills > lastMatchKills {
			diff := data.Player.MatchStats.Kills - lastMatchKills
			roundKills += diff
			lastMatchKills = data.Player.MatchStats.Kills
			weapon := getActiveWeapon(data)
			log.Printf("Kill with %s (Round: %d)", weapon, roundKills)
			if strings.Contains(weapon, "knife") || strings.Contains(weapon, "bayonet") {
				addAward(data.Player.Name, "Knife Kill", "AK-47")
			}
			if weapon == "weapon_fiveseven" {
				fiveSevenKills++
			}
			if roundKills == 5 {
				if fiveSevenKills == 5 {
					addAward(data.Player.Name, "Five-Seven ACE", "Butterfly Knife")
				} else {
					addAward(data.Player.Name, "ACE", "Rare Case")
				}
				roundKills = 0
				fiveSevenKills = 0
			}
		}
	}
}
