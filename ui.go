//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tailscale/walk"
	. "github.com/tailscale/walk/declarative"

	"vidpress/engine"
)

var (
	colorGray    = walk.RGB(105, 105, 105)
	colorGreen   = walk.RGB(16, 124, 16)
	colorRed     = walk.RGB(196, 43, 28)
	colorBlue    = walk.RGB(0, 95, 184)
	colorBarFill = walk.RGB(186, 220, 252)
)

const uiFont = "Malgun Gothic"

func presetNames() []string {
	n := make([]string, len(engine.Presets))
	for i, p := range engine.Presets {
		n[i] = p.Name
	}
	return n
}

func formatLabels(m engine.Method) []string {
	list := engine.FormatsFor(m)
	n := make([]string, len(list))
	for i, f := range list {
		n[i] = f.Label
	}
	return n
}

func resolutionLabels() []string {
	n := make([]string, len(engine.Resolutions))
	for i, r := range engine.Resolutions {
		n[i] = r.Label
	}
	return n
}

func fpsLabels() []string {
	n := make([]string, len(engine.FPSChoices))
	for i, f := range engine.FPSChoices {
		n[i] = f.Label
	}
	return n
}

func audioLabels() []string {
	n := make([]string, len(engine.AudioChoices))
	for i, c := range engine.AudioChoices {
		n[i] = c.Label
	}
	return n
}

// The slider runs from "small file" (left) to "high quality" (right), i.e.
// the inverse of the CRF scale.
func sliderFromQuality(q int) int { return engine.QualityMin + engine.QualityMax - q }
func qualityFromSlider(v int) int { return engine.QualityMin + engine.QualityMax - v }

