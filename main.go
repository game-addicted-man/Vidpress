//go:build windows

// VidPress — a small HandBrake-style video compressor/converter for Windows.
// The GUI is built with walk (native Win32 controls); the actual work is done
// by FFmpeg, which ships in the "ffmpeg" folder next to VidPress.exe.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tailscale/walk"

	"vidpress/engine"
)

// appVersion is set at build time for releases:
//
//	go build -ldflags "-X main.appVersion=1.2.3" .
var appVersion = "1.0.0"

// App holds the window, its controls and the conversion state.
type App struct {
	app    *walk.Application
	mw     *walk.MainWindow
	appDir string
	cfg    *Config

	tools    engine.Tools
	toolsErr error
	hw       map[string]bool
	hwReady  bool

	model *QueueModel
	tv    *walk.TableView

	// settings controls
	cbPreset, cbMethod, cbFormat, cbRes, cbFPS, cbSpeed, cbAudio, cbOutMode *walk.ComboBox
	lblHint, lblQuality                                                     *walk.Label
	slQuality                                                               *walk.Slider
	neTarget                                                                *walk.NumberEdit
	chkAutoScale, chkHW, chkHDR, chkSubs, chkOpen                           *walk.CheckBox
	leOutDir, leSuffix                                                      *walk.LineEdit
	btnBrowse                                                               *walk.PushButton
	settingsBox, outputBox                                                  *walk.GroupBox

	btnAddFiles, btnAddFolder, btnRemove, btnClear *walk.PushButton
	btnStart, btnStop                              *walk.PushButton
	pbTotal                                        *walk.ProgressBar
	lblStatus, lblSummary                          *walk.Label
	teLog                                          *walk.TextEdit

	updating bool            // true while controls are changed programmatically
	settings engine.Settings // current (UI) settings

	running        bool
	batch          int
	cancel         context.CancelFunc
	closeAfterStop bool

	probeSem      chan struct{}
	pendingProbes int

	// command-line options (also used for automated testing)
	autoStart    bool
	exitWhenDone bool
	startFiles   []string
	argPreset    int
	argOutDir    string

	logMu   sync.Mutex
	logFile *os.File
}

func main() {
	a := &App{probeSem: make(chan struct{}, 3), model: &QueueModel{}, argPreset: -1}
	exe, err := os.Executable()
	if err == nil {
		a.appDir = filepath.Dir(exe)
	}
	a.parseArgs(os.Args[1:])

	a.cfg = loadConfig()
	a.settings = a.cfg.Settings
	if a.argPreset >= 0 {
		engine.Presets[a.argPreset].Apply(&a.settings)
	}
	if a.argOutDir != "" {
		a.settings.OutputDir = a.argOutDir
	}
	a.openLogFile()
	createAppMutex()
	a.tools, a.toolsErr = engine.FindTools(a.appDir)

	app, err := walk.InitApp()
	if err != nil {
		walk.MsgBox(nil, "VidPress", "프로그램을 시작할 수 없습니다: "+err.Error(), walk.MsgBoxIconError)
		return
	}
	a.app = app
	if err := a.buildWindow(); err != nil {
		walk.MsgBox(nil, "VidPress", "창을 만들 수 없습니다: "+err.Error(), walk.MsgBoxIconError)
		return
	}
	a.afterStart()
	app.Run()
	a.saveConfig()
	a.closeLogFile()
}

// parseArgs accepts file/folder paths (e.g. files dropped on VidPress.exe)
// plus a few options:
//
//	--preset=N          select preset N (0-based) on start
//	--outdir=PATH       save results to PATH
//	--autostart         start converting as soon as the files are read
//	--exit-when-done    close the program after the batch finishes
func (a *App) parseArgs(args []string) {
	for _, arg := range args {
		switch {
		case arg == "--autostart":
			a.autoStart = true
		case arg == "--exit-when-done":
			a.exitWhenDone = true
		case strings.HasPrefix(arg, "--preset="):
			if n, err := strconv.Atoi(strings.TrimPrefix(arg, "--preset=")); err == nil && n >= 0 && n < engine.CustomPresetIndex {
				a.argPreset = n
			}
		case strings.HasPrefix(arg, "--outdir="):
			a.argOutDir = strings.TrimPrefix(arg, "--outdir=")
		case strings.HasPrefix(arg, "--"):
			// unknown option: ignore
		default:
			a.startFiles = append(a.startFiles, arg)
		}
	}
}

