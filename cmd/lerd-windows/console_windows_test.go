package main

import (
	"fmt"
	"os"
	"testing"
)

// The second agent in a session must find the first one's claim, not take a
// claim of its own and fight it for port 53.
func TestClaimMutex_SecondClaimStepsAside(t *testing.T) {
	name := fmt.Sprintf(`Local\lerd-agent-test-%d`, os.Getpid())
	if !claimMutex(name) {
		t.Fatal("the first claim should succeed")
	}
	if claimMutex(name) {
		t.Error("a second claim should find the first one")
	}
}
