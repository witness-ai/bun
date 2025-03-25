package internal

func IsUpper(c byte) bool {
	return c >= 'A' && c <= 'Z'
}

func IsLower(c byte) bool {
	return c >= 'a' && c <= 'z'
}

func ToUpper(c byte) byte {
	return c - 32
}

func ToLower(c byte) byte {
	return c + 32
}

// Underscore converts "CamelCasedString" to "camel_cased_string".
func Underscore(s string) string {
	if len(s) == 0 {
		return s
	}

	// Pre-allocate buffer with extra space for potential underscores
	r := make([]byte, 0, len(s)*2)

	for i := 0; i < len(s); i++ {
		c := s[i]

		// Handle underscore -> double underscore
		if c == '_' {
			r = append(r, '_', '_')
			continue
		}

		// Special handling for "ID" and "IDs" patterns in the string
		if c == 'I' && i+1 < len(s) && s[i+1] == 'D' {
			// Check for "IDs" (plural form)
			if i+2 < len(s) && s[i+2] == 's' && (i+3 >= len(s) || !IsLower(s[i+3])) {
				// We have "IDs" at the end or followed by non-lowercase
				if i > 0 {
					// Not at the beginning, add underscore if needed
					if r[len(r)-1] != '_' {
						r = append(r, '_')
					}
				}
				r = append(r, 'i', 'd', 's')
				i += 2 // Skip the 'Ds'
				continue
			}

			// Check for "ID" at the end or followed by uppercase
			if i+2 >= len(s) || !IsLower(s[i+2]) {
				// We have "ID" - followed by end of string or non-lowercase
				if i > 0 {
					// Not at the beginning, add underscore if needed
					if r[len(r)-1] != '_' {
						r = append(r, '_')
					}
				}
				r = append(r, 'i', 'd')
				i++ // Skip the 'D'
				continue
			}
		}

		// Check if we need to insert an underscore before this character
		if i > 0 && IsUpper(c) {
			// Don't add underscore for single capital at end of string
			if i == len(s)-1 {
				r = append(r, ToLower(c))
				continue
			}

			prev := s[i-1]

			// Add underscore in two cases:
			// 1. Transition from lowercase to uppercase (camelCase -> camel_case)
			// 2. End of acronym (HTTPRequest -> http_request)
			if IsLower(prev) || (IsUpper(prev) && i+1 < len(s) && IsLower(s[i+1])) {
				r = append(r, '_')
			}
		}

		// Add lowercase version of the current character
		if IsUpper(c) {
			r = append(r, ToLower(c))
		} else {
			r = append(r, c)
		}
	}

	return string(r)
}

func CamelCased(s string) string {
	r := make([]byte, 0, len(s))
	upperNext := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' {
			upperNext = true
			continue
		}
		if upperNext {
			if IsLower(c) {
				c = ToUpper(c)
			}
			upperNext = false
		}
		r = append(r, c)
	}
	return string(r)
}

func ToExported(s string) string {
	if len(s) == 0 {
		return s
	}
	if c := s[0]; IsLower(c) {
		b := []byte(s)
		b[0] = ToUpper(c)
		return string(b)
	}
	return s
}
