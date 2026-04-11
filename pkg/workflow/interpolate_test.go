package workflow

import (
	"encoding/json"
	"fmt"
	"testing"
)

// rawGeocodeResponse is the exact JSON the geocode API returns.
const rawGeocodeResponse = `{
	"results": [{
		"lat": 47.6144219,
		"lon": -122.192337,
		"timezone": {"name": "America/Los_Angeles"}
	}]
}`

// simulatePBRoundTrip marshals then unmarshals a value through JSON,
// replicating what PocketBase does when saving and reloading the workflow context.
func simulatePBRoundTrip(v any) map[string]any {
	b, _ := json.Marshal(v)
	var out map[string]any
	json.Unmarshal(b, &out)
	return out
}

func TestTypeHintNumber(t *testing.T) {
	// Exact context from the geocode step (no round-trip)
	var geocodeBody map[string]any
	json.Unmarshal([]byte(rawGeocodeResponse), &geocodeBody)

	ctx := map[string]any{
		"geocode": geocodeBody,
	}

	bodyStr := "{\n  \"latitude\": \"{{geocode.results.0.lat|number}}\",\n  \"longitude\": \"{{geocode.results.0.lon|number}}\",\n  \"timezone\": \"{{geocode.results.0.timezone.name}}\"\n}"

	result := deepInterpolate(bodyStr, ctx)
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T: %v", result, result)
	}

	lat := m["latitude"]
	f, ok := lat.(float64)
	if !ok {
		t.Fatalf("latitude should be float64, got %T: %v", lat, lat)
	}
	if f != 47.6144219 {
		t.Fatalf("latitude = %v, want 47.6144219", f)
	}
	t.Logf("latitude = %v (type: %T) ✓", lat, lat)
	t.Logf("longitude = %v (type: %T) ✓", m["longitude"], m["longitude"])
}

// TestTypeHintAfterPBRoundTrip simulates what happens when context is
// serialized to JSON (PocketBase storage) and deserialized back.
func TestTypeHintAfterPBRoundTrip(t *testing.T) {
	var geocodeBody map[string]any
	json.Unmarshal([]byte(rawGeocodeResponse), &geocodeBody)

	// This is what the context looks like AFTER PocketBase round-trip
	ctx := simulatePBRoundTrip(map[string]any{
		"geocode": geocodeBody,
	})

	t.Logf("ctx[geocode] type: %T", ctx["geocode"])
	geocode, _ := ctx["geocode"].(map[string]any)
	if geocode == nil {
		t.Fatalf("ctx[geocode] is nil or not map[string]any, got %T: %v", ctx["geocode"], ctx["geocode"])
	}
	results, _ := geocode["results"].([]any)
	if results == nil {
		t.Fatalf("geocode[results] is nil or not []any, got %T: %v", geocode["results"], geocode["results"])
	}
	first, _ := results[0].(map[string]any)
	if first == nil {
		t.Fatalf("results[0] not map[string]any, got %T", results[0])
	}
	t.Logf("lat type: %T, value: %v", first["lat"], first["lat"])

	bodyStr := "{\n  \"latitude\": \"{{geocode.results.0.lat|number}}\",\n  \"longitude\": \"{{geocode.results.0.lon|number}}\"\n}"
	result := deepInterpolate(bodyStr, ctx)
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T: %v", result, result)
	}
	lat := m["latitude"]
	t.Logf("latitude after round-trip: %v (type: %T)", lat, lat)
	if _, ok := lat.(float64); !ok {
		t.Errorf("FAIL: latitude should be float64, got %T: %v", lat, lat)
	}
}

func TestResolveCtxKey(t *testing.T) {
	var geocodeBody map[string]any
	json.Unmarshal([]byte(rawGeocodeResponse), &geocodeBody)
	ctx := map[string]any{"geocode": geocodeBody}

	cases := []struct{ key, want string }{
		{"geocode.results.0.lat", "47.6144219"},
		{"geocode.results.0.lon", "-122.192337"},
		{"geocode.results.0.timezone.name", "America/Los_Angeles"},
	}
	for _, c := range cases {
		got := resolveCtxKey(c.key, ctx)
		if got != c.want {
			t.Errorf("resolveCtxKey(%q) = %q, want %q", c.key, got, c.want)
		} else {
			t.Logf("resolveCtxKey(%q) = %q ✓", c.key, got)
		}
	}
}

