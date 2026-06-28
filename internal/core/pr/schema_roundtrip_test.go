package pr

import "testing"

// TestSchemaRoundTrip guards the agent contract: the JSON emitted by --schema
// must parse back through ParseJSON. Regression test for `draft` being emitted
// as the string "false" (which fails to unmarshal into the bool Draft field).
func TestSchemaRoundTrip(t *testing.T) {
	b, err := Schema()
	if err != nil {
		t.Fatalf("Schema() error = %v", err)
	}

	in, err := ParseJSON(b)
	if err != nil {
		t.Fatalf("ParseJSON(schema) error = %v; schema = %s", err, b)
	}

	if in.Draft != false {
		t.Errorf("expected Draft=false from schema default, got %v", in.Draft)
	}
}