func (a *App) buildWindow() error {
	bold := Font{Family: uiFont, PointSize: 9, Bold: true}
	err := MainWindow{
		AssignTo:    &a.mw,
		Title:       "VidPress — 동영상 용량 줄이기 · 형식 바꾸기",
		Icon:        "APP",
		Size:        Size{Width: 1060, Height: 740},
		MinSize:     Size{Width: 900, Height: 600},
		Font:        Font{Family: uiFont, PointSize: 9},
		Layout:      VBox{Margins: Margins{Left: 12, Top: 8, Right: 12, Bottom: 10}, Spacing: 6},
		OnDropFiles: a.onDropFiles,
		Children: []Widget{
			// Header: title + list buttons
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 8},
				Children: []Widget{
					ImageView{Image: "APP", Mode: ImageViewModeShrink, MinSize: Size{Width: 28, Height: 28}, MaxSize: Size{Width: 28, Height: 28}},
					Label{Text: "VidPress", Font: Font{Family: uiFont, PointSize: 13, Bold: true}},
					Label{Text: "파일을 이 창으로 끌어다 놓거나 [파일 추가]를 누르세요 · 여러 개를 한꺼번에 변환할 수 있습니다", TextColor: colorGray,
						EllipsisMode: EllipsisEnd},
					HSpacer{},
					PushButton{AssignTo: &a.btnAddFiles, Text: "＋ 파일 추가…", Font: bold, MinSize: Size{Width: 112, Height: 30}, OnClicked: a.onAddFiles},
					PushButton{AssignTo: &a.btnAddFolder, Text: "폴더 추가…", MinSize: Size{Width: 88, Height: 30}, OnClicked: a.onAddFolder},
					PushButton{AssignTo: &a.btnRemove, Text: "선택 삭제", MinSize: Size{Width: 80, Height: 30}, OnClicked: a.onRemoveSelected},
					PushButton{AssignTo: &a.btnClear, Text: "목록 비우기", MinSize: Size{Width: 88, Height: 30}, OnClicked: a.onClear},
				},
			},
			TableView{
				AssignTo:                 &a.tv,
				Model:                    a.model,
				MultiSelection:           true,
				AlternatingRowBG:         true,
				NotSortableByHeaderClick: true,
				StretchFactor:            3,
				MinSize:                  Size{Height: 110},
				Columns: []TableViewColumn{
					{Title: "파일 이름", Width: 260},
					{Title: "크기", Width: 72, Alignment: AlignFar},
					{Title: "길이", Width: 62, Alignment: AlignFar},
					{Title: "해상도", Width: 84},
					{Title: "코덱", Width: 100},
					{Title: "상태", Width: 150},
					{Title: "결과", Width: 250},
				},
				StyleCell:       a.styleCell,
				OnItemActivated: a.onItemActivated,
				OnKeyDown: func(key walk.Key) {
					if key == walk.KeyDelete {
						a.onRemoveSelected()
					}
				},
				ContextMenuItems: []MenuItem{
					Action{Text: "결과 파일 위치 열기", OnTriggered: a.onOpenResult},
					Action{Text: "원본 파일 위치 열기", OnTriggered: a.onOpenSource},
					Separator{},
					Action{Text: "다시 변환하기 (대기로 되돌리기)", OnTriggered: a.onRequeue},
					Action{Text: "오류 내용 보기", OnTriggered: a.onShowError},
					Separator{},
					Action{Text: "목록에서 삭제", OnTriggered: a.onRemoveSelected},
				},
			},
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 8},
				Children: []Widget{
					GroupBox{
						AssignTo:      &a.settingsBox,
						Title:         "변환 설정",
						StretchFactor: 3,
						Alignment:     AlignHNearVNear,
						Layout:        Grid{Columns: 4, Spacing: 6, Margins: Margins{Left: 10, Top: 4, Right: 10, Bottom: 8}},
						Children: []Widget{
							Label{Text: "프리셋(용도):", Font: bold},
							ComboBox{AssignTo: &a.cbPreset, Model: presetNames(), ColumnSpan: 3, Font: bold, OnCurrentIndexChanged: a.onPresetChanged},

							Label{Text: ""},
							Label{AssignTo: &a.lblHint, ColumnSpan: 3, TextColor: colorBlue, EllipsisMode: EllipsisEnd},

							Label{Text: "방식:"},
							ComboBox{AssignTo: &a.cbMethod, Model: engine.MethodLabels, OnCurrentIndexChanged: a.onMethodChanged},
							Label{Text: "출력 형식:"},
							ComboBox{AssignTo: &a.cbFormat, Model: formatLabels(engine.MethodQuality), MinSize: Size{Width: 255}, OnCurrentIndexChanged: a.onDetailChanged},

							Label{Text: "화질:"},
							Composite{
								Layout: HBox{MarginsZero: true, Spacing: 4},
								Children: []Widget{
									Label{Text: "작게", TextColor: colorGray},
									Slider{AssignTo: &a.slQuality, MinValue: engine.QualityMin, MaxValue: engine.QualityMax, Tracking: true,
										ToolTipsHidden: true, MinSize: Size{Width: 120}, OnValueChanged: a.onDetailChanged},
									Label{Text: "고화질", TextColor: colorGray},
									Label{AssignTo: &a.lblQuality, MinSize: Size{Width: 128}},
								},
							},
							Label{Text: "해상도:"},
							ComboBox{AssignTo: &a.cbRes, Model: resolutionLabels(), MinSize: Size{Width: 255}, OnCurrentIndexChanged: a.onDetailChanged},

							Label{Text: "목표 용량:"},
							Composite{
								Layout: HBox{MarginsZero: true, Spacing: 8},
								Children: []Widget{
									NumberEdit{AssignTo: &a.neTarget, MinValue: 1, MaxValue: 100000, Decimals: 0, Increment: 5,
										SpinButtonsVisible: true, Suffix: " MB", MinSize: Size{Width: 100}, MaxSize: Size{Width: 110},
										OnValueChanged: a.onDetailChanged},
									CheckBox{AssignTo: &a.chkAutoScale, Text: "필요하면 해상도 자동 조정", OnCheckedChanged: a.onDetailChanged,
										ToolTipText: "목표 용량이 영상 길이에 비해 작으면, 깨져 보이지 않도록 해상도를 알맞게 낮춥니다."},
									HSpacer{},
								},
							},
							Label{Text: "프레임:"},
							ComboBox{AssignTo: &a.cbFPS, Model: fpsLabels(), OnCurrentIndexChanged: a.onDetailChanged},

							Label{Text: "인코딩 속도:"},
							ComboBox{AssignTo: &a.cbSpeed, Model: engine.SpeedLabels, OnCurrentIndexChanged: a.onDetailChanged},
							Label{Text: "소리:"},
							ComboBox{AssignTo: &a.cbAudio, Model: audioLabels(), OnCurrentIndexChanged: a.onDetailChanged},

							Label{Text: "추가 옵션:"},
							Composite{
								ColumnSpan: 3,
								Layout:     HBox{MarginsZero: true, Spacing: 16},
								Children: []Widget{
									CheckBox{AssignTo: &a.chkHW, Text: "하드웨어 가속 (확인 중…)", OnCheckedChanged: a.onDetailChanged,
										ToolTipText: "그래픽카드(NVIDIA·Intel·AMD)의 인코더로 훨씬 빠르게 변환합니다. 같은 화질이면 용량이 조금 더 클 수 있습니다."},
									CheckBox{AssignTo: &a.chkHDR, Text: "HDR 영상 색 보정", OnCheckedChanged: a.onDetailChanged,
										ToolTipText: "아이폰 HDR 같은 HDR(10비트) 영상을 일반 화면용(SDR) 색으로 바꿔, 색이 바래 보이지 않게 합니다."},
									CheckBox{AssignTo: &a.chkSubs, Text: "자막 유지", OnCheckedChanged: a.onDetailChanged,
										ToolTipText: "원본에 들어 있는 자막 트랙을 함께 넣습니다 (MKV는 모든 자막, MP4/MOV는 텍스트 자막)."},
									HSpacer{},
								},
							},
						},
					},
					Composite{
						StretchFactor: 1,
						Alignment:     AlignHNearVNear,
						MinSize:       Size{Width: 250},
						MaxSize:       Size{Width: 300},
						Layout:        VBox{MarginsZero: true, Spacing: 8},
						Children: []Widget{
							GroupBox{
								AssignTo: &a.outputBox,
								Title:    "저장",
								Layout:   VBox{Margins: Margins{Left: 10, Top: 4, Right: 10, Bottom: 8}, Spacing: 6},
								Children: []Widget{
									ComboBox{AssignTo: &a.cbOutMode, Model: []string{"원본 파일과 같은 폴더에 저장", "직접 고른 폴더에 저장"},
										OnCurrentIndexChanged: a.onOutModeChanged},
									Composite{
										Layout: HBox{MarginsZero: true, Spacing: 4},
										Children: []Widget{
											LineEdit{AssignTo: &a.leOutDir, ReadOnly: true, CueBanner: "폴더를 고르세요"},
											PushButton{AssignTo: &a.btnBrowse, Text: "찾아보기…", MaxSize: Size{Width: 80}, OnClicked: a.onBrowseOutDir},
										},
									},
									Composite{
										Layout: HBox{MarginsZero: true, Spacing: 6},
										Children: []Widget{
											Label{Text: "이름 뒤에 붙일 말:"},
											LineEdit{AssignTo: &a.leSuffix, MaxLength: 40, OnTextChanged: a.onDetailChanged,
												ToolTipText: "예) 영상.mp4 → 영상_변환.mp4. 같은 이름이 있으면 (2), (3)… 을 붙여 덮어쓰지 않습니다."},
										},
									},
									CheckBox{AssignTo: &a.chkOpen, Text: "모두 끝나면 저장 폴더 열기", OnCheckedChanged: a.onDetailChanged},
								},
							},
							VSpacer{},
							PushButton{AssignTo: &a.btnStart, Text: "▶  변환 시작", Font: Font{Family: uiFont, PointSize: 12, Bold: true},
								MinSize: Size{Height: 48}, OnClicked: a.onStart},
							PushButton{AssignTo: &a.btnStop, Text: "■  중지", MinSize: Size{Height: 30}, Enabled: false, OnClicked: a.onStop},
						},
					},
				},
			},
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 10},
				Children: []Widget{
					Label{AssignTo: &a.lblStatus, Font: bold, EllipsisMode: EllipsisEnd, StretchFactor: 1},
				},
			},
			ProgressBar{AssignTo: &a.pbTotal, MinValue: 0, MaxValue: 1000, MinSize: Size{Height: 20}, MaxSize: Size{Height: 20}},
			Label{AssignTo: &a.lblSummary, TextColor: colorGray, EllipsisMode: EllipsisEnd},
			TextEdit{AssignTo: &a.teLog, ReadOnly: true, VScroll: true, MinSize: Size{Height: 58}, MaxSize: Size{Height: 58},
				Font: Font{Family: uiFont, PointSize: 8}},
		},
	}.Create()
	if err != nil {
		return err
	}

	a.teLog.SetMaxLength(0) // 0 = no practical limit for multi-line edits
	if gl, ok := a.settingsBox.Layout().(*walk.GridLayout); ok {
		gl.SetColumnStretchFactor(1, 3)
		gl.SetColumnStretchFactor(3, 2)
	}
	a.mw.Closing().Attach(a.onClosing)
	// walk exits the application on every WM_CLOSE, even when a Closing
	// handler cancels it (then it crashes later). Exit only once the window
	// is really going away.
	a.mw.SetExitOnClose(false)
	a.mw.Disposing().Attach(func() { a.app.Exit(0) })
	// Let the "결과" column take the remaining width. (Done by hand instead of
	// LastColumnStretched, which can fail while the table has no size yet.)
	a.tv.SizeChanged().Attach(func() { a.tv.StretchLastColumn() })
	a.applySettingsToControls()
	a.fileLog("window: " + fitAndCenter(a.mw.Handle()))
	return nil
}

