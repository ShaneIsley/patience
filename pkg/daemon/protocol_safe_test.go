package daemon

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTypeSafeProtocolMessages validates that all protocol messages can be serialized/deserialized safely
func TestTypeSafeProtocolMessages(t *testing.T) {
	t.Run("HandshakeRequestJSON serialization", func(t *testing.T) {
		original := HandshakeRequestJSON{
			Type:    "handshake",
			Version: "1.0",
			Client:  "patience-cli",
		}

		// Serialize to JSON
		jsonData, err := json.Marshal(original)
		require.NoError(t, err)

		// Deserialize back
		var deserialized HandshakeRequestJSON
		err = json.Unmarshal(jsonData, &deserialized)
		require.NoError(t, err)

		// Verify data integrity
		assert.Equal(t, original.Type, deserialized.Type)
		assert.Equal(t, original.Version, deserialized.Version)
		assert.Equal(t, original.Client, deserialized.Client)
		assert.Equal(t, "handshake", deserialized.GetType())
	})

	t.Run("HandshakeResponseJSON serialization", func(t *testing.T) {
		original := HandshakeResponseJSON{
			Type:    "handshake_response",
			Status:  "ok",
			Message: "handshake successful",
		}

		jsonData, err := json.Marshal(original)
		require.NoError(t, err)

		var deserialized HandshakeResponseJSON
		err = json.Unmarshal(jsonData, &deserialized)
		require.NoError(t, err)

		assert.Equal(t, original, deserialized)
		assert.Equal(t, "handshake_response", deserialized.GetType())
	})

	t.Run("ScheduleRequestJSON serialization", func(t *testing.T) {
		requestTime := time.Now().UTC().Truncate(time.Second)
		original := ScheduleRequestJSON{
			Type:        "schedule_request",
			ResourceID:  "test-resource-123",
			Command:     []string{"curl", "-X", "GET", "https://api.example.com"},
			RequestedAt: requestTime,
		}

		jsonData, err := json.Marshal(original)
		require.NoError(t, err)

		var deserialized ScheduleRequestJSON
		err = json.Unmarshal(jsonData, &deserialized)
		require.NoError(t, err)

		assert.Equal(t, original.Type, deserialized.Type)
		assert.Equal(t, original.ResourceID, deserialized.ResourceID)
		assert.Equal(t, original.Command, deserialized.Command)
		assert.Equal(t, original.RequestedAt.Unix(), deserialized.RequestedAt.Unix())
		assert.Equal(t, "schedule_request", deserialized.GetType())
	})

	t.Run("RegisterRequestJSON serialization", func(t *testing.T) {
		requestTime := time.Now().UTC().Truncate(time.Second)
		original := RegisterRequestJSON{
			Type:       "register_request",
			ResourceID: "test-resource-456",
			Requests: []RequestInfoJSON{
				{
					RequestedAt: requestTime,
					Command:     []string{"curl", "https://api1.example.com"},
				},
				{
					RequestedAt: requestTime.Add(time.Second),
					Command:     []string{"curl", "https://api2.example.com"},
				},
			},
		}

		jsonData, err := json.Marshal(original)
		require.NoError(t, err)

		var deserialized RegisterRequestJSON
		err = json.Unmarshal(jsonData, &deserialized)
		require.NoError(t, err)

		assert.Equal(t, original.Type, deserialized.Type)
		assert.Equal(t, original.ResourceID, deserialized.ResourceID)
		require.Len(t, deserialized.Requests, 2)
		assert.Equal(t, original.Requests[0].Command, deserialized.Requests[0].Command)
		assert.Equal(t, original.Requests[1].Command, deserialized.Requests[1].Command)
		assert.Equal(t, "register_request", deserialized.GetType())
	})
}

// TestTypeSafeClientMethods validates that client methods use type-safe structs
func TestTypeSafeClientMethods(t *testing.T) {
	t.Run("SendHandshakeTypeSafe method exists", func(t *testing.T) {
		// Test that the method signature exists by checking it compiles
		// We don't actually call it since we don't have a connection

		request := HandshakeRequestJSON{
			Type:    "handshake",
			Version: "1.0",
			Client:  "test-client",
		}

		// Verify the method exists and has correct signature
		client := &DaemonClient{}

		// This should compile, proving the method exists with correct signature
		var _ func(HandshakeRequestJSON) (HandshakeResponseJSON, error) = client.SendHandshakeTypeSafe

		// Verify the request struct is properly formed
		assert.Equal(t, "handshake", request.Type)
		assert.Equal(t, "1.0", request.Version)
		assert.Equal(t, "test-client", request.Client)
	})

	t.Run("SendScheduleRequestTypeSafe method exists", func(t *testing.T) {
		request := ScheduleRequestJSON{
			Type:        "schedule_request",
			ResourceID:  "test-resource",
			Command:     []string{"echo", "test"},
			RequestedAt: time.Now(),
		}

		client := &DaemonClient{}

		// Verify method signature exists
		var _ func(ScheduleRequestJSON) (ScheduleResponseJSON, error) = client.SendScheduleRequestTypeSafe

		// Verify the request struct is properly formed
		assert.Equal(t, "schedule_request", request.Type)
		assert.Equal(t, "test-resource", request.ResourceID)
		assert.Equal(t, []string{"echo", "test"}, request.Command)
	})

	t.Run("SendRegisterRequestTypeSafe method exists", func(t *testing.T) {
		request := RegisterRequestJSON{
			Type:       "register_request",
			ResourceID: "test-resource",
			Requests: []RequestInfoJSON{
				{RequestedAt: time.Now(), Command: []string{"echo", "test"}},
			},
		}

		client := &DaemonClient{}

		// Verify method signature exists
		var _ func(RegisterRequestJSON) (RegisterResponseJSON, error) = client.SendRegisterRequestTypeSafe

		// Verify the request struct is properly formed
		assert.Equal(t, "register_request", request.Type)
		assert.Equal(t, "test-resource", request.ResourceID)
		assert.Len(t, request.Requests, 1)
	})
}

