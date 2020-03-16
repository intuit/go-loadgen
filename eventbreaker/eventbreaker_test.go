package eventbreaker

import "testing"

func TestHasKnownEventBreakerString(t *testing.T) {
	eb := NewEventBreakers()
	result := eb.HasKnownEventBreakerString("[2019-12-05T18:44:57,007+0000] randomField=1 msg=\"this is a test message\"")
	if !result {
		t.Errorf("eventbreaker did not detect a known date pattern [2019-12-05T18:44:57,007+0000] in the input message")
	}
	result = eb.HasKnownEventBreakerString("2019-12-05T18:44:57,007 randomField=1 msg=\"this is a test message\"")
	if !result {
		t.Errorf("eventbreaker did not detect a known date pattern 2019-12-05T18:44:57,007 in the input message")
	}
	result = eb.HasKnownEventBreakerString("2019-12-05 18:44:57 randomField=1 msg=\"this is a test message\"")
	if !result {
		t.Errorf("eventbreaker did not detect a known date pattern 2019-12-05 18:44:57 in the input message")
	}
	//with whitespace in the beginning
	result = eb.HasKnownEventBreakerString(" 2019-12-05 18:44:57 randomField=1 msg=\"this is a test message\"")
	if !result {
		t.Errorf("eventbreaker did not detect a known date pattern \" 2019-12-05 18:44:57\" in the input message")
	}
}