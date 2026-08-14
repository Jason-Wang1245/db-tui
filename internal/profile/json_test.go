package profile

import (
	"encoding/json"
	"testing"
)

func TestUnknownJSONFieldsSurviveRoundTrip(t *testing.T) {
	original := []byte(`{
  "version": 1,
  "future_document": {"enabled": true},
  "profiles": [{
    "id": "profile-1", "name": "Local", "host": "localhost", "port": 5432,
    "database": "app", "user": "developer", "ssl_mode": "prefer",
    "save_password": false, "created_at": "2026-08-13T00:00:00Z",
    "updated_at": "2026-08-13T00:00:00Z", "future_profile": [1, 2, 3]
  }]
}`)
	var document Document
	if err := json.Unmarshal(original, &document); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["future_document"]; !ok {
		t.Fatal("future document field was dropped")
	}
	var profiles []map[string]json.RawMessage
	if err := json.Unmarshal(fields["profiles"], &profiles); err != nil {
		t.Fatal(err)
	}
	if _, ok := profiles[0]["future_profile"]; !ok {
		t.Fatal("future profile field was dropped")
	}
}
