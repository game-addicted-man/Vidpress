package engine

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// FormatSize renders a byte count like Windows Explorer does (1 KB = 1024 B).
func FormatSize(n int64) string {
	if n < 0 {
		return "-"
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	f := float64(n)
	for _, u := range []string{"KB", "MB", "GB", "TB"} {
		f /= unit
		if f < unit || u == "TB" {
			switch {
			case f >= 100:
				return fmt.Sprintf("%.0f %s", f, u)
			case f >= 10:
				return fmt.Sprintf("%.1f %s", f, u)
			default:
				return fmt.Sprintf("%.2f %s", f, u)
			}
		}
	}
	return fmt.Sprintf("%d B", n)
}

// FormatDuration renders seconds as "1:02:03" or "2:03".
func FormatDuration(sec float64) string {
	if sec <= 0 || math.IsNaN(sec) || math.IsInf(sec, 0) {
		return "-"
	}
	t := int(math.Round(sec))
	h, m, s := t/3600, (t/60)%60, t%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// FormatETA renders a remaining time in Korean, e.g. "약 3분 20초".
func FormatETA(d time.Duration) string {
	if d <= 0 {
		return "곧 끝남"
	}
	t := int(d.Round(time.Second).Seconds())
	h, m, s := t/3600, (t/60)%60, t%60
	switch {
	case h > 0:
		return fmt.Sprintf("약 %d시간 %d분", h, m)
	case m >= 10:
		return fmt.Sprintf("약 %d분", m)
	case m > 0:
		return fmt.Sprintf("약 %d분 %d초", m, s)
	default:
		return fmt.Sprintf("약 %d초", s)
	}
}

// SavingText describes the size change, e.g. "120 MB → 35.2 MB (71% 감소)".
func SavingText(before, after int64) string {
	if before <= 0 || after < 0 {
		return FormatSize(after)
	}
	pct := (1 - float64(after)/float64(before)) * 100
	switch {
	case pct >= 0.5:
		return fmt.Sprintf("%s → %s (%.0f%% 감소)", FormatSize(before), FormatSize(after), pct)
	case pct <= -0.5:
		return fmt.Sprintf("%s → %s (%.0f%% 증가)", FormatSize(before), FormatSize(after), -pct)
	default:
		return fmt.Sprintf("%s → %s (비슷함)", FormatSize(before), FormatSize(after))
	}
}

// VideoExtensions are the file types picked up by "폴더 추가" and drag & drop
// of folders.
var VideoExtensions = map[string]bool{
	".mp4": true, ".m4v": true, ".mov": true, ".mkv": true, ".avi": true, ".wmv": true,
	".flv": true, ".webm": true, ".ts": true, ".mts": true, ".m2ts": true, ".mpg": true,
	".mpeg": true, ".3gp": true, ".3g2": true, ".ogv": true, ".vob": true, ".asf": true,
	".f4v": true, ".rm": true, ".rmvb": true, ".divx": true, ".mxf": true, ".dv": true,
	".gif": true, ".hevc": true, ".h264": true, ".264": true, ".265": true, ".y4m": true,
	// audio files can be converted too (e.g. to MP3)
	".mp3": true, ".m4a": true, ".wav": true, ".flac": true, ".aac": true, ".ogg": true,
	".opus": true, ".wma": true, ".ac3": true, ".mka": true, ".aiff": true, ".aif": true,
}

// IsMediaFile reports whether the extension looks like a media file.
func IsMediaFile(path string) bool {
	return VideoExtensions[strings.ToLower(filepath.Ext(path))]
}

// OutputPath builds the destination path for input, avoiding existing files,
// the input itself and names already reserved by this batch.
func OutputPath(input, outDir, suffix, ext string, reserved map[string]bool) string {
	dir := outDir
	if dir == "" {
		dir = filepath.Dir(input)
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	base := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
	suffix = sanitizeName(suffix)
	name := base + suffix
	inAbs, _ := filepath.Abs(input)
	for i := 1; ; i++ {
		cand := name
		if i > 1 {
			cand = fmt.Sprintf("%s (%d)", name, i)
		}
		p := filepath.Join(dir, cand+ext)
		abs, _ := filepath.Abs(p)
		if strings.EqualFold(abs, inAbs) || reserved[strings.ToLower(abs)] {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			continue
		}
		if _, err := os.Stat(PartPath(p)); err == nil {
			continue
		}
		return p
	}
}

// PartPath is the temporary file written while converting.
func PartPath(final string) string {
	ext := filepath.Ext(final)
	return strings.TrimSuffix(final, ext) + ".변환중" + ext
}

func sanitizeName(s string) string {
	repl := strings.NewReplacer("\\", "", "/", "", ":", "", "*", "", "?", "", "\"", "", "<", "", ">", "", "|", "")
	return strings.TrimRight(repl.Replace(s), ". ")
}

var ffLogPrefix = regexp.MustCompile(`\[[^\]]+ @ (0x)?[0-9a-fA-F]+\] `)

// friendlyFFmpegError strips FFmpeg's internal prefixes and adds a short
// Korean explanation for the most common problems.
func friendlyFFmpegError(msg string) string {
	msg = strings.TrimSpace(ffLogPrefix.ReplaceAllString(msg, ""))
	lower := strings.ToLower(msg)
	hint := ""
	switch {
	case strings.Contains(lower, "no space left"):
		hint = "디스크 공간이 부족합니다."
	case strings.Contains(lower, "permission denied"):
		hint = "저장할 권한이 없습니다. 다른 폴더를 고르거나, 파일이 다른 프로그램에서 열려 있는지 확인하세요."
	case strings.Contains(lower, "no such file or directory"):
		hint = "파일이나 폴더를 찾을 수 없습니다."
	case strings.Contains(lower, "invalid data found"), strings.Contains(lower, "moov atom not found"):
		hint = "파일이 손상되었거나 끝까지 저장되지 않은 파일입니다."
	case strings.Contains(lower, "error while opening encoder"), strings.Contains(lower, "could not open encoder"),
		strings.Contains(lower, "error initializing output stream"):
		hint = "이 설정으로 인코더를 시작할 수 없습니다. 다른 출력 형식이나 '추천' 프리셋으로 다시 해 보세요."
	case strings.Contains(lower, "could not find tag for codec"), strings.Contains(lower, "not supported in container"),
		strings.Contains(lower, "codec not currently supported in container"):
		hint = "이 형식(컨테이너)에 넣을 수 없는 코덱입니다. MKV나 다른 형식을 골라 보세요."
	}
	if hint != "" {
		return hint + " (" + msg + ")"
	}
	return msg
}
