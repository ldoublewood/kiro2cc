package parser

import (
	"encoding/json"
	"log"
	"strings"
)

type assistantResponseEvent struct {
	Content   string  `json:"content"`
	Input     *string `json:"input,omitempty"`
	Name      string  `json:"name"`
	ToolUseId string  `json:"toolUseId"`
	Stop      bool    `json:"stop"`
}

type SSEEvent struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

func ParseEvents(resp []byte) []SSEEvent {
	events := []SSEEvent{}
	
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
