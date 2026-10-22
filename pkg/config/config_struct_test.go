package config

import (
	"encoding/json"
	"testing"
)

func TestSecureStringJSONKeepsReference(t *testing.T) {
	t.Setenv("TEST_SECRET", "s3cret")
	s := NewSecureString("$TEST_SECRET")
	data, err := json.Marshal(struct {
		Token SecureString `json:"token"`
	}{Token: *s})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"token":"$TEST_SECRET"}` {
		t.Fatalf("json = %s", data)
	}
}

func TestSecureStringJSONKeepsPlaintext(t *testing.T) {
	s := NewSecureString("plain")
	data, err := json.Marshal(struct {
		Token SecureString `json:"token"`
	}{Token: *s})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"token":"plain"}` {
		t.Fatalf("json = %s", data)
	}
}

func TestSecureStringsJSONKeepsReferences(t *testing.T) {
	t.Setenv("KEY_A", "a")
	t.Setenv("KEY_B", "b")
	values := SecureStrings{NewSecureString("$KEY_A"), NewSecureString("$KEY_B")}
	data, err := json.Marshal(struct {
		Keys SecureStrings `json:"keys"`
	}{Keys: values})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"keys":["$KEY_A","$KEY_B"]}` {
		t.Fatalf("json = %s", data)
	}
}
