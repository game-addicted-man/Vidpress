//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"

	"vidpress/engine"
)

func (a *App) onStart() {
	if a.running {
		return
	}
	if a.toolsErr != nil {
		walk.MsgBox(a.mw, "VidPress", a.toolsErr.Error(), walk.MsgBoxIconWarning)
		return
	}
	a.settings = a.readControls()
	s := a.settings

	st := a.model.Stats(0)
	if st.Total == 0 {
		a.setStatus("먼저 변환할 파일을 추가해 주세요")
		walk.MsgBox(a.mw, "VidPress", "먼저 변환할 동영상 파일을 추가해 주세요.\n\n[＋ 파일 추가]를 누르거나 파일을 창에 끌어다 놓으면 됩니다.", walk.MsgBoxIconInformation)
		return
	}
	if st.Probing > 0 {
		a.autoStart = true
		a.setStatus("파일 정보를 읽은 뒤 바로 시작합니다…")
		return
	}
	// Failed/stopped files are retried automatically.
	retry := 0
	for _, it := range a.model.Snapshot() {
		a.model.Update(it, func(it *Item) {
			if it.State == StFailed || it.State == StCanceled {
				it.State, it.Err, it.Progress = StReady, nil, 0
				retry++
			}
		})
	}
	if retry > 0 {
		a.model.PublishRowsReset()
	}
	st = a.model.Stats(0)
	if st.Ready == 0 {
		if st.Done > 0 {
			if walk.MsgBox(a.mw, "VidPress", "목록의 파일이 모두 이미 변환되었습니다.\n현재 설정으로 다시 변환할까요?", walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != win.IDYES {
				return
			}
			for _, it := range a.model.Snapshot() {
				a.model.Update(it, func(it *Item) {
					if it.State == StDone {
						it.State, it.Result, it.Progress = StReady, nil, 0
					}
				})
			}
			a.model.PublishRowsReset()
		} else {
			a.setStatus("변환할 수 있는 파일이 없습니다")
			return
		}
	}
	if s.Method == engine.MethodTarget && s.TargetMB < 1 {
		walk.MsgBox(a.mw, "VidPress", "목표 용량을 1MB 이상으로 정해 주세요.", walk.MsgBoxIconWarning)
		return
	}
	if s.OutputDir != "" {
		if err := os.MkdirAll(s.OutputDir, 0o755); err != nil {
			walk.MsgBox(a.mw, "VidPress", "저장 폴더를 사용할 수 없습니다:\n"+err.Error(), walk.MsgBoxIconWarning)
			return
		}
	}
	a.saveConfig()

	a.batch++
	batch := a.batch
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.running = true
	a.updateEnabled()
	a.pbTotal.SetValue(0)
	if pi := a.mw.ProgressIndicator(); pi != nil {
		pi.SetTotal(1000)
		pi.SetState(walk.PINormal)
		pi.SetCompleted(0)
	}
	keepAwake(true)

	f := s.Format()
	a.log(fmt.Sprintf("변환 시작 — %s · %s", engine.MethodLabels[s.Method], strings.TrimSpace(strings.SplitN(f.Label, "—", 2)[0])))
	a.setStatus("변환을 시작합니다…")
	a.lblSummary.SetText("변환 중에도 다른 작업을 하셔도 됩니다. 컴퓨터가 절전 모드로 들어가지 않게 해 둡니다.")

	hw := a.hw
	if !a.hwReady {
		hw = nil // detection still running: use the CPU encoders
	}
	conv := &engine.Converter{
		Tools:      a.tools,
		HWEncoders: hw,
		LogCommand: func(args []string) { a.fileLog("ffmpeg " + quoteArgs(args)) },
	}
	go a.runBatch(ctx, batch, s, conv)
}

func quoteArgs(args []string) string {
	q := make([]string, len(args))
	for i, s := range args {
		if strings.ContainsAny(s, " \t'\"") || s == "" {
			q[i] = `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
		} else {
			q[i] = s
		}
	}
	return strings.Join(q, " ")
}

// runBatch converts items one after another (runs on its own goroutine).
func (a *App) runBatch(ctx context.Context, batch int, s engine.Settings, conv *engine.Converter) {
	reserved := map[string]bool{}
	f := s.Format()
	var firstOutput string
	start := time.Now()

	for ctx.Err() == nil {
		it := a.model.ClaimNext(batch)
		if it == nil {
			break
		}
		final := engine.OutputPath(it.Path, s.OutputDir, s.Suffix, f.Ext, reserved)
		reserved[strings.ToLower(final)] = true
		a.mw.Synchronize(func() {
			a.model.NotifyChanged(it)
			a.log(fmt.Sprintf("▶ %s  →  %s", it.Name, filepath.Base(final)))
			a.updateBatchProgress(batch, it, engine.Progress{Fraction: 0, ETA: -1})
		})

		lastUI := time.Time{}
		res, err := conv.Convert(ctx, it.Info, s, final,
			func(p engine.Progress) {
				a.model.Update(it, func(it *Item) {
					it.Progress = p.Fraction
					it.PassLabel = p.PassLabel
				})
				if time.Since(lastUI) < 250*time.Millisecond {
					return
				}
				lastUI = time.Now()
				a.mw.Synchronize(func() {
					a.model.NotifyChanged(it)
					a.updateBatchProgress(batch, it, p)
				})
			},
			func(note string) { a.logAsync("   · " + note) })

		a.model.Update(it, func(it *Item) {
			switch {
			case err == nil:
				it.State, it.Result, it.Progress = StDone, res, 1
			case errors.Is(err, engine.ErrCanceled):
				it.State, it.Progress = StCanceled, 0
			default:
				it.State, it.Err = StFailed, err
			}
		})
		a.mw.Synchronize(func() {
			a.model.NotifyChanged(it)
			switch {
			case err == nil:
				extra := ""
				if res.FellBack {
					extra = " (CPU로 다시 변환함)"
				}
				a.log(fmt.Sprintf("○ 완료: %s — %s, %s 걸림%s", filepath.Base(res.Output), engine.SavingText(it.Info.Size, res.Size), fmtElapsed(res.Elapsed), extra))
				if firstOutput == "" {
					firstOutput = res.Output
				}
			case errors.Is(err, engine.ErrCanceled):
				a.log("■ 중지됨: " + it.Name)
			default:
				a.log(fmt.Sprintf("× 실패: %s — %v", it.Name, err))
			}
		})
	}

	a.mw.Synchronize(func() { a.finishBatch(batch, firstOutput, time.Since(start), ctx.Err() != nil) })
}

func fmtElapsed(d time.Duration) string {
	sec := int(d.Round(time.Second).Seconds())
	switch {
	case sec >= 3600:
		return fmt.Sprintf("%d시간 %d분", sec/3600, (sec/60)%60)
	case sec >= 60:
		return fmt.Sprintf("%d분 %d초", sec/60, sec%60)
	default:
		return fmt.Sprintf("%d초", sec)
	}
}

// updateBatchProgress refreshes the overall progress bar, taskbar progress
// and the status line (UI thread).
func (a *App) updateBatchProgress(batch int, it *Item, p engine.Progress) {
	st := a.model.Stats(batch)
	total := st.BatchTotal
	if total == 0 {
		total = 1
	}
	frac := p.Fraction
	if frac < 0 {
		frac = 0
	}
	overall := (float64(st.BatchFinished) + frac) / float64(total)
	if overall > 1 {
		overall = 1
	}
	v := int(overall * 1000)
	a.pbTotal.SetValue(v)
	if pi := a.mw.ProgressIndicator(); pi != nil {
		pi.SetCompleted(uint32(v))
	}

	idx := st.BatchFinished + 1
	if idx > total {
		idx = total
	}
	msg := fmt.Sprintf("%d / %d  ·  %s", idx, total, it.Name)
	a.setStatus(msg)

	var parts []string
	if p.PassLabel != "" {
		if p.Fraction >= 0 {
			parts = append(parts, fmt.Sprintf("%s %d%%", p.PassLabel, int(p.Fraction*100)))
		} else {
			parts = append(parts, p.PassLabel)
		}
	}
	if p.Speed > 0 {
		parts = append(parts, fmt.Sprintf("속도 %.1f배", p.Speed))
	}
	if p.Size >= 1<<20 {
		parts = append(parts, "현재 "+engine.FormatSize(p.Size))
	}
	if p.ETA > 0 {
		parts = append(parts, "남은 시간 "+engine.FormatETA(p.ETA))
	}
	if remaining := total - idx; remaining > 0 {
		parts = append(parts, fmt.Sprintf("다음 대기 %d개", remaining))
	}
	a.lblSummary.SetText(strings.Join(parts, "  ·  "))
	a.mw.SetTitle(fmt.Sprintf("%d%% — VidPress", int(overall*100)))
}

func (a *App) finishBatch(batch int, firstOutput string, elapsed time.Duration, canceled bool) {
	a.running = false
	a.cancel = nil
	keepAwake(false)
	if a.closeAfterStop {
		// The window is about to close: don't touch the controls any more.
		a.fileLog("변환을 중지하고 프로그램을 종료합니다")
		a.closeLater()
		return
	}
	a.updateEnabled()
	a.mw.SetTitle("VidPress — 동영상 용량 줄이기 · 형식 바꾸기")

	st := a.model.Stats(batch)
	done, failed := 0, 0
	for _, it := range a.model.Snapshot() {
		if it.Batch != batch {
			continue
		}
		switch it.State {
		case StDone:
			done++
		case StFailed:
			failed++
		}
	}
	pi := a.mw.ProgressIndicator()
	var summary string
	switch {
	case canceled:
		summary = fmt.Sprintf("중지했습니다 — 완료 %d개", done)
		if pi != nil {
			pi.SetState(walk.PIPaused)
		}
	case failed > 0:
		summary = fmt.Sprintf("끝났습니다 — 완료 %d개, 실패 %d개", done, failed)
		if pi != nil {
			pi.SetState(walk.PIError)
		}
	default:
		summary = fmt.Sprintf("모두 끝났습니다 — %d개 완료", done)
		a.pbTotal.SetValue(1000)
		if pi != nil {
			pi.SetState(walk.PINoProgress)
		}
	}
	a.setStatus(summary)
	detail := fmt.Sprintf("걸린 시간 %s", fmtElapsed(elapsed))
	if st.InSize > 0 {
		detail = engine.SavingText(st.InSize, st.OutSize) + "  ·  " + detail
	}
	if failed > 0 {
		detail += "  ·  실패한 파일은 더블클릭하면 이유를 볼 수 있습니다"
	}
	a.lblSummary.SetText(detail)
	a.log(summary + " (" + detail + ")")

	if !canceled {
		flashIfInactive(uintptr(a.mw.Handle()))
		beep()
		if a.settings.OpenWhenEnd && firstOutput != "" && !a.exitWhenDone {
			showInExplorer(firstOutput)
		}
	}
	if a.exitWhenDone {
		a.closeLater()
		return
	}
	a.btnStart.SetFocus()
}

// closeLater closes the window from the message loop instead of from inside
// the current callback, so walk finishes any pending layout first (closing
// synchronously right after updating labels can crash walk).
func (a *App) closeLater() {
	time.AfterFunc(400*time.Millisecond, func() {
		a.mw.Synchronize(func() { win.PostMessage(a.mw.Handle(), win.WM_CLOSE, 0, 0) })
	})
}

func (a *App) onStop() {
	if !a.running || a.cancel == nil {
		return
	}
	if walk.MsgBox(a.mw, "VidPress", "지금 변환 중인 파일을 중지할까요?\n(완료된 파일은 그대로 남고, 변환 중이던 파일은 지워집니다)", walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != win.IDYES {
		return
	}
	// The batch may have finished while the question was on screen.
	if !a.running || a.cancel == nil {
		return
	}
	a.setStatus("중지하는 중…")
	a.cancel()
}

func (a *App) onClosing(canceled *bool, reason walk.CloseReason) {
	if !a.running {
		a.settings = a.readControls()
		return
	}
	if a.closeAfterStop || a.exitWhenDone {
		*canceled = true
		if a.cancel != nil {
			a.cancel()
		}
		a.closeAfterStop = true
		return
	}
	if walk.MsgBox(a.mw, "VidPress", "아직 변환 중입니다. 중지하고 종료할까요?", walk.MsgBoxYesNo|walk.MsgBoxIconWarning) != win.IDYES {
		*canceled = true
		return
	}
	if !a.running || a.cancel == nil {
		// Finished while the question was shown: just close.
		a.settings = a.readControls()
		return
	}
	*canceled = true
	a.closeAfterStop = true
	a.setStatus("중지하고 종료하는 중…")
	a.cancel()
}
