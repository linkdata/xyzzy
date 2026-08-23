package main

import (
	"flag"
	"testing"

	"github.com/linkdata/jaws"
)

func TestConfigureJaws(t *testing.T) {
	tests := []struct {
		name                  string
		debug                 bool
		trustForwardedHeaders bool
	}{
		{name: "defaults"},
		{name: "debug", debug: true},
		{name: "trusted forwarding", trustForwardedHeaders: true},
		{name: "both", debug: true, trustForwardedHeaders: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jw, err := jaws.New()
			if err != nil {
				t.Fatalf("jaws.New() error = %v", err)
			}
			t.Cleanup(jw.Close)

			configureJaws(jw, tt.debug, tt.trustForwardedHeaders)

			if jw.Debug != tt.debug {
				t.Errorf("Debug = %t, want %t", jw.Debug, tt.debug)
			}
			if jw.TrustForwardedHeaders != tt.trustForwardedHeaders {
				t.Errorf("TrustForwardedHeaders = %t, want %t", jw.TrustForwardedHeaders, tt.trustForwardedHeaders)
			}
			if jw.CookieName != "xyzzy" {
				t.Errorf("CookieName = %q, want %q", jw.CookieName, "xyzzy")
			}
			if jw.Logger == nil {
				t.Error("Logger is nil")
			}
		})
	}
}

func TestTrustForwardedHeadersFlagDefaultsDisabled(t *testing.T) {
	f := flag.Lookup("trust-forwarded-headers")
	if f == nil {
		t.Fatal("trust-forwarded-headers flag is not registered")
	}
	if f.DefValue != "false" {
		t.Errorf("trust-forwarded-headers default = %q, want %q", f.DefValue, "false")
	}
}