// ---------------------------------------------------------------------------
// Settings <-> controls

func setCombo(cb *walk.ComboBox, i int) {
	if cb.CurrentIndex() != i {
		cb.SetCurrentIndex(i)
	}
}

func (a *App) applySettingsToControls() {
	a.updating = true
	defer func() { a.updating = false }()
	s := a.settings
	s.Normalize()
	a.settings = s

	setCombo(a.cbMethod, int(s.Method))
	a.cbFormat.SetModel(formatLabels(s.Method))
	a.cbFormat.SetCurrentIndex(engine.FormatIndex(s.Method, s.FormatID))
	a.slQuality.SetValue(sliderFromQuality(s.Quality))
	a.neTarget.SetValue(s.TargetMB)
	a.chkAutoScale.SetChecked(s.AutoScale)
	setCombo(a.cbRes, s.ResIndex)
	setCombo(a.cbFPS, s.FPSIndex)
	setCombo(a.cbSpeed, int(s.Speed))
	setCombo(a.cbAudio, s.AudioIndex)
	a.chkHW.SetChecked(s.UseHW)
	a.chkHDR.SetChecked(s.ToneMapHDR)
	a.chkSubs.SetChecked(s.KeepSubs)
	if s.OutputDir == "" {
		setCombo(a.cbOutMode, 0)
		a.leOutDir.SetText("")
	} else {
		setCombo(a.cbOutMode, 1)
		a.leOutDir.SetText(s.OutputDir)
	}
	if a.leSuffix.Text() != s.Suffix {
		a.leSuffix.SetText(s.Suffix)
	}
	a.chkOpen.SetChecked(s.OpenWhenEnd)
	setCombo(a.cbPreset, engine.MatchPreset(s))
	a.updateHWCheckbox()
	a.updateDerivedLabels()
	a.updateEnabled()
}

