package httpserver

import "strings"

func normalizePathLabel(path string) string {
	if path == "" {
		return "/"
	}

	trimmed := strings.TrimSpace(path)
	if trimmed == "" || trimmed == "/" {
		return "/"
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}

	parts := strings.Split(trimmed, "/")
	normalized := make([]string, 0, len(parts))
	normalized = append(normalized, "")

	for _, part := range parts[1:] {
		if part == "" {
			continue
		}
		if isDynamicPathSegment(part) {
			normalized = append(normalized, ":id")
			continue
		}
		normalized = append(normalized, part)
	}

	if len(normalized) == 1 {
		return "/"
	}
	return strings.Join(normalized, "/")
}

func isDynamicPathSegment(segment string) bool {
	if segment == "" {
		return false
	}
	return isAllDigits(segment) || isUUID(segment) || isLongHex(segment)
}

func isAllDigits(value string) bool {
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func isLongHex(value string) bool {
	if len(value) < 16 {
		return false
	}
	for _, char := range value {
		if !isHexChar(char) {
			return false
		}
	}
	return true
}

func isUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, char := range value {
		switch i {
		case 8, 13, 18, 23:
			if char != '-' {
				return false
			}
		default:
			if !isHexChar(char) {
				return false
			}
		}
	}
	return true
}

func isHexChar(char rune) bool {
	return (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')
}
