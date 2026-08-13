package profile

import "testing"

func TestSessionSecretsLifecycle(t *testing.T) {
	secrets := NewSessionSecrets()
	secrets.Set(ID("profile"), "password")
	if got, ok := secrets.Get(ID("profile")); !ok || got != "password" {
		t.Fatalf("Get returned %q, %t", got, ok)
	}
	secrets.Delete(ID("profile"))
	if _, ok := secrets.Get(ID("profile")); ok {
		t.Fatal("deleted secret remains available")
	}
}