// TestTypeSafeServerHandlers validates that server handlers use type-safe structs
func TestTypeSafeServerHandlers(t *testing.T) {
	server := &UnixServer{}

	t.Run("handleHandshakeTypeSafe method exists and works", func(t *testing.T) {
		request := HandshakeRequestJSON{
			Type:    "handshake",
			Version: "1.0",
			Client:  "test-client",
		}

		response := server.handleHandshakeTypeSafe(request)

		assert.Equal(t, "handshake_response", response.Type)
		assert.Equal(t, "ok", response.Status)
	})

	t.Run("handleScheduleRequestTypeSafe method exists and works", func(t *testing.T) {
		request := ScheduleRequestJSON{
			Type:        "schedule_request",
			ResourceID:  "test-resource",
			Command:     []string{"echo", "test"},
			RequestedAt: time.Now(),
		}

		response := server.handleScheduleRequestTypeSafe(request)

		assert.Equal(t, "schedule_response", response.Type)
		assert.NotEmpty(t, response.Status)
	})

	t.Run("handleRegisterRequestTypeSafe method exists and works", func(t *testing.T) {
		request := RegisterRequestJSON{
			Type:       "register_request",
			ResourceID: "test-resource",
			Requests: []RequestInfoJSON{
				{RequestedAt: time.Now(), Command: []string{"echo", "test"}},
			},
		}

		response := server.handleRegisterRequestTypeSafe(request)

		assert.Equal(t, "register_response", response.Type)
		assert.NotEmpty(t, response.Status)
	})
}

// TestBackwardCompatibility ensures new type-safe implementation produces same JSON as old map[string]interface{}
func TestBackwardCompatibility(t *testing.T) {
	t.Run("HandshakeRequestJSON produces same JSON as old map[string]interface{}", func(t *testing.T) {
		// Old way (what we're replacing)
		oldRequest := map[string]interface{}{
			"type":    "handshake",
			"version": "1.0",
			"client":  "patience-cli",
		}
		oldJSON, err := json.Marshal(oldRequest)
		require.NoError(t, err)

		// New way (type-safe)
		newRequest := HandshakeRequestJSON{
			Type:    "handshake",
			Version: "1.0",
			Client:  "patience-cli",
		}
		newJSON, err := json.Marshal(newRequest)
		require.NoError(t, err)

		// Should produce identical JSON
		assert.JSONEq(t, string(oldJSON), string(newJSON))
	})

	t.Run("ScheduleRequestJSON produces same JSON as old map[string]interface{}", func(t *testing.T) {
		requestTime := time.Now().UTC().Truncate(time.Second)

		// Old way
		oldRequest := map[string]interface{}{
			"type":         "schedule_request",
			"resource_id":  "test-resource",
			"command":      []string{"curl", "https://api.example.com"},
			"requested_at": requestTime,
		}
		oldJSON, err := json.Marshal(oldRequest)
		require.NoError(t, err)

		// New way
		newRequest := ScheduleRequestJSON{
			Type:        "schedule_request",
			ResourceID:  "test-resource",
			Command:     []string{"curl", "https://api.example.com"},
			RequestedAt: requestTime,
		}
		newJSON, err := json.Marshal(newRequest)
		require.NoError(t, err)

		// Should produce identical JSON
		assert.JSONEq(t, string(oldJSON), string(newJSON))
	})
}

// TestTypeSafetyImprovements validates that type safety prevents common errors
func TestTypeSafetyImprovements(t *testing.T) {
	t.Run("Type-safe structs prevent field name typos", func(t *testing.T) {
		// With old map[string]interface{}, typos like "resouce_id" instead of "resource_id"
		// would only be caught at runtime. With structs, they're caught at compile time.

		request := ScheduleRequestJSON{
			Type:        "schedule_request",
			ResourceID:  "test-resource", // Compiler ensures correct field name
			Command:     []string{"echo", "test"},
			RequestedAt: time.Now(),
		}

		// This will compile successfully, proving type safety
		assert.Equal(t, "test-resource", request.ResourceID)
	})

	t.Run("Type-safe structs prevent type mismatches", func(t *testing.T) {
		// With old map[string]interface{}, you could accidentally put wrong types
		// With structs, the compiler prevents this

		request := HandshakeRequestJSON{
			Type:    "handshake",
			Version: "1.0",         // Must be string, not int
			Client:  "test-client", // Must be string, not []string
		}

		assert.IsType(t, "", request.Version)
		assert.IsType(t, "", request.Client)
	})
}
