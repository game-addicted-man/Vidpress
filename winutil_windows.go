//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"
	"golang.org/x/sys/windows"
)

var (
	modOle32    = windows.NewLazySystemDLL("ole32.dll")
	modShell32  = windows.NewLazySystemDLL("shell32.dll")
	modUser32   = windows.NewLazySystemDLL("user32.dll")
	modKernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procCoCreateInstance            = modOle32.NewProc("CoCreateInstance")
	procCoTaskMemFree               = modOle32.NewProc("CoTaskMemFree")
	procSHCreateItemFromParsingName = modShell32.NewProc("SHCreateItemFromParsingName")
	procILCreateFromPathW           = modShell32.NewProc("ILCreateFromPathW")
	procILFree                      = modShell32.NewProc("ILFree")
	procSHOpenFolderAndSelectItems  = modShell32.NewProc("SHOpenFolderAndSelectItems")
	procFlashWindowEx               = modUser32.NewProc("FlashWindowEx")
	procGetForegroundWindow         = modUser32.NewProc("GetForegroundWindow")
	procSetThreadExecutionState     = modKernel32.NewProc("SetThreadExecutionState")
	procMessageBeep                 = modUser32.NewProc("MessageBeep")
)

var (
	clsidFileOpenDialog = windows.GUID{Data1: 0xDC1C5A9C, Data2: 0xE88A, Data3: 0x4DDE, Data4: [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}}
	iidIFileOpenDialog  = windows.GUID{Data1: 0xD57C7288, Data2: 0xD4AD, Data3: 0x4768, Data4: [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60}}
	iidIShellItem       = windows.GUID{Data1: 0x43826D1E, Data2: 0xE718, Data3: 0x42EE, Data4: [8]byte{0xBC, 0x55, 0xA1, 0xE2, 0x61, 0xC3, 0x7B, 0xFE}}
)

// comObject is the memory layout of every COM interface pointer: a pointer
// to the vtable.
type comObject struct {
	vtbl *[32]uintptr
}

func (o *comObject) call(method int, args ...uintptr) uintptr {
	all := append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)
	r, _, _ := syscall.SyscallN(o.vtbl[method], all...)
	return r
}

func (o *comObject) release() { o.call(2) }

const (
	fosNoChangeDir     = 0x00000008
	fosPickFolders     = 0x00000020
	fosForceFileSystem = 0x00000040
	fosPathMustExist   = 0x00000800
	sigdnFileSysPath   = 0x80058000
	hrCanceled         = 0x800704C7

	// IFileOpenDialog vtable indexes
	vtShow        = 3
	vtSetOptions  = 9
	vtGetOptions  = 10
	vtSetFolder   = 12
	vtSetTitle    = 17
	vtSetOkButton = 18
	vtGetResult   = 20
	// IShellItem
	vtGetDisplayName = 5
)

// pickFolder shows the modern (Explorer style) folder picker. It falls back
// to the classic tree dialog if the COM dialog is unavailable.
func pickFolder(owner walk.Form, title, initial string) (string, bool) {
	var dlg *comObject
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidFileOpenDialog)), 0, 1, /* CLSCTX_INPROC_SERVER */
		uintptr(unsafe.Pointer(&iidIFileOpenDialog)), uintptr(unsafe.Pointer(&dlg)))
	if int32(hr) < 0 || dlg == nil {
		return pickFolderClassic(owner, title)
	}
	defer dlg.release()

	var opts uint32
	dlg.call(vtGetOptions, uintptr(unsafe.Pointer(&opts)))
	dlg.call(vtSetOptions, uintptr(opts|fosPickFolders|fosForceFileSystem|fosPathMustExist|fosNoChangeDir))
	if t, err := windows.UTF16PtrFromString(title); err == nil {
		dlg.call(vtSetTitle, uintptr(unsafe.Pointer(t)))
	}
	if ok, err := windows.UTF16PtrFromString("폴더 선택"); err == nil {
		dlg.call(vtSetOkButton, uintptr(unsafe.Pointer(ok)))
	}
	if initial != "" {
		if p, err := windows.UTF16PtrFromString(initial); err == nil {
			var item *comObject
			r, _, _ := procSHCreateItemFromParsingName.Call(uintptr(unsafe.Pointer(p)), 0,
				uintptr(unsafe.Pointer(&iidIShellItem)), uintptr(unsafe.Pointer(&item)))
			if int32(r) >= 0 && item != nil {
				dlg.call(vtSetFolder, uintptr(unsafe.Pointer(item)))
				item.release()
			}
		}
	}
	var hwnd uintptr
	if owner != nil {
		hwnd = uintptr(owner.Handle())
	}
	hr = dlg.call(vtShow, hwnd)
	if uint32(hr) == hrCanceled || int32(hr) < 0 {
		return "", false
	}
	var res *comObject
	if r := dlg.call(vtGetResult, uintptr(unsafe.Pointer(&res))); int32(r) < 0 || res == nil {
		return "", false
	}
	defer res.release()
	var psz *uint16
	if r := res.call(vtGetDisplayName, sigdnFileSysPath, uintptr(unsafe.Pointer(&psz))); int32(r) < 0 || psz == nil {
		return "", false
	}
	path := windows.UTF16PtrToString(psz)
	procCoTaskMemFree.Call(uintptr(unsafe.Pointer(psz)))
	return path, path != ""
}