func (a *App) openLogFile() {
	if err := os.MkdirAll(configDir(), 0o755); err != nil {
		return
	}
	p := filepath.Join(configDir(), "VidPress.log")
	if st, err := os.Stat(p); err == nil && st.Size() > 2<<20 {
		os.Rename(p, p+".old")
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		a.logFile = f
		fmt.Fprintf(f, "\r\n===== VidPress %s 시작 %s =====\r\n", appVersion, time.Now().Format("2006-01-02 15:04:05"))
	}
}

func (a *App) closeLogFile() {
	a.logMu.Lock()
	defer a.logMu.Unlock()
	if a.logFile != nil {
		a.logFile.Close()
		a.logFile = nil
	}
}

// fileLog writes a line to the log file only (any goroutine).
func (a *App) fileLog(line string) {
	a.logMu.Lock()
	defer a.logMu.Unlock()
	if a.logFile != nil {
		fmt.Fprintf(a.logFile, "[%s] %s\r\n", time.Now().Format("15:04:05"), line)
	}
}

// log appends a line to the log panel and the log file (UI thread only).
func (a *App) log(line string) {
	a.fileLog(line)
	if a.teLog == nil {
		return
	}
	text := fmt.Sprintf("[%s] %s\r\n", time.Now().Format("15:04:05"), line)
	if a.teLog.TextLength() > 400000 {
		a.teLog.SetText("")
	}
	l := a.teLog.TextLength()
	a.teLog.SetTextSelection(l, l)
	a.teLog.ReplaceSelectedText(text, false)
	a.teLog.ScrollToCaret()
}

// logAsync is log() for background goroutines.
func (a *App) logAsync(line string) {
	a.mw.Synchronize(func() { a.log(line) })
}

func (a *App) saveConfig() {
	a.cfg.Settings = a.settings
	if err := a.cfg.Save(); err != nil {
		a.fileLog("설정 저장 실패: " + err.Error())
	}
}

// afterStart runs once the window exists.
func (a *App) afterStart() {
	a.log(fmt.Sprintf("VidPress %s 준비 완료", appVersion))
	if a.toolsErr != nil {
		a.log("※ " + a.toolsErr.Error())
		a.setStatus("FFmpeg를 찾을 수 없어 변환할 수 없습니다. 프로그램 폴더를 확인해 주세요.")
		a.btnStart.SetEnabled(false)
		walk.MsgBox(a.mw, "VidPress", a.toolsErr.Error()+"\n\n압축을 풀 때 VidPress.exe와 ffmpeg 폴더를 같은 곳에 두었는지 확인해 주세요.", walk.MsgBoxIconWarning)
	} else {
		a.log("FFmpeg: " + a.tools.FFmpeg)
		go a.detectHW()
	}
	if len(a.startFiles) > 0 {
		a.addPaths(a.startFiles)
	}
	a.refreshIdleStatus()
}

func (a *App) detectHW() {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	hw := engine.DetectHW(ctx, a.tools.FFmpeg)
	a.mw.Synchronize(func() {
		a.hw, a.hwReady = hw, true
		if v := engine.HWVendorText(hw); v != "" {
			var names []string
			for n := range hw {
				names = append(names, n)
			}
			a.log("하드웨어 가속 사용 가능: " + v + " (" + strings.Join(names, ", ") + ")")
		} else {
			a.log("하드웨어 가속 장치를 찾지 못했습니다 (CPU로 변환합니다)")
		}
		a.updateHWCheckbox()
		a.updateEnabled()
	})
}
