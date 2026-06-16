package audio

import (
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"runtime"
)

const (
	sampleRate = 44100
)

// PlayBeeNotification plays a short bee-like buzz sound.
// It is best-effort and silently no-ops if audio cannot initialize.
func PlayBeeNotification() {
	go playBestEffort()
}

func playBestEffort() {
	if runtime.GOOS == "windows" {
		_ = exec.Command("powershell", "-NoProfile", "-Command", "[console]::beep(740,120);[console]::beep(620,120)").Run()
		return
	}

	wav := buildBeeWAV()
	tmp, err := os.CreateTemp("", "beez-*.wav")
	if err != nil {
		return
	}
	path := tmp.Name()
	_, _ = tmp.Write(wav)
	_ = tmp.Close()
	defer os.Remove(path)

	var candidates [][]string
	switch runtime.GOOS {
	case "darwin":
		candidates = [][]string{{"afplay", path}}
	default: // linux / unix
		candidates = [][]string{
			{"paplay", path},
			{"pw-play", path},
			{"aplay", path},
		}
	}
	for _, c := range candidates {
		if _, err := exec.LookPath(c[0]); err == nil {
			_ = exec.Command(c[0], c[1:]...).Run()
			return
		}
	}
}

func buildBeeWAV() []byte {
	seconds := 0.28
	total := int(float64(sampleRate) * seconds)
	pcm := make([]byte, total*2)

	for i := 0; i < total; i++ {
		t := float64(i) / sampleRate

		// Quick attack/release envelope to feel like a short "bee buzz".
		env := 1.0
		attack := 0.02
		release := 0.08
		if t < attack {
			env = t / attack
		}
		remaining := seconds - t
		if remaining < release {
			env *= remaining / release
		}
		if env < 0 {
			env = 0
		}

		base := 220.0 + 45.0*math.Sin(2*math.Pi*8*t)
		sig := 0.55*math.Sin(2*math.Pi*base*t) + 0.25*math.Sin(2*math.Pi*(base*2.03)*t)
		val := env * sig
		if val > 1 {
			val = 1
		}
		if val < -1 {
			val = -1
		}

		s := int16(val * 32000)
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(s))
	}

	return wrapWAV(pcm, sampleRate, 1, 16)
}

func wrapWAV(pcm []byte, sampleRate, channels, bitsPerSample int) []byte {
	dataLen := len(pcm)
	byteRate := sampleRate * channels * bitsPerSample / 8
	blockAlign := channels * bitsPerSample / 8
	riffLen := 36 + dataLen

	out := make([]byte, 44+dataLen)
	copy(out[0:4], []byte("RIFF"))
	binary.LittleEndian.PutUint32(out[4:8], uint32(riffLen))
	copy(out[8:12], []byte("WAVE"))
	copy(out[12:16], []byte("fmt "))
	binary.LittleEndian.PutUint32(out[16:20], 16) // PCM chunk size
	binary.LittleEndian.PutUint16(out[20:22], 1)  // PCM format
	binary.LittleEndian.PutUint16(out[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(out[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(out[28:32], uint32(byteRate))
	binary.LittleEndian.PutUint16(out[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(out[34:36], uint16(bitsPerSample))
	copy(out[36:40], []byte("data"))
	binary.LittleEndian.PutUint32(out[40:44], uint32(dataLen))
	copy(out[44:], pcm)
	return out
}
