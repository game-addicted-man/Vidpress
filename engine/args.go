package engine

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// encKind is the concrete encoder implementation.
type encKind int

const (
	encNone encKind = iota
	encX264
	encX265
	encVPX
	encSVT
	encMPEG4
	encNVENC
	encQSV
	encAMF
)

func (k encKind) hardware() bool { return k == encNVENC || k == encQSV || k == encAMF }

// Pass is one ffmpeg invocation.
type Pass struct {
	Args   []string // arguments without the ffmpeg executable
	Weight float64  // share of the overall progress for this file (sums to 1)
	Label  string   // short description for the status text
}

// Plan is the fully resolved list of ffmpeg invocations for one file.
type Plan struct {
	Passes    []Pass
	Notes     []string // human readable notes for the log
	Encoder   string   // video encoder name ("" when copying / audio only)
	Hardware  bool     // true when a GPU encoder is used
	VideoKbps int      // MethodTarget: chosen video bitrate
	Short     int      // resulting short side when downscaled (0 = unchanged)
}

// PlanInput carries everything BuildPlan needs.
type PlanInput struct {
	Info       *MediaInfo
	Settings   Settings
	Output     string          // file ffmpeg writes to
	PassLog    string          // base name for 2-pass statistics (relative to the working dir)
	NullPath   string          // "NUL" on Windows, "/dev/null" elsewhere
	HWEncoders map[string]bool // usable hardware encoders (from DetectHW)
	ForceSW    bool            // ignore Settings.UseHW (used for the automatic fallback)
	// TargetScale multiplies the computed video bitrate in MethodTarget
	// (used to correct an overshoot on a second attempt). 0 means 1.
	TargetScale float64
}

// Bitrate floor below which we refuse to encode (kbps).
const minVideoKbps = 40

// BuildPlan turns settings + media info into ffmpeg arguments.
func BuildPlan(in PlanInput) (*Plan, error) {
	if in.Info == nil {
		return nil, errors.New("동영상 정보가 없습니다")
	}
	if in.NullPath == "" {
		in.NullPath = "-"
	}
	s := in.Settings
	s.Normalize()
	switch s.Method {
	case MethodAudio:
		return buildAudioOnly(in, s)
	case MethodRemux:
		return buildRemux(in, s)
	default:
		if in.Info.Video == nil {
			return nil, errors.New("영상 트랙이 없는 파일입니다 (소리만 있는 파일은 '소리만 추출'을 사용하세요)")
		}
		return buildEncode(in, s)
	}
}

func baseArgs(input string, extraInput ...string) []string {
	a := []string{"-hide_banner", "-nostdin", "-y", "-loglevel", "error", "-progress", "pipe:1", "-nostats"}
	a = append(a, extraInput...)
	return append(a, "-i", input)
}

// ---------------------------------------------------------------------------
// Encoder selection and quality mapping

