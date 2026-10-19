package engine

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ErrCanceled is returned when the user stops a conversion.
var ErrCanceled = errors.New("사용자가 중지함")

// Progress is reported while a file is being converted.
type Progress struct {
	Fraction  float64 // 0..1 for this file across all passes; <0 when unknown
	PassLabel string
	Speed     float64 // processing speed relative to playback (2.0 = twice as fast)
	FPS       float64
	OutTime   float64       // seconds of media processed in the current pass
	Size      int64         // bytes written so far
	ETA       time.Duration // remaining time for this file; <0 when unknown
}

// Result describes a finished conversion.
type Result struct {
	Output   string
	Size     int64
	Elapsed  time.Duration
	Encoder  string
	FellBack bool // hardware encoding failed and the CPU encoder was used
}

// Converter runs conversions with a given set of tools.
type Converter struct {
	Tools      Tools
	HWEncoders map[string]bool
	TempDir    string // where 2-pass statistics go ("" = system temp)
	LogCommand func(args []string)
}

// Convert converts info.Path into finalPath according to s.
// Notes (human readable remarks about the plan) are sent to onNote.
func (c *Converter) Convert(ctx context.Context, info *MediaInfo, s Settings, finalPath string,
	onProgress func(Progress), onNote func(string)) (*Result, error) {

	start := time.Now()
	// FFmpeg runs with a different working directory, so every path it gets
	// must be absolute.
	if abs, err := filepath.Abs(finalPath); err == nil {
		finalPath = abs
	}
	if abs, err := filepath.Abs(info.Path); err == nil && abs != info.Path {
		cp := *info
		cp.Path = abs
		info = &cp
	}
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
		return nil, fmt.Errorf("저장 폴더를 만들 수 없습니다: %w", err)
	}
	part := PartPath(finalPath)
	workDir, err := os.MkdirTemp(c.TempDir, "vidpress-")
	if err != nil {
		return nil, fmt.Errorf("임시 폴더를 만들 수 없습니다: %w", err)
	}
	defer os.RemoveAll(workDir)

	in := PlanInput{
		Info:       info,
		Settings:   s,
		Output:     part,
		PassLog:    "vidpress-pass",
		NullPath:   NullDevice(),
		HWEncoders: c.HWEncoders,
	}
	plan, err := BuildPlan(in)
	if err != nil {
		return nil, err
	}
	if onNote != nil {
		for _, n := range plan.Notes {
			onNote(n)
		}
	}
	res := &Result{Encoder: plan.Encoder}

	err = c.runPlan(ctx, plan, info.Duration, workDir, start, onProgress)
	if err != nil && plan.Hardware && ctx.Err() == nil {
		os.Remove(part)
		if onNote != nil {
			onNote("하드웨어 가속(" + plan.Encoder + ") 인코딩에 실패해 일반 CPU 인코딩으로 다시 시도합니다")
		}
		in.ForceSW = true
		plan, err = BuildPlan(in)
		if err != nil {
			return nil, err
		}
		res.Encoder, res.FellBack = plan.Encoder, true
		err = c.runPlan(ctx, plan, info.Duration, workDir, start, onProgress)
	}
	if err != nil {
		os.Remove(part)
		if ctx.Err() != nil {
			return nil, ErrCanceled
		}
		return nil, err
	}

	st, err := os.Stat(part)
	if err != nil || st.Size() == 0 {
		os.Remove(part)
		return nil, errors.New("결과 파일이 만들어지지 않았습니다")
	}
	// Target size: if the encoder overshot noticeably, encode once more with
	// a corrected bitrate so the file really fits (e.g. upload limits).
	if s.Method == MethodTarget && s.TargetMB > 0 {
		limit := int64(s.TargetMB * 1024 * 1024)
		for attempt := 0; attempt < 2 && st.Size() > limit*1015/1000 && ctx.Err() == nil; attempt++ {
			scale := in.TargetScale
			if scale <= 0 {
				scale = 1
			}
			in.TargetScale = scale * float64(limit) / float64(st.Size()) * 0.97
			if onNote != nil {
				onNote(fmt.Sprintf("결과(%s)가 목표(%s)보다 커서 비트레이트를 낮춰 한 번 더 변환합니다", FormatSize(st.Size()), FormatSize(limit)))
			}
			plan, err = BuildPlan(in)
			if err != nil {
				os.Remove(part)
				return nil, err
			}
			if err = c.runPlan(ctx, plan, info.Duration, workDir, time.Now(), onProgress); err != nil {
				os.Remove(part)
				if ctx.Err() != nil {
					return nil, ErrCanceled
				}
				return nil, err
			}
			if st, err = os.Stat(part); err != nil {
				return nil, errors.New("결과 파일이 만들어지지 않았습니다")
			}
		}
	}
	dest := finalPath
	if _, err := os.Stat(dest); err == nil {
		// Someone created the file meanwhile; pick a fresh name.
		dest = OutputPath(dest, filepath.Dir(dest), "", filepath.Ext(dest), nil)
	}
	if err := os.Rename(part, dest); err != nil {
		os.Remove(part)
		return nil, fmt.Errorf("결과 파일 이름을 바꿀 수 없습니다: %w", err)
	}
	res.Output = dest
	res.Size = st.Size()
	res.Elapsed = time.Since(start)
	return res, nil
}

