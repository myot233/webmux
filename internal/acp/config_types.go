package acp

import (
	"encoding/json"
	"fmt"
)

// Config option types.
const (
	ConfigSelect  = "select"
	ConfigBoolean = "boolean"
)

// ConfigValue is a config option value: a boolean when Bool is non-nil,
// otherwise the select value ID.
type ConfigValue struct {
	Bool *bool
	ID   string
}

// ConfigOption is one session configuration option (schema
// SessionConfigOption). Grouped select options are flattened in order, each
// carrying its group.
type ConfigOption struct {
	ID          string
	Name        string
	Description string
	Category    string
	Type        string
	Current     ConfigValue
	Options     []ConfigSelectOption
}

// ConfigSelectOption is one select choice.
type ConfigSelectOption struct {
	Value       string
	Name        string
	Description string
	Group       string
	GroupName   string
}

type configSelectOptionJSON struct {
	Value       string `json:"value"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type configGroupJSON struct {
	Group   string                   `json:"group"`
	Name    string                   `json:"name"`
	Options []configSelectOptionJSON `json:"options"`
}

type configOptionJSON struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	Category     string          `json:"category,omitempty"`
	Type         string          `json:"type"`
	CurrentValue json.RawMessage `json:"currentValue"`
	Options      json.RawMessage `json:"options,omitempty"`
}

// MarshalJSON writes the option in its schema shape.
func (o ConfigOption) MarshalJSON() ([]byte, error) {
	out := configOptionJSON{ID: o.ID, Name: o.Name, Description: o.Description, Category: o.Category, Type: o.Type}
	var err error
	if o.Current.Bool != nil {
		out.CurrentValue, err = json.Marshal(*o.Current.Bool)
	} else {
		out.CurrentValue, err = json.Marshal(o.Current.ID)
	}
	if err != nil {
		return nil, err
	}
	if o.Type == ConfigSelect {
		if out.Options, err = marshalSelectOptions(o.Options); err != nil {
			return nil, err
		}
	}
	return json.Marshal(out)
}

func marshalSelectOptions(opts []ConfigSelectOption) (json.RawMessage, error) {
	grouped := false
	for _, opt := range opts {
		if opt.Group != "" {
			grouped = true
			break
		}
	}
	if !grouped {
		flat := make([]configSelectOptionJSON, len(opts))
		for i, opt := range opts {
			flat[i] = configSelectOptionJSON{Value: opt.Value, Name: opt.Name, Description: opt.Description}
		}
		return json.Marshal(flat)
	}
	var groups []configGroupJSON
	for _, opt := range opts {
		if len(groups) == 0 || groups[len(groups)-1].Group != opt.Group {
			groups = append(groups, configGroupJSON{Group: opt.Group, Name: opt.GroupName})
		}
		g := &groups[len(groups)-1]
		g.Options = append(g.Options, configSelectOptionJSON{Value: opt.Value, Name: opt.Name, Description: opt.Description})
	}
	return json.Marshal(groups)
}

// UnmarshalJSON reads the schema shape.
func (o *ConfigOption) UnmarshalJSON(b []byte) error {
	var in configOptionJSON
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}
	*o = ConfigOption{ID: in.ID, Name: in.Name, Description: in.Description, Category: in.Category, Type: in.Type}
	switch in.Type {
	case ConfigBoolean:
		var v bool
		if err := json.Unmarshal(in.CurrentValue, &v); err != nil {
			return fmt.Errorf("config option currentValue: %w", err)
		}
		o.Current.Bool = &v
		return nil
	case ConfigSelect:
		if err := json.Unmarshal(in.CurrentValue, &o.Current.ID); err != nil {
			return fmt.Errorf("config option currentValue: %w", err)
		}
	default:
		return nil // future types keep their identity only
	}
	var items []struct {
		configSelectOptionJSON
		Group   string                   `json:"group"`
		Options []configSelectOptionJSON `json:"options"`
	}
	if len(in.Options) > 0 {
		if err := json.Unmarshal(in.Options, &items); err != nil {
			return fmt.Errorf("config option options: %w", err)
		}
	}
	for _, it := range items {
		if it.Group == "" {
			o.Options = append(o.Options, ConfigSelectOption{Value: it.Value, Name: it.Name, Description: it.Description})
			continue
		}
		for _, opt := range it.Options {
			o.Options = append(o.Options, ConfigSelectOption{Value: opt.Value, Name: opt.Name, Description: opt.Description, Group: it.Group, GroupName: it.Name})
		}
	}
	return nil
}

// SetConfigOptionRequest is the session/set_config_option params.
type SetConfigOptionRequest struct {
	SessionID string
	ConfigID  string
	Value     ConfigValue
}

type setConfigOptionJSON struct {
	SessionID string          `json:"sessionId"`
	ConfigID  string          `json:"configId"`
	Type      string          `json:"type,omitempty"`
	Value     json.RawMessage `json:"value"`
}

// MarshalJSON writes a boolean as {type: "boolean", value} and a select
// value as {value}.
func (r SetConfigOptionRequest) MarshalJSON() ([]byte, error) {
	out := setConfigOptionJSON{SessionID: r.SessionID, ConfigID: r.ConfigID}
	var err error
	if r.Value.Bool != nil {
		out.Type = ConfigBoolean
		out.Value, err = json.Marshal(*r.Value.Bool)
	} else {
		out.Value, err = json.Marshal(r.Value.ID)
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

// UnmarshalJSON reads either value shape.
func (r *SetConfigOptionRequest) UnmarshalJSON(b []byte) error {
	var in setConfigOptionJSON
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}
	*r = SetConfigOptionRequest{SessionID: in.SessionID, ConfigID: in.ConfigID}
	if in.Type == ConfigBoolean {
		var v bool
		if err := json.Unmarshal(in.Value, &v); err != nil {
			return fmt.Errorf("config value: %w", err)
		}
		r.Value.Bool = &v
		return nil
	}
	if err := json.Unmarshal(in.Value, &r.Value.ID); err != nil {
		return fmt.Errorf("config value: %w", err)
	}
	return nil
}

// SetConfigOptionResponse is the session/set_config_option result: the
// complete option set after the change.
type SetConfigOptionResponse struct {
	ConfigOptions []ConfigOption `json:"configOptions"`
}

// Permission option kinds.
const (
	PermissionAllowOnce    = "allow_once"
	PermissionAllowAlways  = "allow_always"
	PermissionRejectOnce   = "reject_once"
	PermissionRejectAlways = "reject_always"
)

// PermissionOption is one choice offered by session/request_permission.
type PermissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

// RequestPermissionRequest is the session/request_permission params.
type RequestPermissionRequest struct {
	SessionID string             `json:"sessionId"`
	ToolCall  ToolCallUpdate     `json:"toolCall"`
	Options   []PermissionOption `json:"options"`
}

// Permission outcomes.
const (
	OutcomeCancelled = "cancelled"
	OutcomeSelected  = "selected"
)

// PermissionOutcome is the answer: OutcomeSelected with OptionID, or
// OutcomeCancelled once the turn was cancelled.
type PermissionOutcome struct {
	Outcome  string `json:"outcome"`
	OptionID string `json:"optionId,omitempty"`
}

// RequestPermissionResponse is the session/request_permission result.
type RequestPermissionResponse struct {
	Outcome PermissionOutcome `json:"outcome"`
}

// Elicitation modes and actions.
const (
	ElicitationForm = "form"
	ElicitationURL  = "url"

	ElicitAccept  = "accept"
	ElicitDecline = "decline"
	ElicitCancel  = "cancel"
)

// CreateElicitationRequest is the elicitation/create params. A session
// scoped request has SessionID (and optionally ToolCallID); a request scoped
// one has RequestID.
type CreateElicitationRequest struct {
	Mode            string             `json:"mode"`
	Message         string             `json:"message"`
	SessionID       string             `json:"sessionId,omitempty"`
	ToolCallID      string             `json:"toolCallId,omitempty"`
	RequestID       json.RawMessage    `json:"requestId,omitempty"`
	RequestedSchema *ElicitationSchema `json:"requestedSchema,omitempty"`
}

// ElicitationSchema is a flat form: primitive properties only.
type ElicitationSchema struct {
	Type        string                         `json:"type,omitempty"`
	Title       string                         `json:"title,omitempty"`
	Description string                         `json:"description,omitempty"`
	Properties  map[string]ElicitationProperty `json:"properties,omitempty"`
	Required    []string                       `json:"required,omitempty"`
}

// ElicitationProperty is one form field. Type is string, number, integer,
// boolean or array (multi-select); the other fields apply per type.
type ElicitationProperty struct {
	Type        string            `json:"type"`
	Title       string            `json:"title,omitempty"`
	Description string            `json:"description,omitempty"`
	MinLength   *int              `json:"minLength,omitempty"`
	MaxLength   *int              `json:"maxLength,omitempty"`
	Pattern     string            `json:"pattern,omitempty"`
	Format      string            `json:"format,omitempty"`
	Default     json.RawMessage   `json:"default,omitempty"`
	Enum        []string          `json:"enum,omitempty"`
	OneOf       []EnumOption      `json:"oneOf,omitempty"`
	Minimum     *float64          `json:"minimum,omitempty"`
	Maximum     *float64          `json:"maximum,omitempty"`
	MinItems    *int              `json:"minItems,omitempty"`
	MaxItems    *int              `json:"maxItems,omitempty"`
	Items       *MultiSelectItems `json:"items,omitempty"`
}

// EnumOption is a titled choice.
type EnumOption struct {
	Const       string `json:"const"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

// MultiSelectItems lists the choices of a multi-select field: plain Enum
// values with Type "string", or titled AnyOf choices.
type MultiSelectItems struct {
	Type  string       `json:"type,omitempty"`
	Enum  []string     `json:"enum,omitempty"`
	AnyOf []EnumOption `json:"anyOf,omitempty"`
}

// CreateElicitationResponse is the elicitation/create result. Content
// values are strings, numbers, booleans or string slices.
type CreateElicitationResponse struct {
	Action  string         `json:"action"`
	Content map[string]any `json:"content,omitempty"`
}
