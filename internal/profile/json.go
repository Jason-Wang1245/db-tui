package profile

import "encoding/json"

var documentJSONFields = []string{"version", "profiles"}

var profileJSONFields = []string{
	"id", "name", "host", "port", "database", "user", "ssl_mode",
	"advanced_parameters", "save_password", "created_at", "updated_at", "last_used_at",
}

func (document Document) MarshalJSON() ([]byte, error) {
	type known Document
	return marshalWithUnknown(known(document), document.Unknown)
}

func (document *Document) UnmarshalJSON(data []byte) error {
	type known Document
	var decoded known
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*document = Document(decoded)
	unknown, err := unknownFields(data, documentJSONFields)
	if err != nil {
		return err
	}
	document.Unknown = unknown
	return nil
}

func (saved Profile) MarshalJSON() ([]byte, error) {
	type known Profile
	return marshalWithUnknown(known(saved), saved.Unknown)
}

func (saved *Profile) UnmarshalJSON(data []byte) error {
	type known Profile
	var decoded known
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*saved = Profile(decoded)
	unknown, err := unknownFields(data, profileJSONFields)
	if err != nil {
		return err
	}
	saved.Unknown = unknown
	return nil
}

func marshalWithUnknown(known any, unknown map[string]json.RawMessage) ([]byte, error) {
	encoded, err := json.Marshal(known)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return nil, err
	}
	for name, value := range unknown {
		if _, knownField := fields[name]; !knownField {
			fields[name] = value
		}
	}
	return json.Marshal(fields)
}

func unknownFields(data []byte, known []string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	for _, name := range known {
		delete(fields, name)
	}
	if len(fields) == 0 {
		return nil, nil
	}
	return fields, nil
}
