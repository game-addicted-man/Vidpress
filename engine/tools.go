package engine

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Tools holds the paths of the FFmpeg executables.
type Tools struct {
	FFmpeg  string
	FFprobe string
}

func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// FindTools looks for ffmpeg/ffprobe next to the application first (the
// bundled copy in the "ffmpeg" folder), then on PATH.
func FindTools(appDir string) (Tools, error) {
	var dirs []string
	if appDir != "" {
		dirs = append(dirs,
			filepath.Join(appDir, "ffmpeg"),
			filepath.Join(appDir, "ffmpeg", "bin"),
			appDir,
			filepath.Join(appDir, "bin"),
		)
	}
	for _, d := range dirs {
		ff := filepath.Join(d, exeName("ffmpeg"))
		fp := filepath.Join(d, exeName("ffprobe"))
		if isFile(ff) && isFile(fp) {
			return Tools{FFmpeg: ff, FFprobe: fp}, nil
		}
	}
	ff, err1 := exec.LookPath(exeName("ffmpeg"))
	fp, err2 := exec.LookPath(exeName("ffprobe"))
	if err1 == nil && err2 == nil {
		return Tools{FFmpeg: ff, FFprobe: fp}, nil
	}
	return Tools{}, errors.New("FFmpeg를 찾을 수 없습니다. 프로그램 폴더 안에 'ffmpeg' 폴더(ffmpeg.exe, ffprobe.exe 포함)가 있어야 합니다")
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
