@echo off
rem FFmpeg(BtbN의 GPL shared 빌드)를 내려받아 지정한 폴더에 넣습니다.
rem 사용법: scripts\get-ffmpeg.bat [대상 폴더]    (기본: dist\VidPress\ffmpeg)
rem Windows 10/11에 기본으로 들어 있는 curl.exe 와 tar.exe 를 사용합니다.
chcp 65001 > nul
setlocal
set "DEST=%~1"
if "%DEST%"=="" set "DEST=%~dp0..\dist\VidPress\ffmpeg"
rem 상대 경로를 절대 경로로 (아래에서 작업 폴더로 이동하기 때문)
for %%I in ("%DEST%") do set "DEST=%%~fI"
set "BASE=https://github.com/BtbN/FFmpeg-Builds/releases/download/latest"
set "WORK=%TEMP%\vidpress-ffmpeg-%RANDOM%%RANDOM%"
mkdir "%WORK%"
if errorlevel 1 goto :fail

echo       FFmpeg 내려받는 중... 약 90MB
curl.exe -fL --retry 3 -o "%WORK%\ffmpeg.zip" "%BASE%/ffmpeg-n8.1-latest-win64-gpl-shared-8.1.zip"
if errorlevel 1 curl.exe -fL --retry 3 -o "%WORK%\ffmpeg.zip" "%BASE%/ffmpeg-master-latest-win64-gpl-shared.zip"
if errorlevel 1 goto :fail

echo       압축 푸는 중...
tar.exe -xf "%WORK%\ffmpeg.zip" -C "%WORK%"
if errorlevel 1 goto :fail
if not exist "%DEST%" mkdir "%DEST%"
pushd "%WORK%"
for /d %%D in (ffmpeg-*) do (
  copy /y "%%~fD\bin\ffmpeg.exe" "%DEST%\" > nul
  copy /y "%%~fD\bin\ffprobe.exe" "%DEST%\" > nul
  copy /y "%%~fD\bin\*.dll" "%DEST%\" > nul
  copy /y "%%~fD\LICENSE.txt" "%DEST%\" > nul
)
popd
rmdir /s /q "%WORK%"
if not exist "%DEST%\ffmpeg.exe" goto :fail
echo       FFmpeg 준비 완료
exit /b 0

:fail
if exist "%WORK%" rmdir /s /q "%WORK%"
echo [오류] FFmpeg를 내려받지 못했습니다. 인터넷 연결을 확인하거나,
echo        https://github.com/BtbN/FFmpeg-Builds/releases 에서 win64-gpl-shared zip을 받아
echo        bin 폴더의 exe, dll 파일을 "%DEST%" 에 직접 넣어 주세요.
exit /b 1