// readControls builds Settings from the controls.
func (a *App) readControls() engine.Settings {
	s := a.settings
	s.Method = engine.Method(a.cbMethod.CurrentIndex())
	if list := engine.FormatsFor(s.Method); a.cbFormat.CurrentIndex() >= 0 && a.cbFormat.CurrentIndex() < len(list) {
		s.FormatID = list[a.cbFormat.CurrentIndex()].ID
	}
	s.Quality = qualityFromSlider(a.slQuality.Value())
	s.TargetMB = a.neTarget.Value()
	s.AutoScale = a.chkAutoScale.Checked()
	s.ResIndex = a.cbRes.CurrentIndex()
	s.FPSIndex = a.cbFPS.CurrentIndex()
	s.Speed = engine.Speed(a.cbSpeed.CurrentIndex())
	s.AudioIndex = a.cbAudio.CurrentIndex()
	if a.chkHW.Enabled() || !a.hwReady {
		s.UseHW = a.chkHW.Checked()
	}
	s.ToneMapHDR = a.chkHDR.Checked()
	s.KeepSubs = a.chkSubs.Checked()
	if a.cbOutMode.CurrentIndex() == 1 {
		s.OutputDir = a.leOutDir.Text()
	} else {
		s.OutputDir = ""
	}
	s.Suffix = a.leSuffix.Text()
	s.OpenWhenEnd = a.chkOpen.Checked()
	s.Normalize()
	return s
}

func (a *App) onDetailChanged() {
	if a.updating {
		return
	}
	a.settings = a.readControls()
	a.updating = true
	setCombo(a.cbPreset, engine.MatchPreset(a.settings))
	a.updating = false
	a.updateDerivedLabels()
	a.updateEnabled()
}

