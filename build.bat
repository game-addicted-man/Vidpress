@echo off
rem ============================================================
rem  VidPress 빌드 (Windows)
rem    1) dist\VidPress\VidPress.exe 빌드
rem    2) FFmpeg가 없으면 내려받아 dist\VidPress\ffmpeg 에 넣기
rem    3) Inno Setup 6이 있으면 설치 파일 만들기
rem       -> dist\VidPress-Setup-<버전>.exe
rem  필요: Go 1.24 이상 (https://go.dev/dl/)
rem  선택: Inno Setup 6.7 이상 (https://jrsoftware.org/isdl.php)
rem  사용법: build.bat [버전]     예) build.bat 1.2.0
rem ============================================================
chcp 65001 > nul
setlocal
cd /d "%~dp0"

set "VERSION=1.0.0"
if not "%~1"=="" set "VERSION=%~1"
set "OUT=dist\VidPress"

where go > nul 2> nul
if errorlevel 1 (
  echo [오류] Go가 설치되어 있지 않습니다. https://go.dev/dl/ 에서 설치한 뒤 다시 실행하세요.
  goto :fail
)

echo [1/3] VidPress.exe 빌드 중... 버전 %VERSION%
if not exist "%OUT%" mkdir "%OUT%"
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=amd64
go build -trimpath -ldflags "-H windowsgui -s -w -X main.appVersion=%VERSION%" -o "%OUT%\VidPress.exe" .
if errorlevel 1 goto :fail
copy /y docs\*.txt "%OUT%\" > nul
copy /y LICENSE "%OUT%\LICENSE.txt" > nul
copy /y THIRD_PARTY_NOTICES.md "%OUT%\THIRD_PARTY_NOTICES.txt" > nul

echo [2/3] FFmpeg 확인 중...
if exist "%OUT%\ffmpeg\ffmpeg.exe" goto :have_ffmpeg
call scripts\get-ffmpeg.bat "%OUT%\ffmpeg"
if errorlevel 1 goto :fail
:have_ffmpeg
echo       FFmpeg: %OUT%\ffmpeg

echo [3/3] 설치 파일 만들기...
set "ISCC="
if exist "%ProgramFiles(x86)%\Inno Setup 6\ISCC.exe" set "ISCC=%ProgramFiles(x86)%\Inno Setup 6\ISCC.exe"
if exist "%LOCALAPPDATA%\Programs\Inno Setup 6\ISCC.exe" set "ISCC=%LOCALAPPDATA%\Programs\Inno Setup 6\ISCC.exe"
if not defined ISCC goto :no_inno
"%ISCC%" /Q /DAppVersion=%VERSION% installer\VidPress.iss
if errorlevel 1 goto :fail
echo       완료: dist\VidPress-Setup-%VERSION%.exe
goto :done

:no_inno
echo       Inno Setup 6이 없어서 설치 파일 만들기는 건너뜁니다.
echo       설치 파일이 필요하면 https://jrsoftware.org/isdl.php 에서 Inno Setup을 설치하세요.

:done
echo.
echo 빌드 완료. 실행: %OUT%\VidPress.exe
if not defined CI pause
exit /b 0

:fail
echo.
echo [오류] 빌드에 실패했습니다. 위 메시지를 확인하세요.
if not defined CI pause
exit /b 1
