package conversation

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
)

const capacityErrorEvent = `{"sequence_number":0,"type":"error","code":null,"message":"The model is currently at capacity due to high demand."}`

func TestConvertResponsesStreamNormalizesCapacityError(t *testing.T) {
	stream := "event: error\ndata: " + capacityErrorEvent + "\n\n"
	converted, err := io.ReadAll(ConvertResponseStream(io.NopCloser(strings.NewReader(stream)), OperationChat))
	if err != nil {
		t.Fatal(err)
	}
	text := string(converted)
	if !strings.Contains(text, `"error":{"code":"model_capacity_exceeded"`) || !strings.Contains(text, `"message":"The model is currently at capacity due to high demand."`) || strings.Contains(text, `"choices"`) || strings.Count(text, "data: [DONE]") != 1 {
		t.Fatalf("chat error stream = %s", text)
	}
}

func TestConvertResponsesStreamNormalizesMessagesCapacityError(t *testing.T) {
	stream := "event: error\ndata: " + capacityErrorEvent + "\n\n"
	converted, err := io.ReadAll(ConvertResponseStream(io.NopCloser(strings.NewReader(stream)), OperationMessages))
	if err != nil {
		t.Fatal(err)
	}
	text := string(converted)
	if !strings.Contains(text, `event: error`) || !strings.Contains(text, `"error":{"message":"The model is currently at capacity due to high demand.","type":"api_error"}`) || strings.Contains(text, `sequence_number`) {
		t.Fatalf("messages error stream = %s", text)
	}
}

func TestConvertResponsesJSONNormalizesRootCapacityError(t *testing.T) {
	converted, err := ConvertResponseJSON([]byte(capacityErrorEvent), OperationChat)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(converted, &payload); err != nil {
		t.Fatal(err)
	}
	errorObject, _ := payload["error"].(map[string]any)
	if errorObject["code"] != "model_capacity_exceeded" || errorObject["message"] != "The model is currently at capacity due to high demand." {
		t.Fatalf("chat error = %#v", payload)
	}
}
