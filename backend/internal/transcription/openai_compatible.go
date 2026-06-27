package transcription

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultTranscriptionTimeout   = 2 * time.Minute
	defaultTranscriptionChunk     = 10 * time.Minute
	defaultTranscriptionMaxUpload = int64(24 * 1024 * 1024)
	maxTranscriptionResponseBytes = int64(8 * 1024 * 1024)
)

type OpenAICompatibleConfig struct {
	BaseURL        string
	APIKey         string
	Model          string
	Language       string
	Timeout        time.Duration
	ChunkDuration  time.Duration
	MaxUploadBytes int64
	FFmpegPath     string
	HTTPClient     *http.Client
}

type OpenAICompatibleProvider struct {
	endpoint       string
	apiKey         string
	model          string
	language       string
	timeout        time.Duration
	chunkDuration  time.Duration
	maxUploadBytes int64
	ffmpegPath     string
	client         *http.Client
}

func NewOpenAICompatibleProvider(config OpenAICompatibleConfig) (*OpenAICompatibleProvider, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		return nil, errors.New("TRANSCRIPTION_OPENAI_BASE_URL is required")
	}
	if strings.TrimSpace(config.Model) == "" {
		return nil, errors.New("TRANSCRIPTION_OPENAI_MODEL is required")
	}
	if config.Timeout <= 0 {
		config.Timeout = defaultTranscriptionTimeout
	}
	if config.ChunkDuration <= 0 {
		config.ChunkDuration = defaultTranscriptionChunk
	}
	if config.MaxUploadBytes <= 0 {
		config.MaxUploadBytes = defaultTranscriptionMaxUpload
	}
	if strings.TrimSpace(config.FFmpegPath) == "" {
		config.FFmpegPath = "ffmpeg"
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: config.Timeout}
	}
	return &OpenAICompatibleProvider{
		endpoint:       baseURL + "/audio/transcriptions",
		apiKey:         strings.TrimSpace(config.APIKey),
		model:          strings.TrimSpace(config.Model),
		language:       strings.TrimSpace(config.Language),
		timeout:        config.Timeout,
		chunkDuration:  config.ChunkDuration,
		maxUploadBytes: config.MaxUploadBytes,
		ffmpegPath:     strings.TrimSpace(config.FFmpegPath),
		client:         client,
	}, nil
}

func (*OpenAICompatibleProvider) Name() string {
	return "openai_compatible"
}

func (p *OpenAICompatibleProvider) TranscribeFile(ctx context.Context, input FileInput) ([]Segment, error) {
	if strings.TrimSpace(input.LocalPath) == "" {
		return nil, &ProviderError{Operation: "prepare", Err: errors.New("local audio path is required")}
	}
	chunks, cleanup, err := p.prepareChunks(ctx, input.LocalPath, input.DurationSeconds)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	trackKey := trackKeyFromMetadata(input.MetadataJSON)
	if trackKey == "" {
		trackKey = filepath.Base(input.LocalPath)
	}
	speakerID := participantIDFromTrackKey(trackKey)
	segments := make([]Segment, 0, len(chunks))
	for _, chunk := range chunks {
		items, err := p.transcribeChunk(ctx, chunk.path)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			item.StartMS += chunk.offsetMS
			item.EndMS += chunk.offsetMS
			item.TrackKey = trackKey
			item.SpeakerUserID = speakerID
			segments = append(segments, item)
		}
	}
	return segments, nil
}

type preparedAudioChunk struct {
	path     string
	offsetMS int64
}

func (p *OpenAICompatibleProvider) prepareChunks(ctx context.Context, sourcePath string, durationSeconds int64) ([]preparedAudioChunk, func(), error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return nil, func() {}, &ProviderError{Operation: "prepare", Err: err}
	}
	chunkSeconds := int64(p.chunkDuration / time.Second)
	if chunkSeconds <= 0 {
		chunkSeconds = int64(defaultTranscriptionChunk / time.Second)
	}
	if info.Size() <= p.maxUploadBytes && (durationSeconds <= 0 || durationSeconds <= chunkSeconds) {
		return []preparedAudioChunk{{path: sourcePath}}, func() {}, nil
	}

	tempDir, err := os.MkdirTemp("", "allcallall-transcription-chunks-")
	if err != nil {
		return nil, func() {}, &ProviderError{Operation: "chunk", Err: err}
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }
	outputPattern := filepath.Join(tempDir, "chunk-%06d.ogg")
	command := exec.CommandContext(ctx, p.ffmpegPath,
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", sourcePath,
		"-map", "0:a:0", "-c", "copy",
		"-f", "segment", "-segment_time", strconv.FormatInt(chunkSeconds, 10),
		"-reset_timestamps", "1", outputPattern,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		cleanup()
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return nil, func() {}, &ProviderError{Operation: "chunk", Err: errors.New(message)}
	}
	paths, err := filepath.Glob(filepath.Join(tempDir, "chunk-*.ogg"))
	if err != nil || len(paths) == 0 {
		cleanup()
		if err == nil {
			err = errors.New("ffmpeg produced no audio chunks")
		}
		return nil, func() {}, &ProviderError{Operation: "chunk", Err: err}
	}
	sort.Strings(paths)
	chunks := make([]preparedAudioChunk, 0, len(paths))
	for index, path := range paths {
		chunkInfo, statErr := os.Stat(path)
		if statErr != nil {
			cleanup()
			return nil, func() {}, &ProviderError{Operation: "chunk", Err: statErr}
		}
		if chunkInfo.Size() > p.maxUploadBytes {
			cleanup()
			return nil, func() {}, &ProviderError{Operation: "chunk", Err: fmt.Errorf("audio chunk exceeds %d bytes", p.maxUploadBytes)}
		}
		chunks = append(chunks, preparedAudioChunk{
			path:     path,
			offsetMS: int64(index) * chunkSeconds * 1000,
		})
	}
	return chunks, cleanup, nil
}

