package http1

import (
	"strings"
)

func Cut(s, sep string) (before, after string, found bool) {
	if i := strings.Index(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return s, "", false
}

// https://www.rfc-editor.org/rfc/rfc9110#section-5.6.1
func walkReverse(s string, fn func(string) bool) (last string) {
	once := true
	i := len(s) - 1
	for {
		for i > 0 && (s[i] == ' ' || s[i] == '\t') && s[i] != ',' {
			i--
		}
		start, end := i+1, i+1
		for ; i >= 0 && s[i] != ','; i-- {
			if s[i] > ' ' {
				start = i
			}
		}
		if start >= 0 {
			token := s[start:end]
			if fn == nil {
				return token
			}
			if once {
				last, once = token, false
			}
			if !fn(token) {
				return
			}
		}
		if i < 0 {
			return
		}
		i--
	}
}
