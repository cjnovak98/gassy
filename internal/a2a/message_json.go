package a2a

import (
	"encoding/json"
	"time"
)

// messageJSON is the intermediate struct used for JSON unmarshaling of Message.
// It holds parts as raw bytes so we can decode each part by its "type" field.
type messageJSON struct {
	ID        string            `json:"id,omitempty"`
	Role      string            `json:"role,omitempty"`
	Parts     []json.RawMessage `json:"parts"`
	Timestamp time.Time         `json:"timestamp,omitempty"`
}

// UnmarshalJSON decodes a Message, dispatching each element of Parts to the
// correct concrete type (TextPart, DataPart) based on the "type" field.
// Without this, JSON round-trips produce []interface{} of map[string]interface{}
// which breaks type assertions like part.(TextPart).
func (m *Message) UnmarshalJSON(data []byte) error {
	var raw messageJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	m.ID = raw.ID
	m.Role = raw.Role
	m.Timestamp = raw.Timestamp
	m.Parts = make([]Part, 0, len(raw.Parts))

	for _, rawPart := range raw.Parts {
		var peek struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(rawPart, &peek); err != nil {
			// Unrecognised structure — store as generic map
			var generic map[string]interface{}
			_ = json.Unmarshal(rawPart, &generic)
			m.Parts = append(m.Parts, generic)
			continue
		}

		switch peek.Type {
		case "text":
			var tp TextPart
			if err := json.Unmarshal(rawPart, &tp); err == nil {
				m.Parts = append(m.Parts, tp)
			}
		case "data":
			var dp DataPart
			if err := json.Unmarshal(rawPart, &dp); err == nil {
				m.Parts = append(m.Parts, dp)
			}
		default:
			var generic map[string]interface{}
			_ = json.Unmarshal(rawPart, &generic)
			m.Parts = append(m.Parts, generic)
		}
	}

	return nil
}