func pickEncoder(c VideoCodec, useHW bool, hw map[string]bool) (string, encKind) {
	if useHW {
		var cands []string
		switch c {
		case VH264:
			cands = []string{"h264_nvenc", "h264_qsv", "h264_amf"}
		case VHEVC:
			cands = []string{"hevc_nvenc", "hevc_qsv", "hevc_amf"}
		case VAV1:
			cands = []string{"av1_nvenc"}
		}
		for _, n := range cands {
			if hw[n] {
				switch {
				case strings.HasSuffix(n, "_nvenc"):
					return n, encNVENC
				case strings.HasSuffix(n, "_qsv"):
					return n, encQSV
				default:
					return n, encAMF
				}
			}
		}
	}
	switch c {
	case VH264:
		return "libx264", encX264
	case VHEVC:
		return "libx265", encX265
	case VAV1:
		return "libsvtav1", encSVT
	case VVP9:
		return "libvpx-vp9", encVPX
	case VMPEG4:
		return "mpeg4", encMPEG4
	}
	return "", encNone
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// mapQuality converts the x264-style quality value to the encoder's scale.
func mapQuality(name string, k encKind, q int) int {
	switch k {
	case encX264:
		return clampInt(q, 0, 51)
	case encX265:
		return clampInt(q+5, 0, 51)
	case encVPX:
		return clampInt(int(math.Round(float64(q)*1.35+2)), 0, 63)
	case encSVT:
		return clampInt(int(math.Round(float64(q)*1.3+5)), 1, 63)
	case encMPEG4:
		return clampInt(int(math.Round(float64(q-11)/2.5)), 2, 31)
	case encNVENC:
		if strings.HasPrefix(name, "av1") {
			return clampInt(int(math.Round(float64(q)*1.3+5)), 1, 63)
		}
		if strings.HasPrefix(name, "hevc") {
			return clampInt(q+4, 1, 51)
		}
		return clampInt(q+1, 1, 51)
	case encQSV:
		if strings.HasPrefix(name, "hevc") {
			return clampInt(q+3, 1, 51)
		}
		return clampInt(q+1, 1, 51)
	case encAMF:
		if strings.HasPrefix(name, "hevc") {
			return clampInt(q+2, 0, 51)
		}
		return clampInt(q, 0, 51)
	}
	return q
}

func speedPreset(k encKind, sp Speed) []string {
	pick := func(fast, normal, slow string) string {
		switch sp {
		case SpeedFast:
			return fast
		case SpeedSlow:
			return slow
		}
		return normal
	}
	switch k {
	case encX264, encX265:
		return []string{"-preset", pick("veryfast", "medium", "slow")}
	case encVPX:
		return []string{"-deadline", "good", "-cpu-used", pick("5", "3", "1"), "-row-mt", "1"}
	case encSVT:
		return []string{"-preset", pick("10", "8", "6")}
	case encNVENC:
		return []string{"-preset", pick("p2", "p5", "p7"), "-tune", "hq"}
	case encQSV:
		return []string{"-preset", pick("veryfast", "medium", "veryslow")}
	case encAMF:
		return []string{"-quality", pick("speed", "balanced", "quality")}
	}
	return nil
}

// HWTestArgs returns the encoder options used both for the real encode and for
// the availability test in DetectHW, so a successful test means the real
// configuration works too.
func hwQualityArgs(name string, k encKind, q int) []string {
	v := strconv.Itoa(mapQuality(name, k, q))
	switch k {
	case encNVENC:
		return []string{"-rc", "vbr", "-cq", v, "-b:v", "0"}
	case encQSV:
		return []string{"-global_quality", v}
	case encAMF:
		if strings.HasPrefix(name, "h264") {
			return []string{"-rc", "cqp", "-qp_i", v, "-qp_p", v, "-qp_b", v}
		}
		return []string{"-rc", "cqp", "-qp_i", v, "-qp_p", v}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Filters

// scaleShort returns the short side the picture should be limited to
// (0 = keep), considering the user's choice and the source size.
func scaleShort(v *VideoStream, limit int) int {
	if limit <= 0 || v == nil {
		return 0
	}
	short := v.DispW
	if v.DispH < short {
		short = v.DispH
	}
	if short > 0 && short <= limit {
		return 0
	}
	return limit
}

func videoFilters(v *VideoStream, short int, toneMap bool, fpsLimit float64, pixFmt string) string {
	var f []string
	if v.Interlaced() {
		f = append(f, "bwdif=mode=send_frame:parity=auto:deint=interlaced")
	}
	if short > 0 {
		s := strconv.Itoa(short)
		f = append(f, "scale=w='if(gte(iw,ih),-2,2*trunc(min(iw,"+s+")/2))':h='if(gte(iw,ih),2*trunc(min(ih,"+s+")/2),-2)':flags=lanczos")
	} else if v.Width%2 != 0 || v.Height%2 != 0 {
		// 4:2:0 encoders need even dimensions.
		f = append(f, "scale=w='2*trunc(iw/2)':h='2*trunc(ih/2)'")
	}
	if toneMap {
		tin := v.ColorTransfer
		pin := v.ColorPrimaries
		if pin == "" || pin == "unknown" {
			pin = "bt2020"
		}
		min := v.ColorSpace
		switch min {
		case "", "unknown", "bt2020_ncl":
			min = "bt2020nc"
		case "bt2020_cl":
			min = "bt2020c"
		}
		f = append(f,
			"zscale=tin="+tin+":pin="+pin+":min="+min+":t=linear:npl=100",
			"format=gbrpf32le",
			"zscale=p=bt709",
			"tonemap=tonemap=hable:desat=0",
			"zscale=t=bt709:m=bt709:r=tv",
		)
	}
	if fpsLimit > 0 && v.FPS > fpsLimit+0.5 {
		f = append(f, "fps="+trimFloat(fpsLimit))
	}
	if pixFmt != "" {
		f = append(f, "format="+pixFmt)
	}
	return strings.Join(f, ",")
}

func trimFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// pixFmtFor picks the output pixel format.
func pixFmtFor(k encKind, c VideoCodec, v *VideoStream, toneMapped bool) (string, bool) {
	tenBit := v.BitDepth > 8 && !toneMapped && (c == VHEVC || c == VAV1)
	if k.hardware() {
		if tenBit {
			return "p010le", true
		}
		return "nv12", false
	}
	if tenBit {
		return "yuv420p10le", true
	}
	return "yuv420p", false
}

// ---------------------------------------------------------------------------
// Audio / subtitle helpers

var copyOK = map[string]struct{ video, audio, subs map[string]bool }{
	"mp4": {
		video: set("h264", "hevc", "av1", "vp9", "mpeg4", "mpeg2video", "mpeg1video"),
		audio: set("aac", "mp3", "ac3", "eac3", "alac", "opus", "flac"),
		subs:  set("mov_text"),
	},
	"mov": {
		video: set("h264", "hevc", "mpeg4", "mpeg2video", "prores", "mjpeg", "dnxhd"),
		audio: set("aac", "mp3", "ac3", "eac3", "alac", "pcm_s16le", "pcm_s24le", "pcm_s16be", "pcm_s24be"),
		subs:  set("mov_text"),
	},
	"webm": {
		video: set("vp8", "vp9", "av1"),
		audio: set("opus", "vorbis"),
		subs:  set("webvtt"),
	},
	"avi": {
		video: set("mpeg4", "h264", "mjpeg", "msmpeg4v3", "msmpeg4v2", "mpeg2video", "mpeg1video"),
		audio: set("mp3", "ac3", "pcm_s16le", "mp2"),
		subs:  set(),
	},
	"mpegts": {
		video: set("h264", "hevc", "mpeg2video", "mpeg1video"),
		audio: set("aac", "mp3", "ac3", "eac3", "mp2"),
		subs:  set("dvb_subtitle"),
	},
	"mp3":  {audio: set("mp3")},
	"wav":  {audio: set("pcm_s16le", "pcm_s24le", "pcm_f32le")},
	"flac": {audio: set("flac")},
}

func set(items ...string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, i := range items {
		m[i] = true
	}
	return m
}

func canCopyVideo(muxer, codec string) bool {
	if muxer == "matroska" {
		return true
	}
	return copyOK[muxer].video[codec]
}

func canCopyAudio(muxer, codec string) bool {
	if muxer == "matroska" {
		return codec != ""
	}
	return copyOK[muxer].audio[codec]
}

func audioEncoder(c AudioCodec) string {
	switch c {
	case AOpus:
		return "libopus"
	case AMP3:
		return "libmp3lame"
	case APCM:
		return "pcm_s16le"
	case AFLAC:
		return "flac"
	}
	return "aac"
}

// audioArgs maps and configures every audio track. kbps: >0 encode, AudioCopy
// copy when possible, AudioRemove drop. It returns the args and the total audio
// bitrate (kbps, estimated for copied tracks).
func audioArgs(info *MediaInfo, f Format, kbps int, notes *[]string) ([]string, int) {
	if kbps == AudioRemove || len(info.Audio) == 0 {
		return []string{"-an"}, 0
	}
	var a []string
	total := 0
	for n, st := range info.Audio {
		a = append(a, "-map", "0:"+strconv.Itoa(st.Index))
		ns := strconv.Itoa(n)
		if kbps == AudioCopy && canCopyAudio(f.Muxer, st.Codec) {
			a = append(a, "-c:a:"+ns, "copy")
			if st.BitRate > 0 {
				total += int(st.BitRate / 1000)
			} else {
				total += 192
			}
			continue
		}
		br := kbps
		if br <= 0 { // copy requested but not possible
			br = 192
			if n == 0 {
				*notes = append(*notes, fmt.Sprintf("원본 소리(%s)는 %s에 그대로 넣을 수 없어 다시 압축합니다", prettyCodec(st.Codec), strings.ToUpper(strings.TrimPrefix(f.Ext, "."))))
			}
		}
		if f.Audio == AOpus && br > 256 {
			br = 256
		}
		a = append(a, "-c:a:"+ns, audioEncoder(f.Audio), "-b:a:"+ns, strconv.Itoa(br)+"k")
		if st.Channels > 2 || st.Channels == 0 {
			a = append(a, "-ac:a:"+ns, "2")
		}
		if f.Audio == AMP3 && st.SampleRate > 48000 {
			a = append(a, "-ar:a:"+ns, "48000")
		}
		total += br
	}
	return a, total
}

func subtitleArgs(info *MediaInfo, f Format, keep bool) []string {
	if !keep || len(info.Subs) == 0 {
		return []string{"-sn"}
	}
	var a []string
	n := 0
	for _, st := range info.Subs {
		var codec string
		switch f.Muxer {
		case "matroska":
			codec = "copy"
		case "mp4", "mov":
			if st.TextBased() {
				codec = "mov_text"
			}
		case "webm":
			if st.TextBased() {
				codec = "webvtt"
			}
		}
		if codec == "" {
			continue
		}
		ns := strconv.Itoa(n)
		a = append(a, "-map", "0:"+strconv.Itoa(st.Index), "-c:s:"+ns, codec)
		n++
	}
	if n == 0 {
		return []string{"-sn"}
	}
	return a
}

func containerArgs(f Format, videoName string) []string {
	var a []string
	switch f.Muxer {
	case "mp4", "mov":
		a = append(a, "-movflags", "+faststart")
		if strings.HasPrefix(videoName, "hevc") || videoName == "libx265" {
			a = append(a, "-tag:v", "hvc1")
		}
	case "avi":
		if videoName == "mpeg4" {
			a = append(a, "-vtag", "XVID")
		}
	case "mp3":
		a = append(a, "-id3v2_version", "3")
	}
	return a
}

// ---------------------------------------------------------------------------
// Encode (quality / target size)

type videoSetup struct {
	name    string
	kind    encKind
	filters string
	pixFmt  string
	tenBit  bool
}

func setupVideo(in PlanInput, s Settings, f Format, short int) videoSetup {
	v := in.Info.Video
	name, kind := pickEncoder(f.Video, s.UseHW && !in.ForceSW, in.HWEncoders)
	toneMap := s.ToneMapHDR && v.HDR()
	pix, tenBit := pixFmtFor(kind, f.Video, v, toneMap)
	vs := videoSetup{name: name, kind: kind, pixFmt: pix, tenBit: tenBit}
	filters := videoFilters(v, short, toneMap, s.FPS().FPS, pix)
	// Colour tags are set on the frames (setparams) rather than with the
	// -color_* output options, which newer FFmpeg versions ignore when the
	// frames already carry colour properties.
	var tags []string
	switch {
	case toneMap:
		tags = []string{"color_primaries=bt709", "color_trc=bt709", "colorspace=bt709"}
	case v.HDR() && tenBit:
		for _, kv := range [][2]string{{"color_primaries", v.ColorPrimaries}, {"color_trc", v.ColorTransfer}, {"colorspace", v.ColorSpace}} {
			if kv[1] != "" && kv[1] != "unknown" {
				tags = append(tags, kv[0]+"="+kv[1])
			}
		}
	}
	if len(tags) > 0 {
		filters += ",setparams=" + strings.Join(tags, ":")
	}
	vs.filters = strings.TrimPrefix(filters, ",")
	return vs
}

func buildEncode(in PlanInput, s Settings) (*Plan, error) {
	info := in.Info
	v := info.Video
	f := s.Format()
	p := &Plan{}

	short := scaleShort(v, s.Resolution().Short)

	// Audio first: the target-size math needs its bitrate.
	akbps := s.Audio().Kbps
	var videoKbps int
	fitsAlready := false
	if s.Method == MethodTarget {
		if info.Duration <= 0.5 {
			return nil, errors.New("영상 길이를 알 수 없어 목표 용량을 계산할 수 없습니다")
		}
		targetBits := s.TargetMB * 1024 * 1024 * 8
		totalKbps := targetBits / info.Duration / 1000 * 0.96 // ~4% container overhead
		// Shrink audio when the budget is tight.
		if akbps > 0 {
			for _, lim := range []struct{ below, max float64 }{{150, 32}, {300, 48}, {600, 64}, {1200, 96}} {
				if totalKbps < lim.below && float64(akbps) > lim.max {
					p.Notes = append(p.Notes, fmt.Sprintf("목표 용량이 작아 소리를 %dkbps → %dkbps로 낮춥니다", akbps, int(lim.max)))
					akbps = int(lim.max)
					break
				}
			}
		}
		var tmpNotes []string
		_, audioTotal := audioArgs(info, f, akbps, &tmpNotes)
		videoKbps = int(totalKbps) - audioTotal
		if videoKbps < minVideoKbps {
			need := (float64(minVideoKbps+audioTotal) * 1000 / 8 * info.Duration / 0.96) / (1024 * 1024)
			return nil, fmt.Errorf("목표 용량(%.0fMB)이 영상 길이(%s)에 비해 너무 작습니다. 최소 %.0fMB 이상으로 정해 주세요", s.TargetMB, FormatDuration(info.Duration), math.Ceil(need))
		}
		// The source already fits: spending the whole budget would only make
		// the file bigger without improving quality, so encode for (high)
		// quality instead, capped at the budget.
		if srcKbps := sourceVideoKbps(info); srcKbps > 0 && videoKbps > srcKbps && in.TargetScale == 0 {
			fitsAlready = true
			p.Notes = append(p.Notes, "원본이 이미 목표 용량보다 작아서, 용량이 늘지 않도록 고화질 기준으로 변환합니다")
		}
		if s.AutoScale && !fitsAlready {
			if ns := autoShort(v, short, videoKbps, f.Video, s.FPS().FPS); ns > 0 && (short == 0 || ns < short) {
				p.Notes = append(p.Notes, fmt.Sprintf("목표 용량에 맞춰 선명하게 보이도록 해상도를 %dp로 낮춥니다", ns))
				short = ns
			}
		}
	}
	p.Short = short

	vs := setupVideo(in, s, f, short)
	if s.Method == MethodTarget {
		scale := in.TargetScale
		if scale <= 0 {
			scale = 1
		}
		// Single-pass rate control (AV1, GPU encoders) tends to overshoot a
		// little, so leave some headroom.
		if !(vs.kind == encX264 || vs.kind == encX265 || vs.kind == encVPX || vs.kind == encMPEG4) {
			scale *= 0.93
		}
		videoKbps = int(float64(videoKbps) * scale)
		if videoKbps < minVideoKbps {
			videoKbps = minVideoKbps
		}
		p.VideoKbps = videoKbps
	}
	p.Encoder, p.Hardware = vs.name, vs.kind.hardware()
	if v.HDR() && s.ToneMapHDR {
		p.Notes = append(p.Notes, "HDR 영상을 일반(SDR) 색으로 변환합니다")
	}

	vmap := []string{"-map", "0:" + strconv.Itoa(v.Index)}
	vf := []string{}
	if vs.filters != "" {
		vf = []string{"-vf", vs.filters}
	}
	codec := []string{"-c:v", vs.name}
	codec = append(codec, speedPreset(vs.kind, s.Speed)...)

	var aNotes []string
	aargs, _ := audioArgs(info, f, akbps, &aNotes)
	p.Notes = append(p.Notes, aNotes...)
	sargs := subtitleArgs(info, f, s.KeepSubs)
	tail := []string{"-map_metadata", "0"}
	tail = append(tail, containerArgs(f, vs.name)...)
	tail = append(tail, "-f", f.Muxer, in.Output)

	full := func(rc []string) []string {
		a := baseArgs(info.Path)
		a = append(a, vmap...)
		a = append(a, vf...)
		a = append(a, codec...)
		a = append(a, rc...)
		a = append(a, aargs...)
		a = append(a, sargs...)
		return append(a, tail...)
	}

	if s.Method == MethodQuality || fitsAlready {
		q := s.Quality
		if fitsAlready {
			q = 20 // visually close to the source
		}
		var rc []string
		switch vs.kind {
		case encX264, encX265, encSVT:
			rc = []string{"-crf", strconv.Itoa(mapQuality(vs.name, vs.kind, q))}
			if vs.kind == encX265 {
				rc = append(rc, "-x265-params", "log-level=error")
			}
		case encVPX:
			rc = []string{"-crf", strconv.Itoa(mapQuality(vs.name, vs.kind, q)), "-b:v", "0"}
		case encMPEG4:
			rc = []string{"-q:v", strconv.Itoa(mapQuality(vs.name, vs.kind, q)), "-mbd", "rd", "-trellis", "1"}
		default:
			rc = hwQualityArgs(vs.name, vs.kind, q)
		}
		if fitsAlready && vs.kind != encSVT && vs.kind != encVPX {
			// Cap the peaks so the result stays within the budget.
			rc = append(rc, "-maxrate", strconv.Itoa(videoKbps)+"k", "-bufsize", strconv.Itoa(videoKbps*2)+"k")
		}
		p.Passes = []Pass{{Args: full(rc), Weight: 1, Label: "변환 중"}}
		return p, nil
	}

	// Target size.
	br := strconv.Itoa(videoKbps) + "k"
	twoPass := vs.kind == encX264 || vs.kind == encX265 || vs.kind == encVPX || vs.kind == encMPEG4
	if !twoPass {
		rc := []string{"-b:v", br, "-maxrate", strconv.Itoa(videoKbps*3/2) + "k", "-bufsize", strconv.Itoa(videoKbps*2) + "k"}
		switch vs.kind {
		case encNVENC:
			rc = append([]string{"-rc", "vbr"}, rc...)
		case encAMF:
			rc = append([]string{"-rc", "vbr_peak"}, rc...)
		case encSVT:
			rc = []string{"-b:v", br}
		}
		p.Passes = []Pass{{Args: full(rc), Weight: 1, Label: "변환 중"}}
		return p, nil
	}

	var rc1, rc2 []string
	switch vs.kind {
	case encX265:
		rc1 = []string{"-b:v", br, "-x265-params", "log-level=error:pass=1:stats=" + in.PassLog + ".x265"}
		rc2 = []string{"-b:v", br, "-x265-params", "log-level=error:pass=2:stats=" + in.PassLog + ".x265"}
	case encMPEG4:
		rc1 = []string{"-b:v", br, "-pass", "1", "-passlogfile", in.PassLog, "-mbd", "rd", "-trellis", "1"}
		rc2 = []string{"-b:v", br, "-pass", "2", "-passlogfile", in.PassLog, "-mbd", "rd", "-trellis", "1"}
	default:
		rc1 = []string{"-b:v", br, "-pass", "1", "-passlogfile", in.PassLog}
		rc2 = []string{"-b:v", br, "-pass", "2", "-passlogfile", in.PassLog}
	}
	codec1 := codec
	if vs.kind == encVPX { // the analysis pass can be much faster
		codec1 = append([]string{"-c:v", vs.name}, "-deadline", "good", "-cpu-used", "4", "-row-mt", "1")
	}
	first := baseArgs(info.Path)
	first = append(first, vmap...)
	first = append(first, vf...)
	first = append(first, codec1...)
	first = append(first, rc1...)
	first = append(first, "-an", "-sn", "-dn", "-f", "null", in.NullPath)
	p.Passes = []Pass{
		{Args: first, Weight: 0.35, Label: "1/2단계 분석 중"},
		{Args: full(rc2), Weight: 0.65, Label: "2/2단계 변환 중"},
	}
	return p, nil
}

// sourceVideoKbps estimates the source video bitrate.
func sourceVideoKbps(info *MediaInfo) int {
	if info.Video != nil && info.Video.BitRate > 0 {
		return int(info.Video.BitRate / 1000)
	}
	total := info.BitRate
	if total <= 0 && info.Duration > 0 && info.Size > 0 {
		total = int64(float64(info.Size*8) / info.Duration)
	}
	if total <= 0 {
		return 0
	}
	var audio int64
	for _, a := range info.Audio {
		if a.BitRate > 0 {
			audio += a.BitRate
		} else {
			audio += 128000
		}
	}
	v := (total - audio) / 1000
	if v <= 0 {
		return 0
	}
	return int(v)
}

// autoShort picks a smaller short side when the bitrate would spread too
// thin over the picture. Returns 0 when no change is needed.
func autoShort(v *VideoStream, curShort, kbps int, c VideoCodec, fpsLimit float64) int {
	if v == nil || v.DispW <= 0 || v.DispH <= 0 {
		return 0
	}
	srcShort := v.DispW
	if v.DispH < srcShort {
		srcShort = v.DispH
	}
	start := srcShort
	if curShort > 0 && curShort < start {
		start = curShort
	}
	fps := v.FPS
	if fps <= 0 || fps > 240 {
		fps = 30
	}
	if fpsLimit > 0 && fps > fpsLimit {
		fps = fpsLimit
	}
	minBPP := 0.045 // bits per pixel per frame that still looks decent
	switch c {
	case VHEVC, VAV1, VVP9:
		minBPP = 0.028
	case VMPEG4:
		minBPP = 0.07
	}
	pixels := func(short int) float64 {
		f := float64(short) / float64(srcShort)
		return float64(v.DispW) * float64(v.DispH) * f * f
	}
	bpp := func(short int) float64 { return float64(kbps) * 1000 / (pixels(short) * fps) }
	if bpp(start) >= minBPP {
		return 0
	}
	for _, cand := range []int{1440, 1080, 720, 540, 480, 360, 240} {
		if cand >= start {
			continue
		}
		if bpp(cand) >= minBPP {
			return cand
		}
	}
	return 240
}

// ---------------------------------------------------------------------------
// Remux

func buildRemux(in PlanInput, s Settings) (*Plan, error) {
	info := in.Info
	f := s.Format()
	p := &Plan{}
	a := baseArgs(info.Path, "-fflags", "+genpts")

	if v := info.Video; v != nil {
		a = append(a, "-map", "0:"+strconv.Itoa(v.Index))
		if canCopyVideo(f.Muxer, v.Codec) {
			a = append(a, "-c:v", "copy")
			if (f.Muxer == "mp4" || f.Muxer == "mov") && v.Codec == "hevc" {
				a = append(a, "-tag:v", "hvc1")
			}
			if f.Muxer == "avi" && v.Codec == "mpeg4" {
				a = append(a, "-vtag", "XVID")
			}
		} else {
			// The picture has to be re-encoded for this container.
			name, kind := pickEncoder(f.Video, false, nil)
			p.Encoder = name
			p.Notes = append(p.Notes, fmt.Sprintf("영상 코덱(%s)은 %s에 그대로 넣을 수 없어 고화질로 다시 압축합니다 (시간이 걸립니다)", prettyCodec(v.Codec), f.Label))
			pix, _ := pixFmtFor(kind, f.Video, v, false)
			if filt := videoFilters(v, 0, false, 0, pix); filt != "" {
				a = append(a, "-vf", filt)
			}
			a = append(a, "-c:v", name)
			a = append(a, speedPreset(kind, SpeedNormal)...)
			q := 18
			switch kind {
			case encVPX:
				a = append(a, "-crf", strconv.Itoa(mapQuality(name, kind, q)), "-b:v", "0")
			case encMPEG4:
				a = append(a, "-q:v", "3", "-vtag", "XVID")
			default:
				a = append(a, "-crf", strconv.Itoa(mapQuality(name, kind, q)))
			}
		}
	}

	// Audio: copy when possible.
	var notes []string
	kbps := s.Audio().Kbps
	if kbps > 0 {
		// Remux keeps audio untouched unless it is incompatible.
		kbps = AudioCopy
	}
	aargs, _ := audioArgs(info, f, kbps, &notes)
	p.Notes = append(p.Notes, notes...)
	a = append(a, aargs...)
	if f.Muxer == "matroska" {
		// Keep every subtitle and attachment (fonts) as-is.
		if len(info.Subs) > 0 && s.KeepSubs {
			a = append(a, "-map", "0:s?", "-c:s", "copy")
		}
		a = append(a, "-map", "0:t?", "-c:t", "copy")
	} else {
		a = append(a, subtitleArgs(info, f, s.KeepSubs)...)
	}
	a = append(a, "-map_metadata", "0")
	if f.Muxer == "mp4" || f.Muxer == "mov" {
		a = append(a, "-movflags", "+faststart")
	}
	a = append(a, "-f", f.Muxer, in.Output)
	label := "형식 바꾸는 중"
	if p.Encoder != "" {
		label = "변환 중"
	}
	p.Passes = []Pass{{Args: a, Weight: 1, Label: label}}
	return p, nil
}

// ---------------------------------------------------------------------------
// Audio only

func buildAudioOnly(in PlanInput, s Settings) (*Plan, error) {
	info := in.Info
	if len(info.Audio) == 0 {
		return nil, errors.New("소리 트랙이 없는 파일입니다")
	}
	f := s.Format()
	st := info.Audio[0]
	p := &Plan{}
	a := baseArgs(info.Path)
	a = append(a, "-map", "0:"+strconv.Itoa(st.Index), "-vn", "-sn", "-dn")
	kbps := s.Audio().Kbps
	if kbps == AudioRemove {
		kbps = 192
	}
	switch {
	case kbps == AudioCopy && canCopyAudio(f.Muxer, st.Codec):
		a = append(a, "-c:a", "copy")
	case f.Audio == APCM || f.Audio == AFLAC:
		a = append(a, "-c:a", audioEncoder(f.Audio))
	default:
		if kbps <= 0 {
			kbps = 192
			p.Notes = append(p.Notes, fmt.Sprintf("원본 소리(%s)를 %s로 바꾸기 위해 192kbps로 압축합니다", prettyCodec(st.Codec), strings.ToUpper(strings.TrimPrefix(f.Ext, "."))))
		}
		a = append(a, "-c:a", audioEncoder(f.Audio), "-b:a", strconv.Itoa(kbps)+"k")
		if st.Channels > 2 || st.Channels == 0 {
			a = append(a, "-ac", "2")
		}
		if f.Audio == AMP3 && st.SampleRate > 48000 {
			a = append(a, "-ar", "48000")
		}
	}
	a = append(a, "-map_metadata", "0")
	a = append(a, containerArgs(f, "")...)
	a = append(a, "-f", f.Muxer, in.Output)
	p.Passes = []Pass{{Args: a, Weight: 1, Label: "소리 추출 중"}}
	return p, nil
}
