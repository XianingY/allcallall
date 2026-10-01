package server

import "testing"

// Logs are the widest-retained store in most systems and a full IP identifies
// a person, so the default must be the redacted form.
func TestMaskLoggedIPRedactsTheHostPortion(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"ipv4 keeps the /24", "203.0.113.45", "203.0.113.0/24"},
		{"ipv4 loopback", "127.0.0.1", "127.0.0.0/24"},
		{"ipv6 keeps the /48", "2001:db8:1234:5678:9abc:def0:1234:5678", "2001:db8:1234::/48"},
		{"ipv4-mapped ipv6 is treated as ipv4", "::ffff:203.0.113.45", "203.0.113.0/24"},
		{"empty stays empty", "", ""},
		{"unparseable is not echoed", "not-an-ip", "invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LOG_FULL_IP", "")
			if got := maskLoggedIP(tc.in); got != tc.want {
				t.Fatalf("maskLoggedIP(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// The escape hatch has to work, or incident response cannot get host detail.
func TestMaskLoggedIPHonoursTheFullIPOptIn(t *testing.T) {
	t.Setenv("LOG_FULL_IP", "true")
	if got := maskLoggedIP("203.0.113.45"); got != "203.0.113.45" {
		t.Fatalf("with LOG_FULL_IP=true the address should pass through, got %q", got)
	}
}

func TestMaskLoggedIPDefaultIsRedactedEvenWithoutEnv(t *testing.T) {
	// t.Setenv registers cleanup, so set it empty rather than unsetting.
	t.Setenv("LOG_FULL_IP", "")
	if got := maskLoggedIP("198.51.100.7"); got == "198.51.100.7" {
		t.Fatal("the default must redact; only LOG_FULL_IP=true may bypass")
	}
}
