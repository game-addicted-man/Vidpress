package engine

import "fmt"

// Method is how a file is processed.
type Method int

const (
	MethodQuality Method = iota // constant quality (CRF) — best for "줄이기"
	MethodTarget                // fit into a target file size
	MethodRemux                 // copy streams into another container (no re-encode)
	MethodAudio                 // extract audio only
)

// MethodLabels are shown in the "방식" combo box, indexed by Method.
var MethodLabels = []string{
	"화질 기준으로 줄이기 (권장)",
	"목표 용량에 맞추기",
	"형식만 바꾸기 (재압축 없음 · 빠름 · 무손실)",
	"소리만 추출",
}

// Speed trades encoding time for file size.
type Speed int

const (
	SpeedFast Speed = iota
	SpeedNormal
	SpeedSlow
)

var SpeedLabels = []string{
	"빠르게 (용량 조금 큼)",
	"보통 (권장)",
	"천천히 (더 작게)",
}

// Resolution limits the *shorter* side of the picture (so portrait phone
// videos are treated the same as landscape ones). Videos are never upscaled.
type Resolution struct {
	Label string
	Short int // 0 = keep original
}

var Resolutions = []Resolution{
	{"원본 크기 유지", 0},
	{"4K (2160p)", 2160},
	{"QHD (1440p)", 1440},
	{"FHD (1080p)", 1080},
	{"HD (720p)", 720},
	{"SD (480p)", 480},
	{"저화질 (360p)", 360},
}

// FPSChoice limits the frame rate (never increases it).
type FPSChoice struct {
	Label string
	FPS   float64 // 0 = keep original
}

var FPSChoices = []FPSChoice{
	{"원본 유지", 0},
	{"최대 60fps", 60},
	{"최대 30fps", 30},
	{"최대 24fps", 24},
}

// AudioChoice controls the audio track(s).
type AudioChoice struct {
	Label string
	Kbps  int // >0 encode at this bitrate, 0 = remove audio, -1 = copy if possible
}

const (
	AudioCopy   = -1
	AudioRemove = 0
)

var AudioChoices = []AudioChoice{
	{"고음질 (192kbps)", 192},
	{"좋음 (160kbps)", 160},
	{"보통 (128kbps)", 128},
	{"작게 (96kbps)", 96},
	{"아주 작게 (64kbps)", 64},
	{"원본 소리 그대로", AudioCopy},
	{"소리 없음 (제거)", AudioRemove},
}

// Quality is expressed on the familiar x264 CRF scale: lower = better
// quality and bigger files. Other encoders map from this scale.
const (
	QualityMin     = 14
	QualityMax     = 38
	QualityDefault = 23
)

// QualityWord gives a human description of a quality value.
func QualityWord(q int) string {
	switch {
	case q <= 17:
		return "최고 화질 · 용량 큼"
	case q <= 20:
		return "아주 좋음"
	case q <= 23:
		return "좋음 (권장)"
	case q <= 26:
		return "보통 · 용량 작음"
	case q <= 30:
		return "낮음 · 용량 아주 작음"
	default:
		return "아주 낮음 · 최소 용량"
	}
}

// Settings is everything the user chose for a batch.
type Settings struct {
	Method      Method
	FormatID    string
	Quality     int     // CRF-like, see QualityMin/Max
	TargetMB    float64 // MethodTarget: desired size per file in MB (1 MB = 1024*1024 bytes)
	AutoScale   bool    // MethodTarget: lower resolution when the bitrate would be too low
	Speed       Speed
	ResIndex    int // index into Resolutions
	FPSIndex    int // index into FPSChoices
	AudioIndex  int // index into AudioChoices
	UseHW       bool
	ToneMapHDR  bool // convert HDR (PQ/HLG) to SDR so colours don't look washed out
	KeepSubs    bool
	OutputDir   string // "" = same folder as the source file
	Suffix      string // appended to the file name, e.g. "_변환"
	OpenWhenEnd bool
}

