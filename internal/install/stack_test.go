package install

import "testing"

func TestStackFor(t *testing.T) {
	cases := map[string]Stack{
		"2.4.9":    {"8.5", "11.4", "3.0.0", "22"},
		"2.4.8":    {"8.4", "11.4", "2.19.1", "22"},
		"2.4.7-p3": {"8.3", "10.6", "2.12.0", "22"},
		"2.4.6":    {"8.2", "10.6", "2.5.0", "22"},
		"2.4.5":    {"8.1", "10.4", "1.2.4", "22"},
		"2.4.4":    {"8.1", "10.4", "1.2.4", "22"},
	}
	for v, want := range cases {
		got, err := StackFor(v)
		if err != nil || got != want {
			t.Errorf("%s: got %+v %v want %+v", v, got, err, want)
		}
	}
	if _, err := StackFor("2.3.7"); err == nil {
		t.Error("2.3.7 should error")
	}
}
