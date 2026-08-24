package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAIProviderIsolation(t *testing.T) {
	// Start a mock server to intercept requests and verify credentials
	var mu sync.Mutex
	receivedKeys := make(map[string]bool)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		key := ""
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			key = authHeader[7:]
		}

		mu.Lock()
		receivedKeys[key] = true
		mu.Unlock()

		// Mock successful Ollama response
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"response": "mock response for " + key,
		})
	}))
	defer server.Close()

	factory := NewProviderFactory()
	
	// Create channels for synchronization
	startCh := make(chan struct{})
	var wg sync.WaitGroup

	tenantCount := 10
	wg.Add(tenantCount)

	// Launch concurrent requests with distinct configurations
	for i := 0; i < tenantCount; i++ {
		go func(tenantID int) {
			defer wg.Done()
			
			key := "tenant_key_" + string(rune('A'+tenantID))
			
			cfg := ProviderConfig{
				ProviderName:    "ollama",
				DecryptedAPIKey: key,
				BaseURL:         server.URL,
				Timeout:         2 * time.Second,
			}

			provider, err := factory.New(context.Background(), cfg)
			require.NoError(t, err)

			<-startCh // Wait for all goroutines to be ready

			res, err := provider.Generate(context.Background(), GenerateRequest{Prompt: "test"})
			require.NoError(t, err)
			assert.Contains(t, res.Text, key)
		}(i)
	}

	// Trigger all goroutines simultaneously
	close(startCh)
	wg.Wait()

	// Verify that the server received all unique keys
	mu.Lock()
	defer mu.Unlock()
	for i := 0; i < tenantCount; i++ {
		key := "tenant_key_" + string(rune('A'+i))
		assert.True(t, receivedKeys[key], "Server did not receive key: %s", key)
	}
	assert.Equal(t, tenantCount, len(receivedKeys), "Expected %d unique keys, got %d", tenantCount, len(receivedKeys))
}
