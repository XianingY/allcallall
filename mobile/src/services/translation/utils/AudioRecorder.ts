// mobile/src/services/translation/utils/AudioRecorder.ts
// Using expo-av instead of react-native-audio-recorder-player for RN 0.74 compatibility
import { Audio } from 'expo-av';
import { Platform, PermissionsAndroid } from 'react-native';
import * as FileSystem from 'expo-file-system';

export interface AudioRecordingResult {
    audioData: Float32Array;
    durationMs: number;
    sampleRate: number;
}

class AudioRecorder {
    private recording: Audio.Recording | null = null;
    private isRecording = false;

    /**
     * Request audio recording permissions
     */
    async requestPermissions(): Promise<boolean> {
        try {
            const { status } = await Audio.requestPermissionsAsync();
            if (status !== 'granted') {
                console.error('[AudioRecorder] Permission denied');
                return false;
            }

            // Configure audio mode for recording
            await Audio.setAudioModeAsync({
                allowsRecordingIOS: true,
                playsInSilentModeIOS: true,
            });

            return true;
        } catch (err) {
            console.error('[AudioRecorder] Permission request error:', err);
            return false;
        }
    }

    /**
     * Start audio recording
     */
    async startRecording(): Promise<void> {
        if (this.isRecording) {
            console.warn('[AudioRecorder] Already recording');
            return;
        }

        const hasPermission = await this.requestPermissions();
        if (!hasPermission) {
            throw new Error('Audio recording permission denied');
        }

        try {
            // Create recording with settings optimized for speech recognition
            const { recording } = await Audio.Recording.createAsync({
                android: {
                    extension: '.wav',
                    outputFormat: Audio.AndroidOutputFormat.DEFAULT,
                    audioEncoder: Audio.AndroidAudioEncoder.DEFAULT,
                    sampleRate: 16000,
                    numberOfChannels: 1,
                    bitRate: 256000,
                },
                ios: {
                    extension: '.wav',
                    audioQuality: Audio.IOSAudioQuality.HIGH,
                    sampleRate: 16000,
                    numberOfChannels: 1,
                    bitRate: 256000,
                    linearPCMBitDepth: 16,
                    linearPCMIsBigEndian: false,
                    linearPCMIsFloat: false,
                },
                web: {}
            });

            this.recording = recording;
            this.isRecording = true;
            console.log('[AudioRecorder] Recording started');
        } catch (error) {
            console.error('[AudioRecorder] Failed to start recording:', error);
            throw error;
        }
    }

    /**
     * Stop recording and return audio data
     */
    async stopRecording(): Promise<AudioRecordingResult> {
        if (!this.isRecording || !this.recording) {
            console.warn('[AudioRecorder] Not recording');
            return {
                audioData: new Float32Array(0),
                durationMs: 0,
                sampleRate: 16000
            };
        }

        try {
            await this.recording.stopAndUnloadAsync();
            const uri = this.recording.getURI();
            const status = await this.recording.getStatusAsync();

            this.isRecording = false;
            console.log('[AudioRecorder] Recording stopped:', uri);

            if (!uri) {
                return {
                    audioData: new Float32Array(0),
                    durationMs: 0,
                    sampleRate: 16000
                };
            }

            // Read and convert the audio file
            const audioData = await this.readWavFile(uri);

            // Clean up the recording
            this.recording = null;

            // Delete the temp file
            try {
                await FileSystem.deleteAsync(uri, { idempotent: true });
            } catch (e) {
                console.warn('[AudioRecorder] Failed to delete temp file:', e);
            }

            return {
                audioData,
                durationMs: status.durationMillis || (audioData.length / 16000) * 1000,
                sampleRate: 16000
            };
        } catch (error) {
            console.error('[AudioRecorder] Failed to stop recording:', error);
            this.recording = null;
            this.isRecording = false;
            throw error;
        }
    }

    /**
     * Record audio for a specified duration
     */
    async recordChunk(durationMs: number): Promise<Float32Array> {
        await this.startRecording();

        return new Promise((resolve, reject) => {
            setTimeout(async () => {
                try {
                    const result = await this.stopRecording();
                    resolve(result.audioData);
                } catch (error) {
                    reject(error);
                }
            }, durationMs);
        });
    }

    /**
     * Check if currently recording
     */
    isCurrentlyRecording(): boolean {
        return this.isRecording;
    }

    /**
     * Read WAV file and convert to Float32Array
     */
    private async readWavFile(uri: string): Promise<Float32Array> {
        try {
            // Read file as base64
            const base64Data = await FileSystem.readAsStringAsync(uri, {
                encoding: FileSystem.EncodingType.Base64
            });

            // Decode base64 to bytes
            const binaryString = atob(base64Data);
            const bytes = new Uint8Array(binaryString.length);
            for (let i = 0; i < binaryString.length; i++) {
                bytes[i] = binaryString.charCodeAt(i);
            }

            // Parse WAV header (skip first 44 bytes for standard WAV)
            const headerSize = 44;
            if (bytes.length <= headerSize) {
                return new Float32Array(0);
            }

            const audioBytes = bytes.slice(headerSize);

            // Convert 16-bit PCM to Float32 (-1.0 to 1.0)
            const samples = new Float32Array(Math.floor(audioBytes.length / 2));
            const dataView = new DataView(audioBytes.buffer, audioBytes.byteOffset, audioBytes.byteLength);

            for (let i = 0; i < samples.length; i++) {
                const int16 = dataView.getInt16(i * 2, true); // little-endian
                samples[i] = int16 / 32768.0;
            }

            return samples;
        } catch (error) {
            console.error('[AudioRecorder] Failed to read WAV file:', error);
            return new Float32Array(0);
        }
    }
}

export default new AudioRecorder();
