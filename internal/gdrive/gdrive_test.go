package gdrive

import "testing"

func TestAPIErrorUnescapes(t *testing.T) {
	e := &APIError{Status: 401, Body: `{"error":"invalid_client","error_description":"Only clients of type &#39;TVs and Limited Input devices&#39; can use it."}`}
	want := "google api 401: invalid_client: Only clients of type 'TVs and Limited Input devices' can use it."
	if got := e.Error(); got != want {
		t.Fatalf("got %q", got)
	}
}