// DefaultSettings returns the "추천" preset.
func DefaultSettings() Settings {
	s := Settings{Suffix: "_변환", OpenWhenEnd: true, ToneMapHDR: true, KeepSubs: true}
	Presets[0].Apply(&s)
	return s
}

// Format returns the selected format (falling back to the first entry of the
// method's list if the ID does not belong to it).
func (s Settings) Format() Format {
	list := FormatsFor(s.Method)
	for _, f := range list {
		if f.ID == s.FormatID {
			return f
		}
	}
	return list[0]
}

func (s Settings) Resolution() Resolution {
	if s.ResIndex < 0 || s.ResIndex >= len(Resolutions) {
		return Resolutions[0]
	}
	return Resolutions[s.ResIndex]
}

func (s Settings) FPS() FPSChoice {
	if s.FPSIndex < 0 || s.FPSIndex >= len(FPSChoices) {
		return FPSChoices[0]
	}
	return FPSChoices[s.FPSIndex]
}

func (s Settings) Audio() AudioChoice {
	if s.AudioIndex < 0 || s.AudioIndex >= len(AudioChoices) {
		return AudioChoices[2]
	}
	return AudioChoices[s.AudioIndex]
}

// Normalize clamps every field into a valid range.
func (s *Settings) Normalize() {
	if s.Method < MethodQuality || s.Method > MethodAudio {
		s.Method = MethodQuality
	}
	s.FormatID = s.Format().ID
	if s.Quality < QualityMin || s.Quality > QualityMax {
		s.Quality = QualityDefault
	}
	if s.TargetMB <= 0 {
		s.TargetMB = 50
	}
	if s.Speed < SpeedFast || s.Speed > SpeedSlow {
		s.Speed = SpeedNormal
	}
	if s.ResIndex < 0 || s.ResIndex >= len(Resolutions) {
		s.ResIndex = 0
	}
	if s.FPSIndex < 0 || s.FPSIndex >= len(FPSChoices) {
		s.FPSIndex = 0
	}
	if s.AudioIndex < 0 || s.AudioIndex >= len(AudioChoices) {
		s.AudioIndex = 2
	}
}

// Preset is a named bundle of settings.
type Preset struct {
	Name  string
	Hint  string
	Apply func(s *Settings)
}

func resIndex(short int) int {
	for i, r := range Resolutions {
		if r.Short == short {
			return i
		}
	}
	return 0
}

func audioIndex(kbps int) int {
	for i, a := range AudioChoices {
		if a.Kbps == kbps {
			return i
		}
	}
	return 2
}

