package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/config"
	"github.com/allcallall/backend/internal/translation"
	"github.com/allcallall/backend/internal/translation/providers"
)

type smokeReport struct {
	Provider             string    `json:"provider"`
	StartedAt            time.Time `json:"started_at"`
	EndedAt              time.Time `json:"ended_at"`
	DurationSec          int       `json:"duration_sec"`
	ChunkMS              int       `json:"chunk_ms"`
	SourceLang           string    `json:"source_lang"`
	TargetLang           string    `json:"target_lang"`
	SentChunks           int       `json:"sent_chunks"`
	PartialCount         int       `json:"partial_count"`
	FinalCount           int       `json:"final_count"`
	ErrorCount           int       `json:"error_count"`
	DisconnectCount      int       `json:"disconnect_count"`
	FirstPartialDelayMS  int64     `json:"first_partial_delay_ms"`
	FirstFinalDelayMS    int64     `json:"first_final_delay_ms"`
	ObservedLatencyMSP50 int64     `json:"observed_latency_ms_p50"`
	ObservedLatencyMSP95 int64     `json:"observed_latency_ms_p95"`
	RecoverableErrors    []string  `json:"recoverable_errors"`
	NonRecoverableErrors []string  `json:"non_recoverable_errors"`
}

func main() {
	var (
		wsURL      = flag.String("ws-url", os.Getenv("VOLC_AST_WS_URL"), "volc ast websocket url")
		appKey     = flag.String("app-key", os.Getenv("VOLC_AST_APP_KEY"), "volc ast app key")
		accessKey  = flag.String("access-key", os.Getenv("VOLC_AST_ACCESS_KEY"), "volc ast access key")
		resourceID = flag.String("resource-id", os.Getenv("VOLC_AST_RESOURCE_ID"), "volc ast resource id")
		appID      = flag.String("app-id", os.Getenv("VOLC_AST_APP_ID"), "volc ast app id (optional)")

		sourceLang = flag.String("source-lang", "zh", "source language")
		targetLang = flag.String("target-lang", "en", "target language")
		duration   = flag.Int("duration-sec", 600, "streaming duration in seconds")
		chunkMS    = flag.Int("chunk-ms", 400, "chunk size in milliseconds")
		reportPath = flag.String("report", "/Users/byzantium/github/allcallall/backend/tmp/translation_smoke_report.json", "output report json path")
	)
	flag.Parse()

	logger := zerolog.New(os.Stdout).With().Timestamp().Str("component", "translation_smoke").Logger()
	provider := providers.NewVolcASTProvider(logger, config.VolcASTConfig{
		WSURL:      *wsURL,
		AppKey:     *appKey,
		AccessKey:  *accessKey,
		ResourceID: *resourceID,
		AppID:      *appID,
	})
	service := translation.NewService(logger, provider, 1)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*duration+15)*time.Second)
	defer cancel()

	report := smokeReport{
		Provider:    provider.Name(),
		StartedAt:   time.Now().UTC(),
		DurationSec: *duration,
		ChunkMS:     *chunkMS,
		SourceLang:  *sourceLang,
		TargetLang:  *targetLang,
	}

	session, err := service.StartSession(ctx, "smoke@allcallall.local", translation.StartRequest{
		CallID:     "smoke-call",
		To:         "smoke-peer@allcallall.local",
		SourceLang: *sourceLang,
		TargetLang: *targetLang,
		ChunkMS:    *chunkMS,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "start session failed: %v\n", err)
		report.ErrorCount++
		report.NonRecoverableErrors = append(report.NonRecoverableErrors, err.Error())
		finishReport(report, *reportPath)
		os.Exit(1)
	}
	defer func() {
		_ = session.Stop(context.Background())
	}()

	latencies := make([]int64, 0, 64)
	eventsDone := make(chan struct{})
	go func() {
		defer close(eventsDone)
		for evt := range session.Events() {
			if evt.Error != nil {
				report.ErrorCount++
				if strings.Contains(strings.ToUpper(evt.Error.Code), "PROVIDER_ERROR") &&
					strings.Contains(strings.ToLower(evt.Error.Message), "closed") {
					report.DisconnectCount++
				}
				if evt.Error.Recoverable {
					report.RecoverableErrors = append(report.RecoverableErrors, evt.Error.Code+":"+evt.Error.Message)
				} else {
					report.NonRecoverableErrors = append(report.NonRecoverableErrors, evt.Error.Code+":"+evt.Error.Message)
				}
				continue
			}
			if evt.Result == nil {
				continue
			}
			if evt.Result.LatencyMS > 0 {
				latencies = append(latencies, evt.Result.LatencyMS)
			}
			if evt.Result.IsFinal {
				report.FinalCount++
				if report.FirstFinalDelayMS == 0 {
					report.FirstFinalDelayMS = time.Since(report.StartedAt).Milliseconds()
				}
			} else {
				report.PartialCount++
				if report.FirstPartialDelayMS == 0 {
					report.FirstPartialDelayMS = time.Since(report.StartedAt).Milliseconds()
				}
			}
		}
	}()

	ticker := time.NewTicker(time.Duration(*chunkMS) * time.Millisecond)
	defer ticker.Stop()
	deadline := time.Now().Add(time.Duration(*duration) * time.Second)

	seq := int64(1)
	silence := makeSilenceChunkBase64(*chunkMS, 16000)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			break
		case <-ticker.C:
			err := session.SendAudio(ctx, translation.AudioChunk{
				Seq:         seq,
				PCM16Base64: silence,
				SampleRate:  16000,
				Channels:    1,
				TimestampMS: time.Now().UnixMilli(),
			})
			if err != nil {
				report.ErrorCount++
				report.RecoverableErrors = append(report.RecoverableErrors, "SEND_AUDIO:"+err.Error())
			}
			report.SentChunks++
			seq++
		}
	}

	_ = session.Stop(context.Background())
	<-eventsDone

	report.EndedAt = time.Now().UTC()
	report.ObservedLatencyMSP50 = percentile(latencies, 50)
	report.ObservedLatencyMSP95 = percentile(latencies, 95)

	if err := finishReport(report, *reportPath); err != nil {
		fmt.Fprintf(os.Stderr, "write report failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("smoke report written to %s\n", *reportPath)
}

func makeSilenceChunkBase64(chunkMS int, sampleRate int) string {
	if chunkMS <= 0 {
		chunkMS = 400
	}
	if sampleRate <= 0 {
		sampleRate = 16000
	}
	sampleCount := (sampleRate * chunkMS) / 1000
	if sampleCount <= 0 {
		sampleCount = 1
	}
	// int16 PCM, mono.
	buf := make([]byte, sampleCount*2)
	return base64.StdEncoding.EncodeToString(buf)
}

func percentile(values []int64, p int) int64 {
	if len(values) == 0 {
		return 0
	}
	if p <= 0 {
		return values[0]
	}
	if p >= 100 {
		return values[len(values)-1]
	}

	sorted := make([]int64, len(values))
	copy(sorted, values)
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	idx := (p * (len(sorted) - 1)) / 100
	return sorted[idx]
}

func finishReport(report smokeReport, path string) error {
	if report.EndedAt.IsZero() {
		report.EndedAt = time.Now().UTC()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Clean(path), data, 0o644)
}