func (a *App) onPresetChanged() {
	if a.updating {
		return
	}
	i := a.cbPreset.CurrentIndex()
	if i < 0 || i >= engine.CustomPresetIndex {
		a.updateDerivedLabels()
		return
	}
	s := a.readControls()
	engine.Presets[i].Apply(&s)
	a.settings = s
	a.applySettingsToControls()
}

func (a *App) onMethodChanged() {
	if a.updating {
		return
	}
	s := a.readControls()
	m := engine.Method(a.cbMethod.CurrentIndex())
	if _, ok := findIn(engine.FormatsFor(m), s.FormatID); !ok {
		s.FormatID = engine.FormatsFor(m)[0].ID
	}
	switch m {
	case engine.MethodRemux:
		s.AudioIndex = audioChoiceIndex(engine.AudioCopy)
	case engine.MethodAudio:
		if s.Audio().Kbps <= 0 {
			s.AudioIndex = audioChoiceIndex(192)
		}
	case engine.MethodTarget:
		if s.Audio().Kbps == engine.AudioCopy {
			s.AudioIndex = audioChoiceIndex(128)
		}
	}
	s.Method = m
	a.settings = s
	a.applySettingsToControls()
}

func findIn(list []engine.Format, id string) (int, bool) {
	for i, f := range list {
		if f.ID == id {
			return i, true
		}
	}
	return 0, false
}

func audioChoiceIndex(kbps int) int {
	for i, c := range engine.AudioChoices {
		if c.Kbps == kbps {
			return i
		}
	}
	return 2
}

func (a *App) onOutModeChanged() {
	if a.updating {
		return
	}
	if a.cbOutMode.CurrentIndex() == 1 && a.leOutDir.Text() == "" {
		if !a.onBrowseOutDirInner() {
			a.updating = true
			a.cbOutMode.SetCurrentIndex(0)
			a.updating = false
		}
	}
	a.onDetailChanged()
}

func (a *App) onBrowseOutDir() {
	if a.onBrowseOutDirInner() {
		a.updating = true
		a.cbOutMode.SetCurrentIndex(1)
		a.updating = false
		a.onDetailChanged()
	}
}

func (a *App) onBrowseOutDirInner() bool {
	initial := a.leOutDir.Text()
	if initial == "" {
		initial = a.cfg.LastAddDir
	}
	dir, ok := pickFolder(a.mw, "변환한 파일을 저장할 폴더를 고르세요", initial)
	if !ok {
		return false
	}
	a.leOutDir.SetText(dir)
	return true
}

// updateDerivedLabels refreshes the hint and quality text.
func (a *App) updateDerivedLabels() {
	q := qualityFromSlider(a.slQuality.Value())
	a.lblQuality.SetText(fmt.Sprintf("%d · %s", q, engine.QualityWord(q)))

	i := a.cbPreset.CurrentIndex()
	hint := ""
	if i >= 0 && i < engine.CustomPresetIndex {
		hint = engine.Presets[i].Hint
	} else {
		hint = a.customSummary()
	}
	if a.settings.UseHW && a.hwReady && engine.HWVendorText(a.hw) != "" &&
		(a.settings.Method == engine.MethodQuality || a.settings.Method == engine.MethodTarget) {
		hint += "  (하드웨어 가속 사용)"
	}
	a.lblHint.SetText(hint)
	a.lblHint.SetToolTipText(hint)
}

func (a *App) customSummary() string {
	s := a.settings
	f := s.Format()
	label := strings.TrimSpace(strings.SplitN(f.Label, "—", 2)[0])
	switch s.Method {
	case engine.MethodRemux:
		return "사용자 지정: " + label + " 형식으로 재압축 없이 옮깁니다"
	case engine.MethodAudio:
		return "사용자 지정: 소리만 " + label + " 파일로 저장합니다"
	case engine.MethodTarget:
		return fmt.Sprintf("사용자 지정: %s · 파일당 %.0fMB 이하 · %s", label, s.TargetMB, s.Resolution().Label)
	}
	return fmt.Sprintf("사용자 지정: %s · 화질 %d · %s · %s", label, s.Quality, s.Resolution().Label, engine.SpeedLabels[s.Speed])
}