func pickFolderClassic(owner walk.Form, title string) (string, bool) {
	dlg := walk.FileDialog{Title: title}
	ok, err := dlg.ShowBrowseFolder(owner)
	if err != nil || !ok {
		return "", false
	}
	return dlg.FilePath, true
}

// showInExplorer opens Explorer with the file selected (or the folder).
func showInExplorer(path string) {
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		openFolder(path)
		return
	}
	p, err := windows.UTF16PtrFromString(path)
	if err == nil {
		pidl, _, _ := procILCreateFromPathW.Call(uintptr(unsafe.Pointer(p)))
		if pidl != 0 {
			r, _, _ := procSHOpenFolderAndSelectItems.Call(pidl, 0, 0, 0)
			procILFree.Call(pidl)
			if int32(r) >= 0 {
				return
			}
		}
	}
	openFolder(filepath.Dir(path))
}

func openFolder(dir string) {
	cmd := exec.Command("explorer.exe", dir)
	cmd.Start()
}

type flashWInfo struct {
	cbSize    uint32
	hwnd      uintptr
	dwFlags   uint32
	uCount    uint32
	dwTimeout uint32
}

// flashIfInactive flashes the taskbar button when the window is not in front.
func flashIfInactive(hwnd uintptr) {
	fg, _, _ := procGetForegroundWindow.Call()
	if fg == hwnd {
		return
	}
	fi := flashWInfo{hwnd: hwnd, dwFlags: 0x2 | 0xC /* FLASHW_TRAY | FLASHW_TIMERNOFG */}
	fi.cbSize = uint32(unsafe.Sizeof(fi))
	procFlashWindowEx.Call(uintptr(unsafe.Pointer(&fi)))
}

// keepAwake prevents the PC from going to sleep while converting.
func keepAwake(on bool) {
	const esContinuous, esSystemRequired = 0x80000000, 0x00000001
	flags := uintptr(esContinuous)
	if on {
		flags |= esSystemRequired
	}
	procSetThreadExecutionState.Call(flags)
}

func beep() { procMessageBeep.Call(0x40 /* MB_ICONINFORMATION */) }

// fitAndCenter shrinks the window to the monitor's work area if needed and
// centres it (so it also fits on 1366×768 laptops).
func fitAndCenter(hwnd win.HWND) string {
	mon := win.MonitorFromWindow(hwnd, win.MONITOR_DEFAULTTOPRIMARY)
	mi := win.MONITORINFO{}
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	if !win.GetMonitorInfo(mon, &mi) {
		return "GetMonitorInfo failed"
	}
	var r win.RECT
	if !win.GetWindowRect(hwnd, &r) {
		return "GetWindowRect failed"
	}
	work := mi.RcWork
	ww, wh := work.Right-work.Left, work.Bottom-work.Top
	w, h := r.Right-r.Left, r.Bottom-r.Top
	if w > ww {
		w = ww
	}
	if h > wh {
		h = wh
	}
	x := work.Left + (ww-w)/2
	y := work.Top + (wh-h)/2
	win.SetWindowPos(hwnd, 0, x, y, w, h, win.SWP_NOZORDER|win.SWP_NOACTIVATE)
	return fmt.Sprintf("monitor=%v work=%v window=%v -> %d,%d %dx%d", mi.RcMonitor, work, r, x, y, w, h)
}

// appMutexName must match AppMutex in installer/VidPress.iss: it lets the
// installer and uninstaller detect a running VidPress and ask to close it.
const appMutexName = "VidPress.App.Mutex"

var appMutex windows.Handle

func createAppMutex() {
	if name, err := windows.UTF16PtrFromString(appMutexName); err == nil {
		appMutex, _ = windows.CreateMutex(nil, false, name)
	}
}
