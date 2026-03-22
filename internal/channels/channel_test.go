package channels

import (
	"errors"
	"sort"
	"testing"

	"torus_go_agent/internal/config"
	"torus_go_agent/internal/core"
	"torus_go_agent/internal/features"
)

// mockChannel implements the Channel interface for testing.
type mockChannel struct {
	name      string
	startErr  error
	startChan chan struct{}
}

func (m *mockChannel) Name() string {
	return m.name
}

func (m *mockChannel) Start(agent *core.Agent, cfg config.Config, skills *features.SkillRegistry) error {
	if m.startChan != nil {
		<-m.startChan
	}
	return m.startErr
}

func TestRegister(t *testing.T) {
	// Save original registry to restore after test
	originalRegistry := registry
	defer func() { registry = originalRegistry }()

	tests := []struct {
		name     string
		channels []string
		setup    func()
		verify   func(t *testing.T)
	}{
		{
			name:     "register_single_channel",
			channels: []string{"tui"},
			setup: func() {
				registry = make(map[string]Channel)
			},
			verify: func(t *testing.T) {
				if len(registry) != 1 {
					t.Errorf("expected 1 channel registered, got %d", len(registry))
				}
				if _, ok := registry["tui"]; !ok {
					t.Error("expected 'tui' channel to be registered")
				}
			},
		},
		{
			name:     "register_multiple_channels",
			channels: []string{"tui", "telegram", "discord"},
			setup: func() {
				registry = make(map[string]Channel)
			},
			verify: func(t *testing.T) {
				if len(registry) != 3 {
					t.Errorf("expected 3 channels registered, got %d", len(registry))
				}
				for _, name := range []string{"tui", "telegram", "discord"} {
					if _, ok := registry[name]; !ok {
						t.Errorf("expected %q channel to be registered", name)
					}
				}
			},
		},
		{
			name:     "register_overwrites_existing",
			channels: []string{"tui", "tui"},
			setup: func() {
				registry = make(map[string]Channel)
			},
			verify: func(t *testing.T) {
				if len(registry) != 1 {
					t.Errorf("expected 1 channel registered (overwrite), got %d", len(registry))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			for _, chName := range tt.channels {
				ch := &mockChannel{name: chName}
				Register(ch)
			}
			tt.verify(t)
		})
	}
}

func TestGet(t *testing.T) {
	// Save original registry to restore after test
	originalRegistry := registry
	defer func() { registry = originalRegistry }()

	tests := []struct {
		name            string
		setup           func()
		queryName       string
		expectError     bool
		expectErrorText string
		verify          func(t *testing.T, ch Channel, err error)
	}{
		{
			name: "get_existing_channel",
			setup: func() {
				registry = make(map[string]Channel)
				Register(&mockChannel{name: "tui"})
			},
			queryName:   "tui",
			expectError: false,
			verify: func(t *testing.T, ch Channel, err error) {
				if ch == nil {
					t.Error("expected non-nil channel")
				}
				if ch.Name() != "tui" {
					t.Errorf("expected channel name 'tui', got %q", ch.Name())
				}
			},
		},
		{
			name: "get_nonexistent_channel",
			setup: func() {
				registry = make(map[string]Channel)
				Register(&mockChannel{name: "tui"})
			},
			queryName:       "discord",
			expectError:     true,
			expectErrorText: "unknown channel",
			verify: func(t *testing.T, ch Channel, err error) {
				if ch != nil {
					t.Error("expected nil channel when not found")
				}
			},
		},
		{
			name: "get_from_empty_registry",
			setup: func() {
				registry = make(map[string]Channel)
			},
			queryName:       "tui",
			expectError:     true,
			expectErrorText: "unknown channel",
			verify: func(t *testing.T, ch Channel, err error) {
				if ch != nil {
					t.Error("expected nil channel from empty registry")
				}
			},
		},
		{
			name: "get_with_case_sensitivity",
			setup: func() {
				registry = make(map[string]Channel)
				Register(&mockChannel{name: "tui"})
			},
			queryName:       "TUI",
			expectError:     true,
			expectErrorText: "unknown channel",
			verify: func(t *testing.T, ch Channel, err error) {
				if ch != nil {
					t.Error("expected nil channel - names are case sensitive")
				}
			},
		},
		{
			name: "get_empty_string",
			setup: func() {
				registry = make(map[string]Channel)
				Register(&mockChannel{name: "tui"})
			},
			queryName:       "",
			expectError:     true,
			expectErrorText: "unknown channel",
			verify: func(t *testing.T, ch Channel, err error) {
				if ch != nil {
					t.Error("expected nil channel for empty name")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			ch, err := Get(tt.queryName)

			if tt.expectError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
			if tt.expectError && err != nil && tt.expectErrorText != "" {
				if !contains(err.Error(), tt.expectErrorText) {
					t.Errorf("expected error containing %q, got: %v", tt.expectErrorText, err)
				}
			}

			tt.verify(t, ch, err)
		})
	}
}

func TestNames(t *testing.T) {
	// Save original registry to restore after test
	originalRegistry := registry
	defer func() { registry = originalRegistry }()

	tests := []struct {
		name           string
		setup          func()
		expectedNames  []string
		expectedLength int
	}{
		{
			name: "names_empty_registry",
			setup: func() {
				registry = make(map[string]Channel)
			},
			expectedNames:  []string{},
			expectedLength: 0,
		},
		{
			name: "names_single_channel",
			setup: func() {
				registry = make(map[string]Channel)
				Register(&mockChannel{name: "tui"})
			},
			expectedNames:  []string{"tui"},
			expectedLength: 1,
		},
		{
			name: "names_multiple_channels",
			setup: func() {
				registry = make(map[string]Channel)
				Register(&mockChannel{name: "tui"})
				Register(&mockChannel{name: "telegram"})
				Register(&mockChannel{name: "discord"})
			},
			expectedNames:  []string{"tui", "telegram", "discord"},
			expectedLength: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			names := Names()

			if len(names) != tt.expectedLength {
				t.Errorf("expected %d names, got %d", tt.expectedLength, len(names))
			}

			// Sort both slices for comparison since map iteration order is not guaranteed
			sort.Strings(names)
			sort.Strings(tt.expectedNames)

			for i, name := range names {
				if i >= len(tt.expectedNames) {
					t.Errorf("unexpected name: %q", name)
					continue
				}
				if name != tt.expectedNames[i] {
					t.Errorf("at index %d: expected %q, got %q", i, tt.expectedNames[i], name)
				}
			}
		})
	}
}

func TestChannelInterface(t *testing.T) {
	t.Run("channel_name", func(t *testing.T) {
		ch := &mockChannel{name: "test_channel"}
		name := ch.Name()
		if name != "test_channel" {
			t.Errorf("expected name %q, got %q", "test_channel", name)
		}
	})

	t.Run("channel_start_success", func(t *testing.T) {
		ch := &mockChannel{name: "test", startErr: nil, startChan: make(chan struct{})}
		go func() {
			ch.startChan <- struct{}{}
		}()

		err := ch.Start(nil, config.Config{}, nil)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	t.Run("channel_start_error", func(t *testing.T) {
		expectedErr := errors.New("start failed")
		ch := &mockChannel{name: "test", startErr: expectedErr}

		err := ch.Start(nil, config.Config{}, nil)
		if err != expectedErr {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})
}

func TestRegistryIsolation(t *testing.T) {
	// Save original registry
	originalRegistry := registry
	defer func() { registry = originalRegistry }()

	// Test that registry changes don't persist across tests
	t.Run("first_test", func(t *testing.T) {
		registry = make(map[string]Channel)
		Register(&mockChannel{name: "channel1"})
		if len(registry) != 1 {
			t.Errorf("expected 1 channel, got %d", len(registry))
		}
	})

	t.Run("second_test", func(t *testing.T) {
		registry = make(map[string]Channel)
		names := Names()
		if len(names) != 0 {
			t.Errorf("expected empty registry in new test, got %d names", len(names))
		}
	})
}

func TestGetErrorMessage(t *testing.T) {
	// Save original registry
	originalRegistry := registry
	defer func() { registry = originalRegistry }()

	registry = make(map[string]Channel)
	Register(&mockChannel{name: "tui"})
	Register(&mockChannel{name: "telegram"})

	_, err := Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent channel")
	}

	errMsg := err.Error()

	// Verify error message contains the queried name
	if !contains(errMsg, "nonexistent") {
		t.Errorf("expected error message to contain 'nonexistent', got: %s", errMsg)
	}

	// Verify error message lists available channels
	if !contains(errMsg, "available") {
		t.Errorf("expected error message to mention available channels, got: %s", errMsg)
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
