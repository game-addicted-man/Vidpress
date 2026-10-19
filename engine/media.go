package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// VideoStream describes the main picture of a file.
type VideoStream struct {
	Index          int // absolute stream index for -map 0:N
	Codec          string
	Profile        string
	Width, Height  int // coded size
	DispW, DispH   int // size after applying rotation
	FPS            float64
	PixFmt         string
	BitDepth       int
	FieldOrder     string
	ColorTransfer  string
	ColorPrimaries string
	ColorSpace     string
	ColorRange     string
	Rotation       int
	BitRate        int64
}

// Interlaced reports whether the stream is flagged as interlaced.
func (v *VideoStream) Interlaced() bool {
	switch v.FieldOrder {
	case "tt", "bb", "tb", "bt":
		return true
	}
	return false
}

// HDR reports whether the stream uses an HDR transfer function (PQ or HLG).
func (v *VideoStream) HDR() bool {
	return v.ColorTransfer == "smpte2084" || v.ColorTransfer == "arib-std-b67"
}

// AudioStream describes one audio track.
type AudioStream struct {
	Index      int
	Codec      string
	Channels   int
	SampleRate int
	BitRate    int64
	Language   string
}

// SubStream describes one subtitle track.
type SubStream struct {
	Index    int
	Codec    string
	Language string
}

// TextBased reports whether the subtitle can be converted to another text format.
func (s SubStream) TextBased() bool {
	switch s.Codec {
	case "subrip", "srt", "ass", "ssa", "webvtt", "mov_text", "text", "microdvd", "subviewer", "subviewer1", "sami", "realtext", "stl", "mpl2", "jacosub", "vplayer", "pjs":
		return true
	}
	return false
}

// MediaInfo is the subset of ffprobe output VidPress needs.
type MediaInfo struct {
	Path       string
	Size       int64
	Duration   float64 // seconds; 0 when unknown
	FormatName string
	BitRate    int64
	Video      *VideoStream // nil for audio-only files
	Audio      []AudioStream
	Subs       []SubStream
}

// Resolution returns a short "1920×1080" style description.
func (m *MediaInfo) ResolutionText() string {
	if m.Video == nil {
		return "-"
	}
	return fmt.Sprintf("%d×%d", m.Video.DispW, m.Video.DispH)
}

// CodecText returns a short codec description, e.g. "H.264 / AAC".
func (m *MediaInfo) CodecText() string {
	var parts []string
	if m.Video != nil {
		parts = append(parts, prettyCodec(m.Video.Codec))
	}
	if len(m.Audio) > 0 {
		parts = append(parts, prettyCodec(m.Audio[0].Codec))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, " / ")
}

func prettyCodec(c string) string {
	switch c {
	case "h264":
		return "H.264"
	case "hevc":
		return "H.265"
	case "av1":
		return "AV1"
	case "vp9":
		return "VP9"
	case "vp8":
		return "VP8"
	case "mpeg4":
		return "MPEG-4"
	case "mpeg2video":
		return "MPEG-2"
	case "mpeg1video":
		return "MPEG-1"
	case "wmv3", "wmv2", "wmv1":
		return "WMV"
	case "vc1":
		return "VC-1"
	case "prores":
		return "ProRes"
	case "aac":
		return "AAC"
	case "mp3":
		return "MP3"
	case "ac3":
		return "AC3"
	case "eac3":
		return "E-AC3"
	case "opus":
		return "Opus"
	case "vorbis":
		return "Vorbis"
	case "flac":
		return "FLAC"
	case "alac":
		return "ALAC"
	case "dts":
		return "DTS"
	case "truehd":
		return "TrueHD"
	case "wmav2", "wmav1", "wmapro":
		return "WMA"
	}
	if strings.HasPrefix(c, "pcm_") {
		return "PCM"
	}
	return strings.ToUpper(c)
}

type probeStream struct {
	Index          int               `json:"index"`
	CodecType      string            `json:"codec_type"`
	CodecName      string            `json:"codec_name"`
	Profile        string            `json:"profile"`
	Width          int               `json:"width"`
	Height         int               `json:"height"`
	PixFmt         string            `json:"pix_fmt"`
	FieldOrder     string            `json:"field_order"`
	ColorTransfer  string            `json:"color_transfer"`
	ColorPrimaries string            `json:"color_primaries"`
	ColorSpace     string            `json:"color_space"`
	ColorRange     string            `json:"color_range"`
	RFrameRate     string            `json:"r_frame_rate"`
	AvgFrameRate   string            `json:"avg_frame_rate"`
	BitRate        string            `json:"bit_rate"`
	BitsPerRaw     string            `json:"bits_per_raw_sample"`
	Channels       int               `json:"channels"`
	SampleRate     string            `json:"sample_rate"`
	Duration       string            `json:"duration"`
	Disposition    map[string]int    `json:"disposition"`
	Tags           map[string]string `json:"tags"`
	SideDataList   []map[string]any  `json:"side_data_list"`
}

type probeOutput struct {
	Streams []probeStream `json:"streams"`
	Format  struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
		Size       string `json:"size"`
		BitRate    string `json:"bit_rate"`
	} `json:"format"`
}