func (a *App) updateHWCheckbox() {
	switch {
	case a.toolsErr != nil:
		a.chkHW.SetText("하드웨어 가속 (사용 불가)")
	case !a.hwReady:
		a.chkHW.SetText("하드웨어 가속 (확인 중…)")
	case engine.HWVendorText(a.hw) == "":
		a.chkHW.SetText("하드웨어 가속 (지원 장치 없음)")
	default:
		a.chkHW.SetText("하드웨어 가속 · " + engine.HWVendorText(a.hw) + " (빠름)")
	}
}

// updateEnabled enables only the controls that apply to the current method.
func (a *App) updateEnabled() {
	s := a.settings
	idle := !a.running
	enc := s.Method == engine.MethodQuality || s.Method == engine.MethodTarget
	f := s.Format()

	a.cbPreset.SetEnabled(idle)
	a.cbMethod.SetEnabled(idle)
	a.cbFormat.SetEnabled(idle)
	a.slQuality.SetEnabled(idle && s.Method == engine.MethodQuality)
	a.lblQuality.SetEnabled(s.Method == engine.MethodQuality)
	a.neTarget.SetEnabled(idle && s.Method == engine.MethodTarget)
	a.chkAutoScale.SetEnabled(idle && s.Method == engine.MethodTarget)
	a.cbRes.SetEnabled(idle && enc)
	a.cbFPS.SetEnabled(idle && enc)
	a.cbSpeed.SetEnabled(idle && enc)
	a.cbAudio.SetEnabled(idle && s.Method != engine.MethodRemux &&
		!(s.Method == engine.MethodAudio && (f.Audio == engine.APCM || f.Audio == engine.AFLAC)))
	hwOK := a.hwReady && engine.HWVendorText(a.hw) != "" && engine.HWSupports(a.hw, f.Video)
	a.chkHW.SetEnabled(idle && enc && hwOK)
	if a.hwReady {
		// Show the box unticked when acceleration cannot be used for this
		// format (the saved preference is kept for when it can).
		if want := s.UseHW && hwOK; a.chkHW.Checked() != want {
			prev := a.updating
			a.updating = true
			a.chkHW.SetChecked(want)
			a.updating = prev
		}
	}
	a.chkHDR.SetEnabled(idle && enc)
	subsOK := f.Muxer == "mp4" || f.Muxer == "mov" || f.Muxer == "matroska" || f.Muxer == "webm"
	a.chkSubs.SetEnabled(idle && s.Method != engine.MethodAudio && subsOK)

	a.cbOutMode.SetEnabled(idle)
	a.btnBrowse.SetEnabled(idle)
	a.leOutDir.SetEnabled(idle && a.cbOutMode.CurrentIndex() == 1)
	a.leSuffix.SetEnabled(idle)
	a.chkOpen.SetEnabled(true)

	a.btnRemove.SetEnabled(true)
	a.btnClear.SetEnabled(true)
	a.btnStart.SetEnabled(idle && a.toolsErr == nil)
	a.btnStop.SetEnabled(a.running)
}

// ---------------------------------------------------------------------------
// Adding / removing files

func (a *App) onAddFiles() {
	dlg := walk.FileDialog{
		Title:          "변환할 동영상 파일을 고르세요 (여러 개 선택 가능)",
		InitialDirPath: a.cfg.LastAddDir,
		Filter: "동영상·오디오 파일|*.mp4;*.m4v;*.mov;*.mkv;*.avi;*.wmv;*.flv;*.webm;*.ts;*.mts;*.m2ts;*.mpg;*.mpeg;*.3gp;*.ogv;*.vob;*.asf;*.f4v;*.rm;*.rmvb;*.divx;*.mxf;*.dv;*.gif;*.mp3;*.m4a;*.wav;*.flac;*.aac;*.ogg;*.opus;*.wma" +
			"|동영상 파일|*.mp4;*.m4v;*.mov;*.mkv;*.avi;*.wmv;*.flv;*.webm;*.ts;*.mts;*.m2ts;*.mpg;*.mpeg;*.3gp;*.ogv;*.vob;*.asf;*.f4v;*.rm;*.rmvb;*.divx;*.mxf;*.dv" +
			"|모든 파일 (*.*)|*.*",
	}
	ok, err := dlg.ShowOpenMultiple(a.mw)
	if err != nil {
		a.log("파일 선택 창 오류: " + err.Error())
		return
	}
	if !ok || len(dlg.FilePaths) == 0 {
		return
	}
	a.cfg.LastAddDir = filepath.Dir(dlg.FilePaths[0])
	a.addPaths(dlg.FilePaths)
}