// TestTelegramConditionRouting verifies that {{message.voice.file_id}} exists
// correctly routes voice vs text Telegram messages.
// This exercises lookupPath + evalCondition with the exact context structure
// produced by the Telegram webhook handler after the raw message is injected.
// TestSplitFallback verifies that {{key1}} || {{key2}} resolves as a fallback.
func TestSplitFallback(t *testing.T) {
	cases := []struct {
		name       string
		template   string
		ctx        map[string]any
		want       string
	}{
		{
			name:     "left wins when present",
			template: `{"text": "{{message.text}} || {{transcript}}"}`,
			ctx:      map[string]any{"message": map[string]any{"text": "start topic"}, "transcript": ""},
			want:     "start topic",
		},
		{
			name:     "right wins when left empty",
			template: `{"text": "{{message.text}} || {{transcript}}"}`,
			ctx:      map[string]any{"message": map[string]any{"text": ""}, "transcript": "hello from voice"},
			want:     "hello from voice",
		},
		{
			name:     "right wins when left missing",
			template: `{"text": "{{message.text}} || {{transcript}}"}`,
			ctx:      map[string]any{"transcript": "hello from voice"},
			want:     "hello from voice",
		},
		{
			name:     "empty when both missing",
			template: `{"text": "{{message.text}} || {{transcript}}"}`,
			ctx:      map[string]any{},
			want:     "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := deepInterpolate(c.template, c.ctx)
			m, ok := result.(map[string]any)
			if !ok {
				t.Fatalf("expected map, got %T: %v", result, result)
			}
			got := fmt.Sprintf("%v", m["text"])
			if got != c.want {
				t.Errorf("text = %q, want %q", got, c.want)
			} else {
				t.Logf("text = %q ✓", got)
			}
		})
	}
}

func TestTelegramConditionRouting(t *testing.T) {
	// Simulate what telegram_handler.go puts in context for a TEXT message:
	// flat keys + nested "message" from the raw Telegram JSON.
	textCtx := map[string]any{
		"chat_id":      int64(-1003878227299),
		"user_id":      int64(6326470139),
		"text":         "start topic",
		"message_type": "text",
		"message": map[string]any{
			"message_id": int64(8),
			"text":       "start topic",
			"from": map[string]any{
				"id":       int64(6326470139),
				"username": "laurenpoppinsraye",
			},
			"chat": map[string]any{"id": int64(-1003878227299), "type": "supergroup"},
			// NO "voice" key
		},
	}

	// Simulate a VOICE message context:
	voiceCtx := map[string]any{
		"chat_id":       int64(-1003878227299),
		"voice_file_id": "BQACAgIAAx0ABC123",
		"message_type":  "voice",
		"message": map[string]any{
			"message_id": int64(9),
			"voice": map[string]any{
				"file_id":   "BQACAgIAAx0ABC123",
				"duration":  12,
				"mime_type": "audio/ogg",
			},
		},
	}

	voiceCondition := Condition{Key: "{{message.voice.file_id}}", Operator: "exists"}

	if evalCondition(voiceCondition, textCtx) {
		t.Error("FAIL: {{message.voice.file_id}} exists evaluated TRUE for text message — should be FALSE")
	} else {
		t.Log("text message: {{message.voice.file_id}} exists = false ✓")
	}

	if !evalCondition(voiceCondition, voiceCtx) {
		t.Error("FAIL: {{message.voice.file_id}} exists evaluated FALSE for voice message — should be TRUE")
	} else {
		t.Log("voice message: {{message.voice.file_id}} exists = true ✓")
	}

	// Also verify that matchAll with empty conditions always passes (default branch).
	if !matchAll(nil, textCtx) {
		t.Error("FAIL: matchAll(nil) should always be true")
	}
}

func TestTypeHintRegex(t *testing.T) {
	bodyStr := "{\n  \"latitude\": \"{{geocode.results.0.lat|number}}\"\n}"
	matches := typeHintRe.FindAllString(bodyStr, -1)
	t.Logf("typeHintRe matches in body: %v", matches)
	if len(matches) == 0 {
		t.Fatal("typeHintRe found no matches — regex is broken")
	}
	t.Logf("match[0] = %q", matches[0])
	match := matches[0]
	inner := match[3 : len(match)-3]
	t.Logf("inner = %q", inner)
	fmt.Printf("inner bytes: %v\n", []byte(inner))
}

func TestBareNumberPlaceholder(t *testing.T) {
	var geocodeBody map[string]any
	json.Unmarshal([]byte(rawGeocodeResponse), &geocodeBody)
	ctx := map[string]any{"geocode": geocodeBody}

	// No quotes around {{...}}, no type hints — bare number in JSON.
	// deepInterpolate returns a string (can't pre-parse invalid JSON),
	// but values are substituted. http_request then JSON-parses the result
	// and latitude lands as float64.
	bodyStr := "{\n  \"latitude\": {{geocode.results.0.lat}},\n  \"longitude\": {{geocode.results.0.lon}},\n  \"timezone\": \"{{geocode.results.0.timezone.name}}\"\n}"

	result := deepInterpolate(bodyStr, ctx)
	s, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T: %v", result, result)
	}
	t.Logf("interpolated body: %s", s)

	// Simulate what http_request does: parse the substituted string as JSON
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("FAIL: interpolated body is not valid JSON: %v\nbody: %s", err, s)
	}
	lat := m["latitude"]
	t.Logf("latitude = %v (type: %T)", lat, lat)
	if _, ok := lat.(float64); !ok {
		t.Errorf("FAIL: latitude should be float64, got %T: %v", lat, lat)
	}
}
