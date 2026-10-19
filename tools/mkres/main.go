// mkres builds the Windows resource object (icon, manifest, version info)
// that gets linked into VidPress.exe.
//
// Usage (from the project root):
//
//	cd tools/mkres && go run . -root ../..
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/tc-hib/winres"
	"github.com/tc-hib/winres/version"
)

func main() {
	root := flag.String("root", "../..", "project root directory")
	ver := flag.String("version", "1.0.0.0", "file/product version (a.b.c.d)")
	flag.Parse()

	icoFile, err := os.Open(filepath.Join(*root, "assets", "app.ico"))
	if err != nil {
		log.Fatal(err)
	}
	defer icoFile.Close()

	icon, err := winres.LoadICO(icoFile)
	if err != nil {
		log.Fatal(err)
	}

	var rs winres.ResourceSet
	if err := rs.SetIcon(winres.Name("APP"), icon); err != nil {
		log.Fatal(err)
	}

	rs.SetManifest(winres.AppManifest{
		Identity:            winres.AssemblyIdentity{Name: "VidPress.App", Version: [4]uint16{1, 0, 0, 0}},
		Description:         "VidPress - video compressor and converter",
		Compatibility:       winres.Win7AndAbove,
		ExecutionLevel:      winres.AsInvoker,
		DPIAwareness:        winres.DPIPerMonitorV2,
		LongPathAware:       true,
		UseCommonControlsV6: true,
	})

	var vi version.Info
	vi.SetFileVersion(*ver)
	vi.SetProductVersion(*ver)
	for _, kv := range [][2]string{
		{version.ProductName, "VidPress"},
		{version.FileDescription, "VidPress 동영상 압축·변환기"},
		{version.CompanyName, "VidPress"},
		{version.LegalCopyright, "© 2026 VidPress"},
		{version.OriginalFilename, "VidPress.exe"},
		{version.InternalName, "VidPress"},
	} {
		if err := vi.Set(0x0412, kv[0], kv[1]); err != nil { // 0x0412 = Korean
			log.Fatal(err)
		}
	}
	rs.SetVersionInfo(vi)

	out := filepath.Join(*root, "rsrc_windows_amd64.syso")
	f, err := os.Create(out)
	if err != nil {
		log.Fatal(err)
	}
	if err := rs.WriteObject(f, winres.ArchAMD64); err != nil {
		f.Close()
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s", out)
}
