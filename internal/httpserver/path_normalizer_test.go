package httpserver

import "testing"

func TestNormalizePathLabel(t *testing.T) {
	testCases := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "empty path defaults to root",
			path:     "",
			expected: "/",
		},
		{
			name:     "static path remains unchanged",
			path:     "/healthz",
			expected: "/healthz",
		},
		{
			name:     "numeric segment normalized",
			path:     "/api/v1/stacks/12345",
			expected: "/api/v1/stacks/:id",
		},
		{
			name:     "uuid segment normalized",
			path:     "/api/v1/nodes/550e8400-e29b-41d4-a716-446655440000/state",
			expected: "/api/v1/nodes/:id/state",
		},
		{
			name:     "long hex segment normalized",
			path:     "/api/v1/resources/a94a8fe5ccb19ba61c4c0873d391e987982fbbd3",
			expected: "/api/v1/resources/:id",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizePathLabel(tc.path)
			if got != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}