func (c *Converter) runPlan(ctx context.Context, plan *Plan, duration float64, workDir string,
	start time.Time, onProgress func(Progress)) error {

	base := 0.0
	var etaSmooth float64 = -1
	for _, pass := range plan.Passes {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if c.LogCommand != nil {
			c.LogCommand(pass.Args)
		}
		passBase, weight, label := base, pass.Weight, pass.Label
		emit := func(pr Progress) {
			if onProgress == nil {
				return
			}
			pr.PassLabel = label
			if duration > 0 {
				f := pr.OutTime / duration
				if f < 0 {
					f = 0
				}
				if f > 1 {
					f = 1
				}
				pr.Fraction = passBase + weight*f
				el := time.Since(start).Seconds()
				pr.ETA = -1
				if pr.Fraction > 0.01 && el > 2 {
					eta := el * (1 - pr.Fraction) / pr.Fraction
					if etaSmooth < 0 {
						etaSmooth = eta
					} else {
						etaSmooth = etaSmooth*0.8 + eta*0.2
					}
					pr.ETA = time.Duration(etaSmooth * float64(time.Second))
				}
			} else {
				pr.Fraction = -1
				pr.ETA = -1
			}
			onProgress(pr)
		}
		if err := runFFmpeg(ctx, c.Tools.FFmpeg, pass.Args, workDir, emit); err != nil {
			return err
		}
		base += pass.Weight
	}
	return nil
}

// runFFmpeg runs one ffmpeg process with "-progress pipe:1" output parsing.
func runFFmpeg(ctx context.Context, ffmpeg string, args []string, dir string, emit func(Progress)) error {
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	cmd.Dir = dir
	setProcAttr(cmd, true)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("FFmpeg를 실행할 수 없습니다: %w", err)
	}
	attachToJob(cmd)

	var wg sync.WaitGroup
	tail := newTailBuffer(40)
	wg.Add(1)
	go func() {
		defer wg.Done()
		io.Copy(tail, stderr)
	}()

	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	var pr Progress
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok {
			continue
		}
		switch k {
		case "out_time_us":
			if t := parseFloat(v); t > 0 {
				pr.OutTime = t / 1e6
			}
		case "out_time_ms": // historically misnamed: also microseconds
			if pr.OutTime == 0 {
				if t := parseFloat(v); t > 0 {
					pr.OutTime = t / 1e6
				}
			}
		case "speed":
			pr.Speed = parseFloat(strings.TrimSuffix(strings.TrimSpace(v), "x"))
		case "fps":
			pr.FPS = parseFloat(v)
		case "total_size":
			if n := parseInt(v); n > 0 {
				pr.Size = n
			}
		case "progress":
			if emit != nil {
				emit(pr)
			}
		}
	}
	io.Copy(io.Discard, stdout)
	wg.Wait()
	err = cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		msg := tail.meaningful(3)
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(friendlyFFmpegError(msg))
	}
	return nil
}

// tailBuffer keeps the last N lines written to it.
type tailBuffer struct {
	mu      sync.Mutex
	max     int
	lines   []string
	partial strings.Builder
}

func newTailBuffer(n int) *tailBuffer { return &tailBuffer{max: n} }

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, b := range p {
		if b == '\n' || b == '\r' {
			if t.partial.Len() > 0 {
				t.lines = append(t.lines, t.partial.String())
				if len(t.lines) > t.max {
					t.lines = t.lines[len(t.lines)-t.max:]
				}
				t.partial.Reset()
			}
			continue
		}
		if t.partial.Len() < 4096 {
			t.partial.WriteByte(b)
		}
	}
	return len(p), nil
}

// meaningful returns up to n of the last lines, skipping generic noise.
func (t *tailBuffer) meaningful(n int) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	lines := append([]string(nil), t.lines...)
	if t.partial.Len() > 0 {
		lines = append(lines, t.partial.String())
	}
	var out []string
	for i := len(lines) - 1; i >= 0 && len(out) < n; i-- {
		l := strings.TrimSpace(lines[i])
		if l == "" || l == "Conversion failed!" || strings.HasPrefix(l, "Error closing file") {
			continue
		}
		out = append([]string{l}, out...)
	}
	return strings.Join(out, " / ")
}
