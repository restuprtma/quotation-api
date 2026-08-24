package service

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestProvider_ConcurrentIsolation(t *testing.T) {
	// A mock server that verifies the Authorization header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("x-goog-api-key")
		// Simulate network delay
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf(`{"candidates":[{"content":{"parts":[{"text":"hello %s"}]}}]}`, key)))
	}))
	defer server.Close()

	factory := NewProviderFactory()
	var wg sync.WaitGroup
	numRequests := 50

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			expectedKey := fmt.Sprintf("tenant_%d_key", id)
			cfg := ProviderConfig{
				ProviderName:    "gemini",
				DecryptedAPIKey: expectedKey,
				BaseURL:         server.URL,
			}
			
			provider, err := factory.New(context.Background(), cfg)
			require.NoError(t, err)
			
			gemini, ok := provider.(*GeminiProvider)
			require.True(t, ok)
			
			// Simulate race condition scenario where another tenant gets created
			time.Sleep(10 * time.Millisecond)
			assert.Equal(t, expectedKey, gemini.cfg.DecryptedAPIKey)
		}(i)
	}
	wg.Wait()
}

func TestProvider_KeyRotationRace(t *testing.T) {
	factory := NewProviderFactory()
	
	// Tenant API key in "database" (memory for test)
	currentKey := "old_key"
	
	cfg := ProviderConfig{
		ProviderName:    "gemini",
		DecryptedAPIKey: currentKey,
	}
	provider, err := factory.New(context.Background(), cfg)
	require.NoError(t, err)

	// Simulate tenant updating key in DB concurrently
	go func() {
		currentKey = "new_key" // This rotation shouldn't affect the running provider
	}()

	time.Sleep(5 * time.Millisecond)
	gemini := provider.(*GeminiProvider)
	assert.Equal(t, "old_key", gemini.cfg.DecryptedAPIKey)
}