// Probe runs ffprobe on path.
func Probe(ctx context.Context, ffprobe, path string) (*MediaInfo, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("파일을 열 수 없습니다: %w", err)
	}
	if st.IsDir() {
		return nil, errors.New("폴더입니다")
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffprobe, "-v", "error", "-hide_banner",
		"-print_format", "json", "-show_format", "-show_streams", path)
	setProcAttr(cmd, false)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := lastLines(stderr.String(), 2)
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("동영상 정보를 읽을 수 없습니다: %s", msg)
	}
	info, err := parseProbe(stdout.Bytes())
	if err != nil {
		return nil, err
	}
	info.Path = path
	if info.Size == 0 {
		info.Size = st.Size()
	}
	if info.Video == nil && len(info.Audio) == 0 {
		return nil, errors.New("영상이나 소리가 들어 있지 않은 파일입니다")
	}
	return info, nil
}

func parseProbe(data []byte) (*MediaInfo, error) {
	var p probeOutput
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("ffprobe 결과를 해석할 수 없습니다: %w", err)
	}
	info := &MediaInfo{
		FormatName: p.Format.FormatName,
		Duration:   parseFloat(p.Format.Duration),
		Size:       parseInt(p.Format.Size),
		BitRate:    parseInt(p.Format.BitRate),
	}
	for _, s := range p.Streams {
		switch s.CodecType {
		case "video":
			if s.Disposition["attached_pic"] == 1 || info.Video != nil {
				continue
			}
			// Still-image "video" streams (cover art without the attached_pic
			// flag) have no real frame rate. Animated GIFs and MJPEG videos do.
			switch s.CodecName {
			case "", "png", "bmp", "gif", "webp", "mjpeg", "jpegls", "tiff":
				if parseRate(s.AvgFrameRate) <= 1 {
					continue
				}
			}
			v := &VideoStream{
				Index:          s.Index,
				Codec:          s.CodecName,
				Profile:        s.Profile,
				Width:          s.Width,
				Height:         s.Height,
				PixFmt:         s.PixFmt,
				FieldOrder:     s.FieldOrder,
				ColorTransfer:  s.ColorTransfer,
				ColorPrimaries: s.ColorPrimaries,
				ColorSpace:     s.ColorSpace,
				ColorRange:     s.ColorRange,
				BitRate:        parseInt(s.BitRate),
			}
			v.FPS = parseRate(s.AvgFrameRate)
			if v.FPS <= 0 || v.FPS > 1000 {
				v.FPS = parseRate(s.RFrameRate)
			}
			v.BitDepth = int(parseInt(s.BitsPerRaw))
			if v.BitDepth == 0 {
				v.BitDepth = bitDepthFromPixFmt(s.PixFmt)
			}
			v.Rotation = rotationOf(s)
			v.DispW, v.DispH = v.Width, v.Height
			if v.Rotation == 90 || v.Rotation == 270 {
				v.DispW, v.DispH = v.Height, v.Width
			}
			if info.Duration <= 0 {
				info.Duration = parseFloat(s.Duration)
			}
			info.Video = v
		case "audio":
			info.Audio = append(info.Audio, AudioStream{
				Index:      s.Index,
				Codec:      s.CodecName,
				Channels:   s.Channels,
				SampleRate: int(parseInt(s.SampleRate)),
				BitRate:    parseInt(s.BitRate),
				Language:   s.Tags["language"],
			})
			if info.Duration <= 0 {
				info.Duration = parseFloat(s.Duration)
			}
		case "subtitle":
			info.Subs = append(info.Subs, SubStream{Index: s.Index, Codec: s.CodecName, Language: s.Tags["language"]})
		}
	}
	return info, nil
}

func rotationOf(s probeStream) int {
	rot := 0.0
	found := false
	for _, sd := range s.SideDataList {
		if r, ok := sd["rotation"]; ok {
			switch x := r.(type) {
			case float64:
				rot, found = x, true
			case string:
				rot, found = parseFloat(x), true
			}
		}
	}
	if !found {
		if r, ok := s.Tags["rotate"]; ok {
			rot = parseFloat(r)
		}
	}
	n := int(math.Round(rot)) % 360
	if n < 0 {
		n += 360
	}
	// Normalise to the nearest right angle.
	return ((n + 45) / 90 % 4) * 90
}

func bitDepthFromPixFmt(pf string) int {
	switch {
	case strings.Contains(pf, "p10"), strings.Contains(pf, "10le"), strings.Contains(pf, "10be"):
		return 10
	case strings.Contains(pf, "p12"), strings.Contains(pf, "12le"), strings.Contains(pf, "12be"):
		return 12
	case pf == "":
		return 0
	}
	return 8
}

func parseRate(s string) float64 {
	if s == "" {
		return 0
	}
	if i := strings.IndexByte(s, '/'); i >= 0 {
		n, d := parseFloat(s[:i]), parseFloat(s[i+1:])
		if d == 0 {
			return 0
		}
		return n / d
	}
	return parseFloat(s)
}

func parseFloat(s string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return f
}

func parseInt(s string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return int64(parseFloat(s))
	}
	return n
}

// lastLines returns the last n non-empty lines of s, joined by " / ".
func lastLines(s string, n int) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r", "\n"), "\n")
	var out []string
	for i := len(lines) - 1; i >= 0 && len(out) < n; i-- {
		l := strings.TrimSpace(lines[i])
		if l != "" {
			out = append([]string{l}, out...)
		}
	}
	return strings.Join(out, " / ")
}
