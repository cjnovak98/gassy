package a2a

import (
	"encoding/json"
	"os"
	"time"
)

// AgentCardJSON represents the JSON structure at /.well-known/agent.json
type AgentCardJSON struct {
	Name            string                `json:"name"`
	Version         string                `json:"version"`
	Url             string                `json:"url"`
	Capabilities    AgentCapabilitiesJSON `json:"capabilities"`
	Skills          []AgentSkill          `json:"skills,omitempty"`
	Provider        *AgentProvider        `json:"provider,omitempty"`
	SecuritySchemes map[string]any        `json:"securitySchemes,omitempty"`
	DefaultStream   bool                  `json:"defaultStream"`
}

// AgentCapabilitiesJSON represents capabilities in JSON (bool pointers for optional fields)
type AgentCapabilitiesJSON struct {
	Streaming         bool `json:"streaming"`
	PushNotifications bool `json:"pushNotifications"`
	ExtendedAgentCard bool `json:"extendedAgentCard"`
}

// ToAgentCard converts JSON representation to AgentCard
func (j *AgentCardJSON) ToAgentCard() *AgentCard {
	card := &AgentCard{
		Name:    j.Name,
		Version: j.Version,
		Url:     j.Url,
		Capabilities: AgentCapabilities{
			Streaming:         j.Capabilities.Streaming,
			PushNotifications: j.Capabilities.PushNotifications,
			ExtendedAgentCard: j.Capabilities.ExtendedAgentCard,
		},
		Skills:        j.Skills,
		Provider:      j.Provider,
		DefaultStream: j.DefaultStream,
	}
	// Convert map[string]any back to map[string]SecurityScheme
	if j.SecuritySchemes != nil {
		card.SecuritySchemes = make(map[string]SecurityScheme)
		for k, v := range j.SecuritySchemes {
			if m, ok := v.(map[string]any); ok {
				scheme := SecurityScheme{}
				if t, ok := m["type"].(string); ok {
					scheme.Type = t
				}
				if s, ok := m["scheme"].(string); ok {
					scheme.Scheme = s
				}
				if bf, ok := m["bearerFormat"].(string); ok {
					scheme.BearerFormat = bf
				}
				card.SecuritySchemes[k] = scheme
			}
		}
	}
	return card
}

// ToJSON converts AgentCard to its JSON representation
func (a *AgentCard) ToJSON() *AgentCardJSON {
	return &AgentCardJSON{
		Name:    a.Name,
		Version: a.Version,
		Url:     a.Url,
		Capabilities: AgentCapabilitiesJSON{
			Streaming:         a.Capabilities.Streaming,
			PushNotifications: a.Capabilities.PushNotifications,
			ExtendedAgentCard: a.Capabilities.ExtendedAgentCard,
		},
		Skills:        a.Skills,
		Provider:      a.Provider,
		DefaultStream: a.DefaultStream,
	}
}

// SaveAgentCard saves an AgentCard to a JSON file
func SaveAgentCard(card *AgentCard, path string) error {
	j := card.ToJSON()
	data, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// LoadAgentCard loads an AgentCard from a JSON file
func LoadAgentCard(path string) (*AgentCard, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var j AgentCardJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return nil, err
	}
	return j.ToAgentCard(), nil
}

// NewMessage creates a new message with a text part
func NewMessage(role, text string) *Message {
	return &Message{
		Role:      role,
		Parts:     []Part{TextPart{Type: "text", Text: text}},
		Timestamp: time.Now(),
	}
}

// NewTask creates a new task in working state
func NewTask(id string, msg *Message) *Task {
	return &Task{
		ID:      id,
		State:   TaskStateWorking,
		Message: msg,
		Status: &TaskStatus{
			State:     TaskStateWorking,
			Timestamp: time.Now(),
		},
	}
}