func (a *App) onAddFolder() {
	dir, ok := pickFolder(a.mw, "동영상이 들어 있는 폴더를 고르세요 (하위 폴더 포함)", a.cfg.LastAddDir)
	if !ok {
		return
	}
	a.cfg.LastAddDir = dir
	a.addPaths([]string{dir})
}

func (a *App) onDropFiles(files []string) {
	a.addPaths(files)
}

// addPaths adds files, expanding folders recursively.
func (a *App) addPaths(paths []string) {
	var files []string
	skipped := 0
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			skipped++
			continue
		}
		if !st.IsDir() {
			files = append(files, p)
			continue
		}
		var found []string
		filepath.WalkDir(p, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if engine.IsMediaFile(path) && !strings.Contains(filepath.Base(path), ".변환중.") {
				found = append(found, path)
			}
			return nil
		})
		sort.Strings(found)
		files = append(files, found...)
	}
	added := a.model.Add(files)
	if len(added) == 0 {
		if len(files) > 0 {
			a.setStatus("이미 목록에 있는 파일입니다")
		} else if skipped > 0 || len(paths) > 0 {
			a.setStatus("추가할 동영상 파일을 찾지 못했습니다")
		}
		return
	}
	a.log(fmt.Sprintf("파일 %d개를 추가했습니다", len(added)))
	for _, it := range added {
		a.startProbe(it)
	}
	a.refreshIdleStatus()
}

func (a *App) startProbe(it *Item) {
	a.pendingProbes++
	go func() {
		a.probeSem <- struct{}{}
		var info *engine.MediaInfo
		var err error
		if a.toolsErr != nil {
			err = a.toolsErr
		} else {
			info, err = engine.Probe(context.Background(), a.tools.FFprobe, it.Path)
		}
		<-a.probeSem
		a.mw.Synchronize(func() {
			a.pendingProbes--
			a.model.Update(it, func(it *Item) {
				if err != nil {
					it.State, it.ProbeErr = StInvalid, err
				} else {
					it.Info = info
					if it.State == StProbing {
						it.State = StReady
					}
				}
			})
			if err != nil {
				a.log(fmt.Sprintf("읽기 실패: %s — %v", it.Name, err))
			}
			a.model.NotifyChanged(it)
			a.refreshIdleStatus()
			if a.pendingProbes == 0 && a.autoStart && !a.running {
				a.autoStart = false
				a.onStart()
			}
		})
	}()
}

func (a *App) onRemoveSelected() {
	rows := a.tv.SelectedIndexes()
	if len(rows) == 0 {
		return
	}
	a.model.Remove(rows)
	a.refreshIdleStatus()
}

func (a *App) onClear() {
	if a.model.RowCount() == 0 {
		return
	}
	a.model.Clear()
	a.refreshIdleStatus()
}

func (a *App) selectedItems() []*Item {
	var items []*Item
	for _, r := range a.tv.SelectedIndexes() {
		if it := a.model.Item(r); it != nil {
			items = append(items, it)
		}
	}
	return items
}

func (a *App) onOpenResult() {
	for _, it := range a.selectedItems() {
		if it.Result != nil {
			showInExplorer(it.Result.Output)
			return
		}
	}
	a.setStatus("선택한 파일은 아직 변환되지 않았습니다")
}

func (a *App) onOpenSource() {
	if items := a.selectedItems(); len(items) > 0 {
		showInExplorer(items[0].Path)
	}
}

func (a *App) onRequeue() {
	for _, it := range a.selectedItems() {
		a.model.Update(it, func(it *Item) {
			switch it.State {
			case StDone, StFailed, StCanceled:
				it.State, it.Result, it.Err, it.Progress = StReady, nil, nil, 0
			}
		})
		a.model.NotifyChanged(it)
	}
	a.refreshIdleStatus()
}

func (a *App) onShowError() {
	for _, it := range a.selectedItems() {
		if it.Err != nil || it.ProbeErr != nil {
			a.showItemError(it)
			return
		}
	}
}

