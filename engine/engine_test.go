package engine

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// These tests generate small sample videos with the ffmpeg found on PATH and
// run real conversions through the engine. They are skipped when ffmpeg is
// not installed.

var (
	sampleDir string
	tools     Tools
)

func TestMain(m *testing.M) {
	t, err := FindTools("")
	if err != nil {
		fmt.Println("ffmpeg not found, skipping engine tests")
		os.Exit(0)
	}
	tools = t
	dir, err := os.MkdirTemp("", "vidpress-samples-")
	if err != nil {
		panic(err)
	}
	sampleDir = dir
	if err := makeSamples(dir); err != nil {
		fmt.Println("sample generation failed:", err)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	if os.Getenv("VIDPRESS_KEEP_SAMPLES") == "" {
		os.RemoveAll(dir)
	} else {
		fmt.Println("samples kept in", dir)
	}
	os.Exit(code)
}

func ff(args ...string) error {
	cmd := exec.Command(tools.FFmpeg, append([]string{"-hide_banner", "-loglevel", "error", "-y"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, out)
	}
	return nil
}

func sample(name string) string { return filepath.Join(sampleDir, name) }

func makeSamples(dir string) error {
	srt := filepath.Join(dir, "subs.srt")
	if err := os.WriteFile(srt, []byte("1\n00:00:00,500 --> 00:00:02,000\n안녕하세요 자막 테스트\n\n2\n00:00:02,500 --> 00:00:03,500\nSecond line\n"), 0o644); err != nil {
		return err
	}
	steps := [][]string{
		{"-f", "lavfi", "-i", "testsrc2=s=1920x1080:r=30:d=6", "-f", "lavfi", "-i", "sine=f=440:d=6:sample_rate=48000",
			"-ac", "2", "-c:v", "libx264", "-preset", "ultrafast", "-crf", "16", "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "192k", "-shortest", sample("h264_1080.mp4")},
		{"-f", "lavfi", "-i", "testsrc2=s=1080x1920:r=30:d=3", "-f", "lavfi", "-i", "sine=d=3",
			"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", sample("portrait.mp4")},
		{"-display_rotation", "90", "-i", sample("h264_1080.mp4"), "-c", "copy", sample("rotated.mp4")},
		{"-f", "lavfi", "-i", "testsrc2=s=853x479:r=25:d=3", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv444p", sample("odd444.mp4")},
		{"-f", "lavfi", "-i", "testsrc2=s=1280x720:r=24:d=3", "-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le",
			"-vf", "setparams=color_primaries=bt2020:color_trc=smpte2084:colorspace=bt2020nc",
			"-x265-params", "colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc:log-level=error", sample("hdr10.mkv")},
		{"-f", "lavfi", "-i", "testsrc2=s=720x480:r=30000/1001:d=3", "-f", "lavfi", "-i", "sine=d=3",
			"-c:v", "mpeg2video", "-flags", "+ildct+ilme", "-top", "1", "-b:v", "4M", "-c:a", "mp2", "-shortest", sample("interlaced.mpg")},
		{"-f", "lavfi", "-i", "testsrc2=s=1280x720:r=25:d=4", "-f", "lavfi", "-i", "sine=f=300:d=4", "-i", srt,
			"-map", "0", "-map", "1", "-map", "2", "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "ac3", "-ac", "6", "-c:s", "srt", "-shortest", sample("surround_subs.mkv")},
		{"-f", "lavfi", "-i", "testsrc2=s=640x360:r=30:d=3", "-f", "lavfi", "-i", "sine=d=3",
			"-c:v", "libvpx-vp9", "-deadline", "realtime", "-cpu-used", "8", "-b:v", "400k", "-c:a", "libopus", "-shortest", sample("vp9.webm")},
		{"-f", "lavfi", "-i", "color=c=red:s=300x300", "-frames:v", "1", sample("cover.jpg")},
		{"-f", "lavfi", "-i", "sine=f=500:d=5:sample_rate=44100", "-i", sample("cover.jpg"),
			"-map", "0:a", "-map", "1:v", "-c:a", "libmp3lame", "-b:a", "128k", "-c:v", "copy", "-disposition:v", "attached_pic", sample("song_cover.mp3")},
		{"-f", "lavfi", "-i", "testsrc2=s=640x480:r=25:d=3", "-f", "lavfi", "-i", "sine=d=3",
			"-c:v", "mpeg4", "-vtag", "XVID", "-q:v", "4", "-c:a", "libmp3lame", "-shortest", sample("xvid.avi")},
		{"-f", "lavfi", "-i", "testsrc2=s=1280x720:r=30:d=3", "-f", "lavfi", "-i", "sine=d=3",
			"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-c:a", "pcm_s16le", "-shortest", sample("pcm.mov")},
		{"-f", "lavfi", "-i", "testsrc2=s=640x480:r=25:d=3", "-f", "lavfi", "-i", "sine=d=3",
			"-c:v", "wmv2", "-b:v", "1M", "-c:a", "wmav2", "-shortest", sample("old.wmv")},
		{"-f", "lavfi", "-i", "testsrc2=s=1920x1080:r=30:d=20", "-c:v", "libx264", "-preset", "ultrafast", "-crf", "20", sample("long_1080.mp4")},
	}
	for _, s := range steps {
		if err := ff(s...); err != nil {
			return fmt.Errorf("%s: %w", s[len(s)-1], err)
		}
	}
	return nil
}

func probe(t *testing.T, path string) *MediaInfo {
	t.Helper()
	info, err := Probe(context.Background(), tools.FFprobe, path)
	if err != nil {
		t.Fatalf("probe %s: %v", filepath.Base(path), err)
	}
	return info
}

func TestProbe(t *testing.T) {
	m := probe(t, sample("h264_1080.mp4"))
	if m.Video == nil || m.Video.Codec != "h264" || m.Video.DispW != 1920 || m.Video.DispH != 1080 {
		t.Fatalf("unexpected video: %+v", m.Video)
	}
	if math.Abs(m.Duration-6) > 0.2 || len(m.Audio) != 1 || m.Audio[0].Codec != "aac" {
		t.Fatalf("unexpected info: dur=%v audio=%+v", m.Duration, m.Audio)
	}
	if m.Video.FPS < 29.9 || m.Video.FPS > 30.1 {
		t.Fatalf("fps %v", m.Video.FPS)
	}

	r := probe(t, sample("rotated.mp4"))
	if r.Video.Rotation != 90 && r.Video.Rotation != 270 {
		t.Fatalf("rotation not detected: %d", r.Video.Rotation)
	}
	if r.Video.DispW != 1080 || r.Video.DispH != 1920 {
		t.Fatalf("rotated display size %dx%d", r.Video.DispW, r.Video.DispH)
	}

	h := probe(t, sample("hdr10.mkv"))
	if !h.Video.HDR() || h.Video.BitDepth != 10 {
		t.Fatalf("HDR not detected: %+v", h.Video)
	}

	i := probe(t, sample("interlaced.mpg"))
	if !i.Video.Interlaced() {
		t.Fatalf("interlace not detected: %q", i.Video.FieldOrder)
	}

	s := probe(t, sample("surround_subs.mkv"))
	if len(s.Subs) != 1 || !s.Subs[0].TextBased() || s.Audio[0].Channels != 6 {
		t.Fatalf("subs/audio: %+v %+v", s.Subs, s.Audio)
	}

	c := probe(t, sample("song_cover.mp3"))
	if c.Video != nil {
		t.Fatalf("cover art must not count as video: %+v", c.Video)
	}
	if len(c.Audio) != 1 {
		t.Fatalf("audio streams: %d", len(c.Audio))
	}
}

type convCase struct {
	name     string
	input    string
	settings func(s *Settings)
	check    func(t *testing.T, in, out *MediaInfo, res *Result)
}

func runCase(t *testing.T, c convCase) (*MediaInfo, *Result) {
	t.Helper()
	in := probe(t, c.input)
	s := DefaultSettings()
	s.Speed = SpeedFast
	c.settings(&s)
	outDir := t.TempDir()
	final := OutputPath(c.input, outDir, s.Suffix, s.Format().Ext, nil)
	conv := &Converter{Tools: tools, LogCommand: func(a []string) { t.Logf("ffmpeg %s", strings.Join(a, " ")) }}
	var last Progress
	var notes []string
	res, err := conv.Convert(context.Background(), in, s, final, func(p Progress) { last = p }, func(n string) { notes = append(notes, n) })
	if err != nil {
		t.Fatalf("%s: convert failed: %v (notes %v)", c.name, err, notes)
	}
	if _, err := os.Stat(PartPath(final)); err == nil {
		t.Fatalf("part file left behind")
	}
	out := probe(t, res.Output)
	if in.Duration > 0 && out.Duration > 0 && math.Abs(in.Duration-out.Duration) > 0.6 {
		t.Errorf("%s: duration changed %.2f -> %.2f", c.name, in.Duration, out.Duration)
	}
	if last.Fraction < 0.9 && in.Duration > 0 {
		t.Errorf("%s: final progress %.2f", c.name, last.Fraction)
	}
	t.Logf("%s: %s, notes=%v, encoder=%s", c.name, SavingText(in.Size, res.Size), notes, res.Encoder)
	if c.check != nil {
		c.check(t, in, out, res)
	}
	return out, res
}

func wantVideo(t *testing.T, out *MediaInfo, codec string, w, h int) {
	t.Helper()
	if out.Video == nil {
		t.Fatalf("no video in output")
	}
	if out.Video.Codec != codec {
		t.Errorf("codec %s, want %s", out.Video.Codec, codec)
	}
	if w > 0 && (out.Video.DispW != w || out.Video.DispH != h) {
		t.Errorf("size %dx%d, want %dx%d", out.Video.DispW, out.Video.DispH, w, h)
	}
}

func TestConversions(t *testing.T) {
	cases := []convCase{
		{"recommended h264", sample("h264_1080.mp4"), func(s *Settings) { Presets[0].Apply(s) },
			func(t *testing.T, in, out *MediaInfo, r *Result) {
				wantVideo(t, out, "h264", 1920, 1080)
				if out.Video.PixFmt != "yuv420p" || len(out.Audio) != 1 || out.Audio[0].Codec != "aac" {
					t.Errorf("pix/audio: %s %+v", out.Video.PixFmt, out.Audio)
				}
			}},
		{"hevc 720p preset", sample("h264_1080.mp4"), func(s *Settings) { Presets[2].Apply(s) },
			func(t *testing.T, in, out *MediaInfo, r *Result) { wantVideo(t, out, "hevc", 1280, 720) }},
		{"portrait 720p keeps orientation", sample("portrait.mp4"), func(s *Settings) { Presets[3].Apply(s) },
			func(t *testing.T, in, out *MediaInfo, r *Result) { wantVideo(t, out, "h264", 720, 1280) }},
		{"rotated phone video", sample("rotated.mp4"), func(s *Settings) { Presets[3].Apply(s) },
			func(t *testing.T, in, out *MediaInfo, r *Result) { wantVideo(t, out, "h264", 720, 1280) }},
		{"odd size 4:4:4", sample("odd444.mp4"), func(s *Settings) { Presets[0].Apply(s) },
			func(t *testing.T, in, out *MediaInfo, r *Result) {
				wantVideo(t, out, "h264", 852, 478)
				if out.Video.PixFmt != "yuv420p" {
					t.Errorf("pix %s", out.Video.PixFmt)
				}
			}},
		{"hdr tonemap to h264", sample("hdr10.mkv"), func(s *Settings) { Presets[0].Apply(s) },
			func(t *testing.T, in, out *MediaInfo, r *Result) {
				wantVideo(t, out, "h264", 1280, 720)
				if out.Video.HDR() || out.Video.ColorTransfer != "bt709" {
					t.Errorf("still HDR: trc=%s", out.Video.ColorTransfer)
				}
			}},
		{"hdr keep 10-bit hevc", sample("hdr10.mkv"), func(s *Settings) { Presets[1].Apply(s); s.ToneMapHDR = false },
			func(t *testing.T, in, out *MediaInfo, r *Result) {
				wantVideo(t, out, "hevc", 1280, 720)
				if !out.Video.HDR() || out.Video.BitDepth != 10 {
					t.Errorf("HDR lost: %+v", out.Video)
				}
			}},
		{"interlaced mpeg2 deinterlace", sample("interlaced.mpg"), func(s *Settings) { Presets[0].Apply(s) },
			func(t *testing.T, in, out *MediaInfo, r *Result) {
				wantVideo(t, out, "h264", 720, 480)
				if out.Video.Interlaced() {
					t.Errorf("still interlaced")
				}
			}},
		{"surround + subs to mp4", sample("surround_subs.mkv"), func(s *Settings) { Presets[0].Apply(s) },
			func(t *testing.T, in, out *MediaInfo, r *Result) {
				if out.Audio[0].Channels != 2 || len(out.Subs) != 1 || out.Subs[0].Codec != "mov_text" {
					t.Errorf("audio/subs: %+v %+v", out.Audio, out.Subs)
				}
			}},
		{"mkv hevc keeps subs", sample("surround_subs.mkv"), func(s *Settings) { s.Method = MethodQuality; s.FormatID = "mkv_hevc" },
			func(t *testing.T, in, out *MediaInfo, r *Result) {
				wantVideo(t, out, "hevc", 1280, 720)
				if len(out.Subs) != 1 {
					t.Errorf("subs: %+v", out.Subs)
				}
			}},
		{"webm vp9", sample("h264_1080.mp4"), func(s *Settings) { s.FormatID = "webm_vp9"; s.ResIndex = resIndex(480) },
			func(t *testing.T, in, out *MediaInfo, r *Result) {
				wantVideo(t, out, "vp9", 854, 480)
				if out.Audio[0].Codec != "opus" {
					t.Errorf("audio %s", out.Audio[0].Codec)
				}
			}},
		{"av1", sample("vp9.webm"), func(s *Settings) { s.FormatID = "mp4_av1" },
			func(t *testing.T, in, out *MediaInfo, r *Result) { wantVideo(t, out, "av1", 640, 360) }},
		{"avi xvid", sample("h264_1080.mp4"), func(s *Settings) { s.FormatID = "avi_xvid"; s.ResIndex = resIndex(480) },
			func(t *testing.T, in, out *MediaInfo, r *Result) {
				wantVideo(t, out, "mpeg4", 854, 480)
				if out.Audio[0].Codec != "mp3" {
					t.Errorf("audio %s", out.Audio[0].Codec)
				}
			}},
		{"mov h264 30fps limit", sample("h264_1080.mp4"), func(s *Settings) { s.FormatID = "mov_h264"; s.FPSIndex = 3 },
			func(t *testing.T, in, out *MediaInfo, r *Result) {
				wantVideo(t, out, "h264", 1920, 1080)
				if out.Video.FPS > 24.5 {
					t.Errorf("fps %v", out.Video.FPS)
				}
			}},
		{"remove audio", sample("h264_1080.mp4"), func(s *Settings) { s.AudioIndex = audioIndex(AudioRemove) },
			func(t *testing.T, in, out *MediaInfo, r *Result) {
				if len(out.Audio) != 0 {
					t.Errorf("audio still present")
				}
			}},
		{"copy audio when compatible", sample("h264_1080.mp4"), func(s *Settings) { s.AudioIndex = audioIndex(AudioCopy) },
			func(t *testing.T, in, out *MediaInfo, r *Result) {
				if out.Audio[0].Codec != "aac" {
					t.Errorf("audio %s", out.Audio[0].Codec)
				}
			}},
		{"remux mkv->mp4", sample("surround_subs.mkv"), func(s *Settings) { Presets[6].Apply(s) },
			func(t *testing.T, in, out *MediaInfo, r *Result) {
				wantVideo(t, out, "h264", 1280, 720)
				if out.Audio[0].Codec != "ac3" || out.Audio[0].Channels != 6 {
					t.Errorf("audio should be copied: %+v", out.Audio)
				}
				if r.Encoder != "" {
					t.Errorf("remux should not encode video (%s)", r.Encoder)
				}
			}},
		{"remux mov pcm->mp4 re-encodes audio", sample("pcm.mov"), func(s *Settings) { Presets[6].Apply(s) },
			func(t *testing.T, in, out *MediaInfo, r *Result) {
				if out.Audio[0].Codec != "aac" {
					t.Errorf("audio %s", out.Audio[0].Codec)
				}
			}},
		{"remux mp4->mkv", sample("h264_1080.mp4"), func(s *Settings) { Presets[6].Apply(s); s.FormatID = "remux_mkv" },
			func(t *testing.T, in, out *MediaInfo, r *Result) { wantVideo(t, out, "h264", 1920, 1080) }},
		{"remux wmv->mp4 needs video encode", sample("old.wmv"), func(s *Settings) { Presets[6].Apply(s) },
			func(t *testing.T, in, out *MediaInfo, r *Result) { wantVideo(t, out, "h264", 640, 480) }},
		{"remux avi->mp4", sample("xvid.avi"), func(s *Settings) { Presets[6].Apply(s) },
			func(t *testing.T, in, out *MediaInfo, r *Result) { wantVideo(t, out, "mpeg4", 640, 480) }},
		{"remux mp4->ts", sample("h264_1080.mp4"), func(s *Settings) { Presets[6].Apply(s); s.FormatID = "remux_ts" },
			func(t *testing.T, in, out *MediaInfo, r *Result) { wantVideo(t, out, "h264", 1920, 1080) }},
		{"remux webm->webm", sample("vp9.webm"), func(s *Settings) { Presets[6].Apply(s); s.FormatID = "remux_webm" },
			func(t *testing.T, in, out *MediaInfo, r *Result) { wantVideo(t, out, "vp9", 640, 360) }},
		{"remux h264->webm re-encodes", sample("portrait.mp4"), func(s *Settings) { Presets[6].Apply(s); s.FormatID = "remux_webm" },
			func(t *testing.T, in, out *MediaInfo, r *Result) { wantVideo(t, out, "vp9", 1080, 1920) }},
		{"extract mp3", sample("h264_1080.mp4"), func(s *Settings) { Presets[7].Apply(s) },
			func(t *testing.T, in, out *MediaInfo, r *Result) {
				if out.Video != nil || out.Audio[0].Codec != "mp3" {
					t.Errorf("not mp3: %+v %+v", out.Video, out.Audio)
				}
			}},
		{"extract m4a copy", sample("h264_1080.mp4"), func(s *Settings) { Presets[7].Apply(s); s.FormatID = "aud_m4a"; s.AudioIndex = audioIndex(AudioCopy) },
			func(t *testing.T, in, out *MediaInfo, r *Result) {
				if out.Audio[0].Codec != "aac" {
					t.Errorf("audio %s", out.Audio[0].Codec)
				}
			}},
		{"extract wav from 5.1", sample("surround_subs.mkv"), func(s *Settings) { Presets[7].Apply(s); s.FormatID = "aud_wav" },
			func(t *testing.T, in, out *MediaInfo, r *Result) {
				if out.Audio[0].Codec != "pcm_s16le" {
					t.Errorf("audio %s", out.Audio[0].Codec)
				}
			}},
		{"mp3 with cover to m4a", sample("song_cover.mp3"), func(s *Settings) { Presets[7].Apply(s); s.FormatID = "aud_m4a" },
			func(t *testing.T, in, out *MediaInfo, r *Result) {
				if out.Audio[0].Codec != "aac" {
					t.Errorf("audio %s", out.Audio[0].Codec)
				}
			}},
		{"flac", sample("song_cover.mp3"), func(s *Settings) { Presets[7].Apply(s); s.FormatID = "aud_flac" }, nil},
		{"wmv to h264", sample("old.wmv"), func(s *Settings) { Presets[0].Apply(s) },
			func(t *testing.T, in, out *MediaInfo, r *Result) { wantVideo(t, out, "h264", 640, 480) }},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) { runCase(t, c) })
	}
}

func TestTargetSize(t *testing.T) {
	for _, tc := range []struct {
		format string
		mb     float64
	}{
		{"mp4_h264", 1.0},
		{"mp4_hevc", 0.6},
		{"webm_vp9", 0.8},
		{"mp4_av1", 0.5},
		{"avi_xvid", 1.5},
	} {
		tc := tc
		t.Run(tc.format, func(t *testing.T) {
			var res *Result
			runCase(t, convCase{name: tc.format, input: sample("h264_1080.mp4"), settings: func(s *Settings) {
				Presets[5].Apply(s)
				s.FormatID = tc.format
				s.TargetMB = tc.mb
			}, check: func(t *testing.T, in, out *MediaInfo, r *Result) { res = r }})
			limit := int64(tc.mb * 1024 * 1024)
			t.Logf("target %.1fMB -> %s (%.0f%% of target)", tc.mb, FormatSize(res.Size), float64(res.Size)/float64(limit)*100)
			if res.Size > limit*105/100 {
				t.Errorf("output %s exceeds target %.1fMB by more than 5%%", FormatSize(res.Size), tc.mb)
			}
			if res.Size < limit*45/100 {
				t.Errorf("output %s far below target %.1fMB", FormatSize(res.Size), tc.mb)
			}
		})
	}
}

func TestTargetLargerThanSource(t *testing.T) {
	var res *Result
	in, _ := runCase(t, convCase{name: "target larger than source", input: sample("h264_1080.mp4"), settings: func(s *Settings) {
		Presets[5].Apply(s)
		s.TargetMB = 100
	}, check: func(t *testing.T, in, out *MediaInfo, r *Result) { res = r }})
	_ = in
	src := probe(t, sample("h264_1080.mp4"))
	if res.Size >= src.Size {
		t.Errorf("output %s should be smaller than source %s", FormatSize(res.Size), FormatSize(src.Size))
	}
}

func TestTargetTooSmall(t *testing.T) {
	in := probe(t, sample("long_1080.mp4"))
	s := DefaultSettings()
	Presets[5].Apply(&s)
	s.TargetMB = 0.05
	_, err := BuildPlan(PlanInput{Info: in, Settings: s, Output: "x.mp4", PassLog: "p"})
	if err == nil || !strings.Contains(err.Error(), "너무 작습니다") {
		t.Fatalf("expected too-small error, got %v", err)
	}
}

func TestAutoScale(t *testing.T) {
	v := &VideoStream{DispW: 1920, DispH: 1080, FPS: 30}
	if got := autoShort(v, 0, 8000, VH264, 0); got != 0 {
		t.Errorf("8 Mbps 1080p should keep size, got %d", got)
	}
	if got := autoShort(v, 0, 1200, VH264, 0); got != 720 && got != 540 {
		t.Errorf("1.2 Mbps 1080p H.264 -> %d", got)
	}
	if got := autoShort(v, 0, 150, VH264, 0); got != 240 && got != 360 {
		t.Errorf("150 kbps -> %d", got)
	}
	p := &VideoStream{DispW: 1080, DispH: 1920, FPS: 30}
	if got := autoShort(p, 0, 1200, VHEVC, 0); got == 0 || got > 1080 {
		t.Errorf("portrait hevc -> %d", got)
	}
}

func TestCancel(t *testing.T) {
	in := probe(t, sample("long_1080.mp4"))
	s := DefaultSettings()
	s.FormatID, s.Speed = "mp4_hevc", SpeedSlow
	outDir := t.TempDir()
	final := OutputPath(in.Path, outDir, s.Suffix, ".mp4", nil)
	ctx, cancel := context.WithCancel(context.Background())
	conv := &Converter{Tools: tools}
	started := make(chan struct{}, 1)
	go func() {
		<-started
		time.Sleep(700 * time.Millisecond)
		cancel()
	}()
	_, err := conv.Convert(ctx, in, s, final, func(p Progress) {
		select {
		case started <- struct{}{}:
		default:
		}
	}, nil)
	if err != ErrCanceled {
		t.Fatalf("expected ErrCanceled, got %v", err)
	}
	entries, _ := os.ReadDir(outDir)
	if len(entries) != 0 {
		t.Fatalf("leftover files: %v", entries)
	}
}

func TestOutputPath(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "영상.mp4")
	os.WriteFile(in, []byte("x"), 0o644)
	p1 := OutputPath(in, "", "_변환", ".mp4", nil)
	if filepath.Base(p1) != "영상_변환.mp4" {
		t.Fatalf("p1 %s", p1)
	}
	os.WriteFile(p1, []byte("x"), 0o644)
	p2 := OutputPath(in, "", "_변환", ".mp4", nil)
	if filepath.Base(p2) != "영상_변환 (2).mp4" {
		t.Fatalf("p2 %s", p2)
	}
	// Empty suffix with the same extension must never overwrite the source.
	p3 := OutputPath(in, "", "", ".mp4", nil)
	if p3 == in || filepath.Base(p3) != "영상 (2).mp4" {
		t.Fatalf("p3 %s", p3)
	}
	// Reserved names are skipped.
	res := map[string]bool{strings.ToLower(filepath.Join(dir, "영상_변환 (2).mp4")): true}
	p4 := OutputPath(in, "", "_변환", ".mp4", res)
	if filepath.Base(p4) != "영상_변환 (3).mp4" {
		t.Fatalf("p4 %s", p4)
	}
	if got := sanitizeName(`_a:b*?"<>|`); got != "_ab" {
		t.Fatalf("sanitize %q", got)
	}
}

func TestPresetsMatch(t *testing.T) {
	for i := 0; i < CustomPresetIndex; i++ {
		s := DefaultSettings()
		Presets[i].Apply(&s)
		if got := MatchPreset(s); got != i {
			t.Errorf("preset %d matched as %d", i, got)
		}
	}
	s := DefaultSettings()
	s.Quality = 31
	if MatchPreset(s) != CustomPresetIndex {
		t.Errorf("modified settings should be custom")
	}
}

func TestFormatting(t *testing.T) {
	if FormatSize(1536) != "1.50 KB" || FormatSize(150*1024*1024) != "150 MB" {
		t.Errorf("FormatSize: %s %s", FormatSize(1536), FormatSize(150*1024*1024))
	}
	if FormatDuration(3725) != "1:02:05" || FormatDuration(65) != "1:05" {
		t.Errorf("FormatDuration")
	}
	if !strings.Contains(SavingText(100, 30), "70% 감소") {
		t.Errorf("SavingText %s", SavingText(100, 30))
	}
}
