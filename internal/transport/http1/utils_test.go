package http1

import (
	"testing"
)

func TestWalkReverse(t *testing.T) {
	spits := func(input string, expected []string) {
		t.Helper()
		var got []string
		walkReverse(input, func(s string) bool {
			got = append(got, s)
			return true
		})
		if len(got) != len(expected) {
			t.Fatalf("unexpected number of tokens: %d, expected: %d", len(got), len(expected))
		}
		for i := range got {
			if got[i] != expected[i] {
				t.Errorf("unexpected token at index %d: %s, expected: %s", i, got[i], expected[i])
			}
		}
	}
	t.Run("simple", func(t *testing.T) {
		spits("a,b,c", []string{"c", "b", "a"})
		spits("a ", []string{"a"})
		spits(" a", []string{"a"})
	})
	t.Run("with spaces", func(t *testing.T) {
		spits("a, b , c ", []string{"c", "b", "a"})
	})
	t.Run("empty tokens", func(t *testing.T) {
		spits("a,,b, ,c", []string{"c", "", "b", "", "a"})
		spits(",,,,", []string{"", "", "", "", ""})
		spits("", []string{""})
		spits(" ", []string{""})
	})
}
