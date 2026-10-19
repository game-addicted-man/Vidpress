package engine

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"
)

// hwCandidates are tested in order of preference (NVIDIA, Intel, AMD).
var hwCandidates = []string{
	"h264_nvenc", "hevc_nvenc", "av1_nvenc",
	"h264_qsv", "hevc_qsv",
	"h264_amf", "hevc_amf",
}

func kindOf(name string) encKind {
	switch {
	case strings.HasSuffix(name, "_nvenc"):
		return encNVENC
	case strings.HasSuffix(name, "_qsv"):
		return encQSV
	case strings.HasSuffix(name, "_amf"):
		return encAMF
	}
	return encNone
}

// DetectHW returns the hardware encoders that actually work on this PC. Each
// candidate is verified with a tiny test encode using the same options as a
// real conversion, so a GPU with a too-old driver is not offered.
func DetectHW(ctx context.Context, ffmpeg string) map[string]bool {
	res := map[string]bool{}
	cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-encoders")
	setProcAttr(cmd, false)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return res
	}
	list := out.String()
	for _, name := range hwCandidates {
		if !strings.Contains(list, " "+name+" ") {
			continue
		}
		if testHWEncoder(ctx, ffmpeg, name) {
			res[name] = true
		}
	}
	return res
}

func testHWEncoder(ctx context.Context, ffmpeg, name string) bool {
	k := kindOf(name)
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=gray:s=1280x720:r=30:d=1",
		"-frames:v", "10", "-c:v", name}
	args = append(args, speedPreset(k, SpeedNormal)...)
	args = append(args, hwQualityArgs(name, k, QualityDefault)...)
	args = append(args, "-pix_fmt", "nv12", "-f", "null", "-")
	tctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(tctx, ffmpeg, args...)
	setProcAttr(cmd, false)
	return cmd.Run() == nil
}

// HWVendorText describes the detected hardware for the UI, e.g. "NVIDIA".
func HWVendorText(hw map[string]bool) string {
	var v []string
	if hw["h264_nvenc"] || hw["hevc_nvenc"] || hw["av1_nvenc"] {
		v = append(v, "NVIDIA")
	}
	if hw["h264_qsv"] || hw["hevc_qsv"] {
		v = append(v, "Intel")
	}
	if hw["h264_amf"] || hw["hevc_amf"] {
		v = append(v, "AMD")
	}
	return strings.Join(v, ", ")
}

// HWSupports reports whether a hardware encoder exists for the video codec.
func HWSupports(hw map[string]bool, c VideoCodec) bool {
	n, k := pickEncoder(c, true, hw)
	return n != "" && k.hardware()
}
