package core

import "testing"

func TestRequestMetaMatchesEveryIdentity(t *testing.T) {
	want := RequestMeta{Workspace: "workspace", Tab: "tab", Operation: "load", Request: 7}
	if !want.Matches(want) {
		t.Fatal("identical metadata did not match")
	}

	cases := []RequestMeta{
		{Workspace: "other", Tab: "tab", Operation: "load", Request: 7},
		{Workspace: "workspace", Tab: "other", Operation: "load", Request: 7},
		{Workspace: "workspace", Tab: "tab", Operation: "other", Request: 7},
		{Workspace: "workspace", Tab: "tab", Operation: "load", Request: 8},
	}
	for _, candidate := range cases {
		if want.Matches(candidate) {
			t.Fatalf("metadata unexpectedly matched %#v", candidate)
		}
	}
}
