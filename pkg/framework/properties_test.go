package framework_test

import (
	"reflect"
	"testing"

	"github.com/zncdatadev/operator-go/pkg/framework"
)

func TestPropertiesCodecEncodesEntriesDeterministically(t *testing.T) {
	values := map[string]string{
		"z": "$(touch NEVER); `exit 42` ${POD_NAME}",
		"":  "WARN",
		"a": "日志🦆",
	}
	want := "=WARN\na=日志🦆\nz=$(touch NEVER); `exit 42` ${POD_NAME}\n"
	var codec framework.PropertyCodec = framework.PropertiesCodec{}
	for range 3 {
		got, err := codec.Encode(values)
		if err != nil || got != want {
			t.Fatalf("encoded=%q, want=%q, error=%v", got, want, err)
		}
	}
	if !reflect.DeepEqual(values, map[string]string{
		"z": "$(touch NEVER); `exit 42` ${POD_NAME}", "": "WARN", "a": "日志🦆",
	}) {
		t.Fatal("codec mutated product property values")
	}
}

func TestPropertiesCodecEscapesSyntaxAndPreservesWhitespace(t *testing.T) {
	values := map[string]string{
		" #!=:\\\n\r\t\f": " leading \nnext\r\t\f\\ trailing ",
	}
	want := `\ \#\!\=\:\\\n\r\t\f=\ leading \nnext\r\t\f\\ trailing\ ` + "\n"
	got, err := (framework.PropertiesCodec{}).Encode(values)
	if err != nil || got != want {
		t.Fatalf("encoded=%q, want=%q, error=%v", got, want, err)
	}
	for _, test := range []struct{ value, want string }{
		{"", "key=\n"}, {" ", "key=\\ \n"}, {"  ", "key=\\ \\ \n"}, {"one two", "key=one two\n"},
	} {
		got, err := (framework.PropertiesCodec{}).Encode(map[string]string{"key": test.value})
		if err != nil || got != test.want {
			t.Fatalf("value=%q encoded=%q, want=%q, error=%v", test.value, got, test.want, err)
		}
	}
}

func TestPropertiesCodecRejectsInvalidUTF8WithoutPartialOutput(t *testing.T) {
	for _, values := range []map[string]string{
		{"ok": "value", "z": string([]byte{0xff})}, {string([]byte{0xff}): "value"},
	} {
		if got, err := (framework.PropertiesCodec{}).Encode(values); err == nil || got != "" {
			t.Fatalf("invalid UTF-8 produced output %q or no error: %v", got, err)
		}
	}
	for _, values := range []map[string]string{nil, {}} {
		if got, err := (framework.PropertiesCodec{}).Encode(values); err != nil || got != "" {
			t.Fatalf("empty properties produced %q, %v", got, err)
		}
	}
}