type openAITranscriptionResponse struct {
	Text     string  `json:"text"`
	Language string  `json:"language"`
	Duration float64 `json:"duration"`
	Segments []struct {
		Start      float64  `json:"start"`
		End        float64  `json:"end"`
		Text       string   `json:"text"`
		Confidence *float64 `json:"confidence"`
		AvgLogProb *float64 `json:"avg_logprob"`
	} `json:"segments"`
}

func (p *OpenAICompatibleProvider) transcribeChunk(ctx context.Context, path string) ([]Segment, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, &ProviderError{Operation: "upload", Err: err}
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		return nil, &ProviderError{Operation: "upload", Err: err}
	}
	copied, err := io.Copy(part, io.LimitReader(file, p.maxUploadBytes+1))
	if err != nil {
		return nil, &ProviderError{Operation: "upload", Err: err}
	}
	if copied > p.maxUploadBytes {
		return nil, &ProviderError{Operation: "upload", Err: fmt.Errorf("audio file exceeds %d bytes", p.maxUploadBytes)}
	}
	_ = writer.WriteField("model", p.model)
	_ = writer.WriteField("response_format", "verbose_json")
	_ = writer.WriteField("timestamp_granularities[]", "segment")
	if p.language != "" {
		_ = writer.WriteField("language", p.language)
	}
	if err := writer.Close(); err != nil {
		return nil, &ProviderError{Operation: "upload", Err: err}
	}

	requestCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, p.endpoint, &body)
	if err != nil {
		return nil, &ProviderError{Operation: "request", Err: err}
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, &ProviderError{Operation: "request", Retryable: ctx.Err() == nil, Err: err}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxTranscriptionResponseBytes+1))
	if err != nil {
		return nil, &ProviderError{Operation: "response", Retryable: true, StatusCode: resp.StatusCode, Err: err}
	}
	if int64(len(raw)) > maxTranscriptionResponseBytes {
		return nil, &ProviderError{Operation: "response", StatusCode: resp.StatusCode, Err: errors.New("response body too large")}
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		message := strings.TrimSpace(string(raw))
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return nil, &ProviderError{
			Operation:  "request",
			StatusCode: resp.StatusCode,
			Retryable:  resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError,
			Err:        errors.New(message),
		}
	}

	var payload openAITranscriptionResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, &ProviderError{Operation: "decode", StatusCode: resp.StatusCode, Err: err}
	}
	language := strings.TrimSpace(payload.Language)
	if language == "" {
		language = p.language
	}
	segments := make([]Segment, 0, len(payload.Segments))
	for _, item := range payload.Segments {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		confidence := 0.0
		if item.Confidence != nil {
			confidence = clampConfidence(*item.Confidence)
		} else if item.AvgLogProb != nil {
			confidence = clampConfidence(math.Exp(*item.AvgLogProb))
		}
		segments = append(segments, Segment{
			Language:   language,
			Text:       text,
			StartMS:    secondsToMilliseconds(item.Start),
			EndMS:      secondsToMilliseconds(item.End),
			Confidence: confidence,
		})
	}
	if len(segments) == 0 && strings.TrimSpace(payload.Text) != "" {
		segments = append(segments, Segment{
			Language: language,
			Text:     strings.TrimSpace(payload.Text),
			StartMS:  0,
			EndMS:    secondsToMilliseconds(payload.Duration),
		})
	}
	if len(segments) == 0 {
		return nil, &ProviderError{Operation: "decode", StatusCode: resp.StatusCode, Err: errors.New("provider returned no transcript text")}
	}
	return segments, nil
}

func secondsToMilliseconds(value float64) int64 {
	if value <= 0 {
		return 0
	}
	return int64(math.Round(value * 1000))
}

func clampConfidence(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
