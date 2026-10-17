package tools

import "testing"

func TestFacadeConstructorsRemainAvailable(t *testing.T) {
	if NewMessageTool() == nil {
		t.Fatal("NewMessageTool should return a tool")
	}
	if NewReactionTool() == nil {
		t.Fatal("NewReactionTool should return a tool")
	}
}
