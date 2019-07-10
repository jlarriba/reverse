package main

import (
	"testing"
)

func TestReverse(t *testing.T) {
	toReverse := "to_reverse"
	reversed := "esrever_ot"

	if reverse(toReverse) != reversed {
		t.Errorf("Reversal was incorrect, got: %s", reverse(toReverse))
	}
}
