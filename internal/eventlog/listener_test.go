package eventlog

import "testing"

func TestListenerCurrentTokenUsesProvider(t *testing.T) {
	t.Parallel()

	token := "first-token"
	listener := NewListener(nil, nil, "ws://example", func() string {
		return token
	})

	if got := listener.currentToken(); got != "first-token" {
		t.Fatalf("expected first token, got %q", got)
	}

	token = "second-token"
	if got := listener.currentToken(); got != "second-token" {
		t.Fatalf("expected refreshed token, got %q", got)
	}
}

func TestListenerCurrentTokenFallsBackToStaticToken(t *testing.T) {
	t.Parallel()

	listener := &Listener{token: "  static-token  "}
	if got := listener.currentToken(); got != "static-token" {
		t.Fatalf("expected static token fallback, got %q", got)
	}
}
