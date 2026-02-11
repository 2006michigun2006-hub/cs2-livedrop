package gsi

import "strings"

type DerivedEvent struct {
	Type    string
	Payload map[string]interface{}
}

func deriveEvents(payload map[string]interface{}) []DerivedEvent {
	events := make([]DerivedEvent, 0)

	if player, ok := payload["player"].(map[string]interface{}); ok {
		if state, ok := player["state"].(map[string]interface{}); ok {
			if asInt64(state["round_kills"]) > 0 {
				events = append(events, DerivedEvent{Type: "kill", Payload: map[string]interface{}{"round_kills": asInt64(state["round_kills"])}})
			}
			if asInt64(state["round_killhs"]) > 0 {
				events = append(events, DerivedEvent{Type: "headshot", Payload: map[string]interface{}{"round_killhs": asInt64(state["round_killhs"])}})
			}
			if asInt64(state["round_kills"]) >= 5 {
				events = append(events, DerivedEvent{Type: "ace", Payload: map[string]interface{}{"round_kills": asInt64(state["round_kills"])}})
			}
			if asInt64(state["health"]) == 0 {
				events = append(events, DerivedEvent{Type: "death", Payload: map[string]interface{}{"health": 0}})
			}
		}
	}

	if round, ok := payload["round"].(map[string]interface{}); ok {
		phase := strings.ToLower(asString(round["phase"]))
		bomb := strings.ToLower(asString(round["bomb"]))
		if phase == "over" {
			events = append(events, DerivedEvent{Type: "round_win", Payload: map[string]interface{}{"phase": phase}})
		}
		if bomb == "planted" {
			events = append(events, DerivedEvent{Type: "bomb_plant", Payload: map[string]interface{}{"bomb": bomb}})
		}
	}

	if len(events) == 0 {
		events = append(events, DerivedEvent{Type: "game_state", Payload: map[string]interface{}{}})
	}

	return events
}

func asInt64(value interface{}) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case int64:
		return v
	default:
		return 0
	}
}

func asString(value interface{}) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}
