# Third-party notices

VidPress itself is released under the MIT License (see LICENSE).
It uses or distributes the following open-source software.

## FFmpeg 8.1 (folder "ffmpeg" in the installed program / release packages)

- https://ffmpeg.org/ — Windows build by BtbN: https://github.com/BtbN/FFmpeg-Builds
- License: GNU General Public License version 3 (ffmpeg\LICENSE.txt).
  This build includes x264, x265, SVT-AV1, libvpx, LAME, Opus, zimg and other
  GPL/LGPL libraries.
- Source code: https://git.ffmpeg.org/ffmpeg.git (branch release/8.1) and the
  build scripts at https://github.com/BtbN/FFmpeg-Builds.
- VidPress runs ffmpeg.exe / ffprobe.exe as separate programs. FFmpeg is not
  part of this repository; it is added when the release packages are built.

## Go libraries compiled into VidPress.exe

| Library | License | Source |
|---|---|---|
| walk (Tailscale fork) | BSD 3-Clause | https://github.com/tailscale/walk (originally https://github.com/lxn/walk) |
| win (Tailscale fork) | BSD 3-Clause | https://github.com/tailscale/win |
| wingoes | BSD 3-Clause | https://github.com/dblohm7/wingoes |
| golang.org/x/sys, golang.org/x/exp | BSD 3-Clause | https://go.googlesource.com/sys , https://go.googlesource.com/exp |
| govaluate | MIT | https://github.com/Knetic/govaluate |

## Build tools (not part of VidPress.exe)

| Tool | License | Source |
|---|---|---|
| winres (tools/mkres) | 0BSD | https://github.com/tc-hib/winres |
| Inno Setup (installer) | Inno Setup License | https://jrsoftware.org/isinfo.php |
