package parser

import (
	"encoding/json"
	"log"
	"regexp"
	"strings"
)

type assistantResponseEvent struct {
	Content   string  `json:"content"`
	Input     *string `json:"input,omitempty"`
	Name      string  `json:"name"`
	ToolUseId string  `json:"toolUseId"`
	Stop      bool    `json:"stop"`
}

type usageEvent struct {
	Unit       string  `json:"unit"`
	UnitPlural string  `json:"unitPlural"`
	Usage      float64 `json:"usage"`
}

type SSEEvent struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

func ParseEvents(resp []byte) []SSEEvent {
	events := []SSEEvent{}
	
	// Check if this is CodeWhisperer binary format
	if isCodeWhispererFormat(resp) {
		return parseCodeWhispererEvents(resp)
	}
	
	// Parse standard SSE text format
	lines := strings.Split(string(resp), "\n")
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		// Handle SSE data lines
		if strings.HasPrefix(line, "data: ") {
			dataStr := strings.TrimPrefix(line, "data: ")
			if dataStr == "[DONE]" {
				break
			}
			
			var evt assistantResponseEvent
			if err := json.Unmarshal([]byte(dataStr), &evt); err == nil {
				events = append(events, convertAssistantEventToSSE(evt))
				
				if evt.ToolUseId != "" && evt.Name != "" {
					if evt.Stop {
						events = append(events, SSEEvent{
							Event: "message_delta",
							Data: map[string]interface{}{
								"type": "message_delta",
								"delta": map[string]interface{}{
									"stop_reason":   "tool_use",
									"stop_sequence": nil,
								},
								"usage": map[string]interface{}{"output_tokens": 0},
							},
						})
					}
				}
			} else {
				log.Println("json unmarshal error:", err, "data:", dataStr)
			}
		}
	}
	
	return events
}

func isCodeWhispererFormat(resp []byte) bool {
	// Check for CodeWhisperer specific markers
	respStr := string(resp)
	return strings.Contains(respStr, ":message-type") && 
		   strings.Contains(respStr, ":event-type") &&
		   strings.Contains(respStr, "assistantResponseEvent")
}

func parseCodeWhispererEvents(resp []byte) []SSEEvent {
	events := []SSEEvent{}
	respStr := string(resp)
	
	// Extract JSON objects from the binary stream
	jsonRegex := regexp.MustCompile(`\{"[^}]*"\}`)
	matches := jsonRegex.FindAllString(respStr, -1)
	
	for _, match := range matches {
		// Try to parse as content event
		var contentEvt assistantResponseEvent
		if err := json.Unmarshal([]byte(match), &contentEvt); err == nil && contentEvt.Content != "" {
			events = append(events, convertAssistantEventToSSE(contentEvt))
			continue
		}
		
		// Try to parse as usage event
		var usageEvt usageEvent
		if err := json.Unmarshal([]byte(match), &usageEvt); err == nil && usageEvt.Unit != "" {
			// Convert usage event to message_delta with usage info
			events = append(events, SSEEvent{
				Event: "message_delta",
				Data: map[string]interface{}{
					"type": "message_delta",
					"delta": map[string]interface{}{
						"stop_reason":   "end_turn",
						"stop_sequence": nil,
					},
					"usage": map[string]interface{}{
						"input_tokens":  0,
						"output_tokens": int(usageEvt.Usage * 1000), // Convert to approximate token count
					},
				},
			})
			continue
		}
		
		// Try to parse as tool use event
		var toolEvt assistantResponseEvent
		if err := json.Unmarshal([]byte(match), &toolEvt); err == nil && (toolEvt.ToolUseId != "" || toolEvt.Name != "") {
			events = append(events, convertAssistantEventToSSE(toolEvt))
			continue
		}
	}
	
	return events
}

func convertAssistantEventToSSE(evt assistantResponseEvent) SSEEvent {
	if evt.Content != "" {
		return SSEEvent{
			Event: "content_block_delta",
			Data: map[string]interface{}{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]interface{}{
					"type": "text_delta",
					"text": evt.Content,
				},
			},
		}
	} else if evt.ToolUseId != "" && evt.Name != "" && !evt.Stop {

		if evt.Input == nil {
			return SSEEvent{
				Event: "content_block_start",
				Data: map[string]interface{}{
					"type":  "content_block_start",
					"index": 1,
					"content_block": map[string]interface{}{
						"type":  "tool_use",
						"id":    evt.ToolUseId,
						"name":  evt.Name,
						"input": map[string]interface{}{},
					},
				},
			}
		} else {
			return SSEEvent{
				Event: "content_block_delta",
				Data: map[string]interface{}{
					"type":  "content_block_delta",
					"index": 1,
					"delta": map[string]interface{}{
						"type":         "input_json_delta",
						"id":           evt.ToolUseId,
						"name":         evt.Name,
						"partial_json": evt.Input,
					},
				},
			}
		}

	} else if evt.Stop {
		return SSEEvent{
			Event: "content_block_stop",
			Data: map[string]interface{}{
				"type":  "content_block_stop",
				"index": 1,
			},
		}
	}

	return SSEEvent{}
}
