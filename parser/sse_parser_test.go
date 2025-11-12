package parser

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func TestParseCodeWhispererEvents(t *testing.T) {
	data, err := os.ReadFile("codewhisperer_response.raw")
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}

	events := ParseEvents(data)

	fmt.Printf("Parsed %d events:\n", len(events))
	for i, e := range events {
		fmt.Printf("Event %d:\n", i+1)
		fmt.Printf("  event: %s\n", e.Event)
		json, _ := json.Marshal(e.Data)
		fmt.Printf("  data: %s\n\n", string(json))
	}
}

func TestParseStandardSSEEvents(t *testing.T) {
	// Test with standard SSE format
	standardSSE := `data: {"content":"Hello ","name":"","toolUseId":"","stop":false}

data: {"content":"world!","name":"","toolUseId":"","stop":false}

data: [DONE]`

	events := ParseEvents([]byte(standardSSE))
	
	fmt.Printf("Standard SSE parsed %d events:\n", len(events))
	for i, e := range events {
		fmt.Printf("Event %d:\n", i+1)
		fmt.Printf("  event: %s\n", e.Event)
		json, _ := json.Marshal(e.Data)
		fmt.Printf("  data: %s\n\n", string(json))
	}
}
