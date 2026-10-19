package engine

// VideoCodec identifies the video encoder family used for an output format.
type VideoCodec int

const (
	VNone VideoCodec = iota
	VH264
	VHEVC
	VAV1
	VVP9
	VMPEG4 // MPEG-4 Part 2 (Xvid compatible), for old devices / AVI
)

// AudioCodec identifies the audio encoder used for an output format.
type AudioCodec int

const (
	ANone AudioCodec = iota
	AAAC
	AOpus
	AMP3
	APCM  // 16-bit PCM (WAV)
	AFLAC // lossless
)

// Format describes one entry of the "출력 형식" (output format) list.
type Format struct {
	ID    string // stable key stored in settings
	Label string // Korean label shown in the UI
	Ext   string // output file extension including the dot
	Muxer string // ffmpeg muxer name (-f)
	Video VideoCodec
	Audio AudioCodec
}

// VideoFormats are used by the quality and target-size methods.
var VideoFormats = []Format{
	{ID: "mp4_h264", Label: "MP4 · H.264  — 어디서나 재생 (추천)", Ext: ".mp4", Muxer: "mp4", Video: VH264, Audio: AAAC},
	{ID: "mp4_hevc", Label: "MP4 · H.265(HEVC)  — 같은 화질에 더 작게", Ext: ".mp4", Muxer: "mp4", Video: VHEVC, Audio: AAAC},
	{ID: "mp4_av1", Label: "MP4 · AV1  — 가장 작게 (최신 기기, 느림)", Ext: ".mp4", Muxer: "mp4", Video: VAV1, Audio: AAAC},
	{ID: "mkv_h264", Label: "MKV · H.264", Ext: ".mkv", Muxer: "matroska", Video: VH264, Audio: AAAC},
	{ID: "mkv_hevc", Label: "MKV · H.265(HEVC)", Ext: ".mkv", Muxer: "matroska", Video: VHEVC, Audio: AAAC},
	{ID: "webm_vp9", Label: "WebM · VP9  — 웹/브라우저용", Ext: ".webm", Muxer: "webm", Video: VVP9, Audio: AOpus},
	{ID: "mov_h264", Label: "MOV · H.264  — 아이폰/맥/편집용", Ext: ".mov", Muxer: "mov", Video: VH264, Audio: AAAC},
	{ID: "avi_xvid", Label: "AVI · Xvid  — 오래된 기기/TV용", Ext: ".avi", Muxer: "avi", Video: VMPEG4, Audio: AMP3},
}

// RemuxFormats are containers used by the "형식만 바꾸기" (copy streams) method.
// Video/Audio codecs here are the fallbacks used only when a stream cannot be
// copied into the container as-is.
var RemuxFormats = []Format{
	{ID: "remux_mp4", Label: "MP4", Ext: ".mp4", Muxer: "mp4", Video: VH264, Audio: AAAC},
	{ID: "remux_mkv", Label: "MKV", Ext: ".mkv", Muxer: "matroska", Video: VH264, Audio: AAAC},
	{ID: "remux_mov", Label: "MOV", Ext: ".mov", Muxer: "mov", Video: VH264, Audio: AAAC},
	{ID: "remux_webm", Label: "WebM", Ext: ".webm", Muxer: "webm", Video: VVP9, Audio: AOpus},
	{ID: "remux_avi", Label: "AVI", Ext: ".avi", Muxer: "avi", Video: VMPEG4, Audio: AMP3},
	{ID: "remux_ts", Label: "TS (MPEG-TS)", Ext: ".ts", Muxer: "mpegts", Video: VH264, Audio: AAAC},
}

// AudioFormats are used by the "소리만 추출" (extract audio) method.
var AudioFormats = []Format{
	{ID: "aud_mp3", Label: "MP3  — 어디서나 재생", Ext: ".mp3", Muxer: "mp3", Audio: AMP3},
	{ID: "aud_m4a", Label: "M4A(AAC)  — 아이폰/고음질", Ext: ".m4a", Muxer: "mp4", Audio: AAAC},
	{ID: "aud_wav", Label: "WAV  — 무손실 (용량 큼)", Ext: ".wav", Muxer: "wav", Audio: APCM},
	{ID: "aud_flac", Label: "FLAC  — 무손실 압축", Ext: ".flac", Muxer: "flac", Audio: AFLAC},
}

// FormatsFor returns the format list that applies to a method.
func FormatsFor(m Method) []Format {
	switch m {
	case MethodRemux:
		return RemuxFormats
	case MethodAudio:
		return AudioFormats
	default:
		return VideoFormats
	}
}

// FindFormat looks a format up by ID in every list.
func FindFormat(id string) (Format, bool) {
	for _, list := range [][]Format{VideoFormats, RemuxFormats, AudioFormats} {
		for _, f := range list {
			if f.ID == id {
				return f, true
			}
		}
	}
	return Format{}, false
}

// FormatIndex returns the index of id within the list for method m, or 0.
func FormatIndex(m Method, id string) int {
	for i, f := range FormatsFor(m) {
		if f.ID == id {
			return i
		}
	}
	return 0
}
