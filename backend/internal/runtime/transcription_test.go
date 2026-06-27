package runtime

import "testing"

func TestTranscriptionProviderFromEnvOpenAICompatible(t *testing.T) {
	t.Setenv("TRANSCRIPTION_ENABLED", "true")
	t.Setenv("TRANSCRIPTION_PROVIDER", "openai_compatible")
	t.Setenv("TRANSCRIPTION_OPENAI_BASE_URL", "https://asr.example.test/v1")
	t.Setenv("TRANSCRIPTION_OPENAI_MODEL", "whisper-test")
	provider, enabled, err := TranscriptionProviderFromEnv()
	if err != nil {
		t.Fatalf("load provider: %v", err)
	}
	if !enabled || provider == nil || provider.Name() != "openai_compatible" {
		t.Fatalf("unexpected provider enabled=%v provider=%v", enabled, provider)
	}
}

func TestTranscriptionProviderFromEnvRejectsMissingConfiguration(t *testing.T) {
	t.Setenv("TRANSCRIPTION_ENABLED", "true")
	t.Setenv("TRANSCRIPTION_PROVIDER", "openai_compatible")
	t.Setenv("TRANSCRIPTION_OPENAI_BASE_URL", "")
	t.Setenv("TRANSCRIPTION_OPENAI_MODEL", "")
	if _, _, err := TranscriptionProviderFromEnv(); err == nil {
		t.Fatal("expected missing compatible provider configuration to fail")
	}
}

func TestTranscriptionProviderFromEnvStaysDisabled(t *testing.T) {
	t.Setenv("TRANSCRIPTION_ENABLED", "false")
	provider, enabled, err := TranscriptionProviderFromEnv()
	if err != nil || enabled || provider != nil {
		t.Fatalf("unexpected disabled result provider=%v enabled=%v err=%v", provider, enabled, err)
	}
}