func (a *App) showItemError(it *Item) {
	err := it.Err
	if err == nil {
		err = it.ProbeErr
	}
	if err == nil {
		return
	}
	walk.MsgBox(a.mw, "오류 내용 — "+it.Name, err.Error()+"\n\n※ 다른 출력 형식이나 '추천' 프리셋으로 다시 시도해 보세요. 자세한 기록은 아래 작업 기록 창에 있습니다.", walk.MsgBoxIconWarning)
}

func (a *App) onItemActivated() {
	it := a.model.Item(a.tv.CurrentIndex())
	if it == nil {
		return
	}
	switch it.State {
	case StDone:
		if it.Result != nil {
			showInExplorer(it.Result.Output)
		}
	case StFailed, StInvalid:
		a.showItemError(it)
	default:
		showInExplorer(it.Path)
	}
}

// styleCell colours the status column and draws a progress bar for the item
// being converted.
func (a *App) styleCell(style *walk.CellStyle) {
	row, col := style.Row(), style.Col()
	if col != colStatus && col != colResult {
		return
	}
	it := a.model.Item(row)
	if it == nil {
		return
	}
	a.model.mu.Lock()
	state, progress, text := it.State, it.Progress, statusText(it)
	a.model.mu.Unlock()

	switch state {
	case StDone:
		style.TextColor = colorGreen
	case StFailed, StInvalid:
		style.TextColor = colorRed
	case StCanceled, StProbing:
		style.TextColor = colorGray
	case StRunning:
		if col != colStatus {
			return
		}
		canvas := style.Canvas()
		if canvas == nil {
			style.TextColor = colorBlue
			return
		}
		b := style.BoundsPixels()
		if bg, err := walk.NewSolidColorBrush(style.BackgroundColor); err == nil {
			canvas.FillRectanglePixels(bg, b)
			bg.Dispose()
		}
		inner := walk.Rectangle{X: b.X + 2, Y: b.Y + 2, Width: b.Width - 4, Height: b.Height - 4}
		if progress > 0 {
			fillW := int(float64(inner.Width) * progress)
			if fillW > inner.Width {
				fillW = inner.Width
			}
			if fill, err := walk.NewSolidColorBrush(colorBarFill); err == nil {
				canvas.FillRectanglePixels(fill, walk.Rectangle{X: inner.X, Y: inner.Y, Width: fillW, Height: inner.Height})
				fill.Dispose()
			}
		}
		if pen, err := walk.NewCosmeticPen(walk.PenSolid, walk.RGB(120, 170, 220)); err == nil {
			canvas.DrawRectanglePixels(pen, inner)
			pen.Dispose()
		}
		txt := walk.Rectangle{X: inner.X + 6, Y: inner.Y, Width: inner.Width - 8, Height: inner.Height}
		canvas.DrawTextPixels(text, a.tv.Font(), walk.RGB(0, 50, 110), txt, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
	}
}

// ---------------------------------------------------------------------------
// Status line

func (a *App) setStatus(s string) {
	if a.lblStatus != nil {
		a.lblStatus.SetText(s)
	}
}

func (a *App) refreshIdleStatus() {
	if a.running {
		return
	}
	st := a.model.Stats(0)
	switch {
	case a.toolsErr != nil:
		a.setStatus("FFmpeg를 찾을 수 없어 변환할 수 없습니다")
	case st.Total == 0:
		a.setStatus("파일을 추가하고 [변환 시작]을 누르세요")
		a.lblSummary.SetText("지원 형식: MP4, MOV, MKV, AVI, WMV, FLV, WebM, TS, MPG, 3GP 등 대부분의 동영상과 MP3·WAV 같은 오디오 파일")
	case st.Probing > 0:
		a.setStatus(fmt.Sprintf("파일 정보를 읽는 중… (%d개 남음)", st.Probing))
	default:
		msg := fmt.Sprintf("파일 %d개 · 총 %s", st.Total, engine.FormatSize(st.TotalSize))
		if st.Ready > 0 {
			msg += fmt.Sprintf(" · 변환 대기 %d개", st.Ready)
		}
		if st.Done > 0 {
			msg += fmt.Sprintf(" · 완료 %d개", st.Done)
		}
		if st.Failed+st.Invalid > 0 {
			msg += fmt.Sprintf(" · 실패 %d개", st.Failed+st.Invalid)
		}
		a.setStatus(msg)
		if st.Ready > 0 {
			a.lblSummary.SetText("설정을 확인하고 [변환 시작]을 누르세요. 원본 파일은 바뀌지 않습니다.")
		}
	}
}
