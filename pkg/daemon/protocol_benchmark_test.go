package daemon

import (
	"encoding/json"
	"testing"
	"time"
)

// BenchmarkProtocolSerialization compares performance of old vs new protocol handling
func BenchmarkProtocolSerialization(b *testing.B) {
	b.Run("Old map[string]interface{} approach", func(b *testing.B) {
		request := map[string]interface{}{
			"type":         "schedule_request",
			"resource_id":  "test-resource-123",
			"command":      []string{"curl", "-X", "GET", "https://api.example.com"},
			"requested_at": time.Now(),
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Serialize
			data, err := json.Marshal(request)
			if err != nil {
				b.Fatal(err)
			}

			// Deserialize
			var response map[string]interface{}
			err = json.Unmarshal(data, &response)
			if err != nil {
				b.Fatal(err)
			}

			// Type assertions (common in old approach)
			_, ok := response["type"].(string)
			if !ok {
				b.Fatal("type assertion failed")
			}
		}
	})

	b.Run("New type-safe struct approach", func(b *testing.B) {
		request := ScheduleRequestJSON{
			Type:        "schedule_request",
			ResourceID:  "test-resource-123",
			Command:     []string{"curl", "-X", "GET", "https://api.example.com"},
			RequestedAt: time.Now(),
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Serialize
			data, err := json.Marshal(request)
			if err != nil {
				b.Fatal(err)
			}

			// Deserialize
			var response ScheduleRequestJSON
			err = json.Unmarshal(data, &response)
			if err != nil {
				b.Fatal(err)
			}

			// Direct field access (no type assertions needed)
			_ = response.Type
		}
	})
}

// BenchmarkProtocolMessageHandling compares protocol message processing performance
func BenchmarkProtocolMessageHandling(b *testing.B) {
	server := &UnixServer{}

	handshakeMessage := `{"type":"handshake","version":"1.0","client":"test-client"}`
	scheduleMessage := `{"type":"schedule_request","resource_id":"test-123","command":["echo","test"],"requested_at":"2025-07-29T10:00:00Z"}`
	registerMessage := `{"type":"register_request","resource_id":"test-456","requests":[{"requested_at":"2025-07-29T10:00:00Z","command":["curl","https://api.example.com"]}]}`

	b.Run("Old handleProtocolMessage (map[string]interface{})", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Test different message types
			switch i % 3 {
			case 0:
				_ = server.handleProtocolMessage(handshakeMessage)
			case 1:
				_ = server.handleProtocolMessage(scheduleMessage)
			case 2:
				_ = server.handleProtocolMessage(registerMessage)
			}
		}
	})

	b.Run("New handleProtocolMessageTypeSafe (structs)", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Test different message types
			switch i % 3 {
			case 0:
				_ = server.handleProtocolMessageTypeSafe(handshakeMessage)
			case 1:
				_ = server.handleProtocolMessageTypeSafe(scheduleMessage)
			case 2:
				_ = server.handleProtocolMessageTypeSafe(registerMessage)
			}
		}
	})
}

// BenchmarkClientTypeSafeMethods compares client method performance
func BenchmarkClientTypeSafeMethods(b *testing.B) {
	// Note: These benchmarks test the method call overhead, not actual network I/O
	// since we don't have a real connection in benchmarks

	b.Run("Type-safe method call overhead", func(b *testing.B) {
		client := &DaemonClient{}
		request := HandshakeRequestJSON{
			Type:    "handshake",
			Version: "1.0",
			Client:  "benchmark-client",
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// This will fail due to no connection, but we're measuring the overhead
			// of the type-safe method call structure
			_, _ = client.SendHandshakeTypeSafe(request)
		}
	})
}

// BenchmarkMemoryAllocation compares memory allocation patterns
func BenchmarkMemoryAllocation(b *testing.B) {
	b.Run("Old map[string]interface{} allocation", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Create map (heap allocation)
			request := map[string]interface{}{
				"type":         "schedule_request",
				"resource_id":  "test-resource",
				"command":      []string{"echo", "test"},
				"requested_at": time.Now(),
			}

			// Prevent optimization
			_ = request["type"]
		}
	})

	b.Run("New struct allocation", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Create struct (can be stack allocated)
			request := ScheduleRequestJSON{
				Type:        "schedule_request",
				ResourceID:  "test-resource",
				Command:     []string{"echo", "test"},
				RequestedAt: time.Now(),
			}

			// Prevent optimization
			_ = request.Type
		}
	})
}

// BenchmarkJSONProcessing compares JSON processing performance
func BenchmarkJSONProcessing(b *testing.B) {
	jsonData := []byte(`{"type":"handshake","version":"1.0","client":"test-client"}`)

	b.Run("Old approach - unmarshal to map then type assert", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var request map[string]interface{}
			err := json.Unmarshal(jsonData, &request)
			if err != nil {
				b.Fatal(err)
			}

			// Type assertions (runtime overhead)
			msgType, ok := request["type"].(string)
			if !ok || msgType != "handshake" {
				b.Fatal("type assertion failed")
			}

			version, ok := request["version"].(string)
			if !ok || version != "1.0" {
				b.Fatal("version assertion failed")
			}

			client, ok := request["client"].(string)
			if !ok || client != "test-client" {
				b.Fatal("client assertion failed")
			}
		}
	})

	b.Run("New approach - unmarshal to struct", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var request HandshakeRequestJSON
			err := json.Unmarshal(jsonData, &request)
			if err != nil {
				b.Fatal(err)
			}

			// Direct field access (compile-time type safety)
			if request.Type != "handshake" {
				b.Fatal("type check failed")
			}

			if request.Version != "1.0" {
				b.Fatal("version check failed")
			}

			if request.Client != "test-client" {
				b.Fatal("client check failed")
			}
		}
	})
}