// Presets are listed in the "프리셋" combo box. The last entry is the
// "사용자 지정" (custom) marker that is selected automatically when the user
// edits a detail setting.
var Presets = []Preset{
	{
		Name: "추천 — 화질은 그대로, 용량만 줄이기",
		Hint: "MP4(H.264) · 원본 크기 · 어디서나 재생됩니다. 대부분 30~70% 줄어듭니다.",
		Apply: func(s *Settings) {
			s.Method, s.FormatID, s.Quality, s.Speed = MethodQuality, "mp4_h264", 23, SpeedNormal
			s.ResIndex, s.FPSIndex, s.AudioIndex = 0, 0, audioIndex(128)
		},
	},
	{
		Name: "더 작게 — H.265로 크게 줄이기",
		Hint: "MP4(H.265) · 같은 화질에 H.264보다 30~50% 더 작습니다. 최근 휴대폰/PC/TV에서 재생됩니다.",
		Apply: func(s *Settings) {
			s.Method, s.FormatID, s.Quality, s.Speed = MethodQuality, "mp4_hevc", 23, SpeedNormal
			s.ResIndex, s.FPSIndex, s.AudioIndex = 0, 0, audioIndex(128)
		},
	},
	{
		Name: "최대 압축 — 720p + H.265 (용량 최소)",
		Hint: "해상도를 720p로 낮추고 H.265로 압축합니다. 보관·공유용으로 용량이 가장 작아집니다.",
		Apply: func(s *Settings) {
			s.Method, s.FormatID, s.Quality, s.Speed = MethodQuality, "mp4_hevc", 26, SpeedNormal
			s.ResIndex, s.FPSIndex, s.AudioIndex = resIndex(720), 0, audioIndex(96)
		},
	},
	{
		Name: "메신저·카톡 전송용 — 720p H.264",
		Hint: "카카오톡·문자·메일로 보내기 좋게 720p, H.264로 작게 만듭니다.",
		Apply: func(s *Settings) {
			s.Method, s.FormatID, s.Quality, s.Speed = MethodQuality, "mp4_h264", 26, SpeedNormal
			s.ResIndex, s.FPSIndex, s.AudioIndex = resIndex(720), 2, audioIndex(128)
		},
	},
	{
		Name: "고화질 보관용 — 화질 우선 (H.265)",
		Hint: "화질 손실을 거의 느낄 수 없게 압축합니다. 시간이 조금 더 걸립니다.",
		Apply: func(s *Settings) {
			s.Method, s.FormatID, s.Quality, s.Speed = MethodQuality, "mp4_hevc", 19, SpeedSlow
			s.ResIndex, s.FPSIndex, s.AudioIndex = 0, 0, audioIndex(192)
		},
	},
	{
		Name: "목표 용량 맞추기 — 원하는 MB 이하로",
		Hint: "각 파일이 지정한 용량(MB) 이하가 되도록 비트레이트를 계산해 2번 나눠 인코딩합니다.",
		Apply: func(s *Settings) {
			s.Method, s.FormatID, s.Speed = MethodTarget, "mp4_h264", SpeedNormal
			s.ResIndex, s.FPSIndex, s.AudioIndex, s.AutoScale = 0, 0, audioIndex(128), true
			if s.TargetMB <= 0 {
				s.TargetMB = 50
			}
		},
	},
	{
		Name: "형식만 바꾸기 — 재압축 없이 빠르게",
		Hint: "영상·소리를 다시 압축하지 않고 컨테이너만 바꿉니다 (예: MKV → MP4). 화질 손실이 없고 몇 초면 끝납니다.",
		Apply: func(s *Settings) {
			s.Method, s.FormatID = MethodRemux, "remux_mp4"
			s.AudioIndex = audioIndex(AudioCopy)
		},
	},
	{
		Name: "소리만 추출 — MP3",
		Hint: "동영상에서 소리만 뽑아 MP3 파일로 저장합니다.",
		Apply: func(s *Settings) {
			s.Method, s.FormatID = MethodAudio, "aud_mp3"
			s.AudioIndex = audioIndex(192)
		},
	},
	{
		Name:  "사용자 지정",
		Hint:  "아래 세부 설정을 직접 바꾼 상태입니다.",
		Apply: func(s *Settings) {},
	},
}

// CustomPresetIndex is the index of the "사용자 지정" entry.
var CustomPresetIndex = len(Presets) - 1

// MatchPreset returns the index of the preset whose settings equal s
// (ignoring output options), or CustomPresetIndex.
func MatchPreset(s Settings) int {
	for i := 0; i < CustomPresetIndex; i++ {
		t := s
		Presets[i].Apply(&t)
		if presetKey(t) == presetKey(s) {
			return i
		}
	}
	return CustomPresetIndex
}

func presetKey(s Settings) string {
	switch s.Method {
	case MethodRemux:
		return fmt.Sprintf("%d|%s|%d", s.Method, s.FormatID, s.AudioIndex)
	case MethodAudio:
		return fmt.Sprintf("%d|%s|%d", s.Method, s.FormatID, s.AudioIndex)
	case MethodTarget:
		return fmt.Sprintf("%d|%s|%d|%d|%d|%d|%v", s.Method, s.FormatID, s.Speed, s.ResIndex, s.FPSIndex, s.AudioIndex, s.AutoScale)
	default:
		return fmt.Sprintf("%d|%s|%d|%d|%d|%d|%d", s.Method, s.FormatID, s.Quality, s.Speed, s.ResIndex, s.FPSIndex, s.AudioIndex)
	}
}
