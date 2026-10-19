; VidPress 설치 프로그램 스크립트 (Inno Setup 6.7 이상)
;
; 만들기: 저장소 폴더에서 build.bat 을 실행하면 dist\VidPress 를 만든 뒤
; 이 스크립트로 dist\VidPress-Setup-<버전>.exe 를 만듭니다.
; 직접 실행하려면:  "C:\Program Files (x86)\Inno Setup 6\ISCC.exe" installer\VidPress.iss
; 버전 지정:        ISCC.exe /DAppVersion=1.2.3 installer\VidPress.iss
;
; 기본은 관리자 권한 없이 현재 사용자용으로 설치합니다
; (%LOCALAPPDATA%\Programs\VidPress). 모든 사용자용으로 설치하려면 /ALLUSERS 를 붙여 실행하세요.

#ifndef AppVersion
  #define AppVersion "1.0.0"
#endif
#define AppName "VidPress"
#define AppExeName "VidPress.exe"
#define BuildDir "..\dist\VidPress"

[Setup]
; AppId는 업그레이드/제거에 쓰이므로 절대 바꾸지 마세요.
AppId={{ADC9883A-C2BB-4FE8-AC55-9DA539D11418}
AppName={#AppName}
AppVersion={#AppVersion}
AppVerName={#AppName} {#AppVersion}
AppPublisher=VidPress
VersionInfoVersion={#AppVersion}
VersionInfoProductName={#AppName}
VersionInfoDescription={#AppName} Setup
DefaultDirName={autopf}\{#AppName}
DefaultGroupName={#AppName}
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=commandline
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0
AppMutex=VidPress.App.Mutex
CloseApplications=yes
RestartApplications=no
OutputDir=..\dist
OutputBaseFilename=VidPress-Setup-{#AppVersion}
SetupIconFile=..\assets\app.ico
UninstallDisplayIcon={app}\{#AppExeName}
UninstallDisplayName={#AppName}
Compression=lzma2/ultra64
SolidCompression=yes
LZMAUseSeparateProcess=yes
WizardStyle=modern dynamic
WizardImageFile=wizard-large-100.png,wizard-large-150.png,wizard-large-200.png
WizardSmallImageFile=wizard-small-100.png,wizard-small-150.png,wizard-small-200.png
ShowLanguageDialog=auto

[Languages]
Name: "korean"; MessagesFile: "compiler:Languages\Korean.isl"
Name: "english"; MessagesFile: "compiler:Default.isl"

[CustomMessages]
korean.AppComment=동영상 용량 줄이기 · 형식 바꾸기
english.AppComment=Compress and convert videos
korean.SendToTask=탐색기 '보내기' 메뉴에 추가 (동영상 오른쪽 클릭 → 보내기 → VidPress)
english.SendToTask=Add to the Explorer "Send to" menu

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"
Name: "sendto"; Description: "{cm:SendToTask}"; GroupDescription: "{cm:AdditionalIcons}"

[InstallDelete]
; 새 FFmpeg로 바뀔 때 이전 버전 DLL이 남지 않도록 먼저 지웁니다.
Type: filesandordirs; Name: "{app}\ffmpeg"

[Files]
Source: "{#BuildDir}\{#AppExeName}"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#BuildDir}\ffmpeg\*"; DestDir: "{app}\ffmpeg"; Excludes: "ffplay.exe"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "..\docs\사용설명서.txt"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\LICENSE"; DestDir: "{app}"; DestName: "LICENSE.txt"; Flags: ignoreversion
Source: "..\THIRD_PARTY_NOTICES.md"; DestDir: "{app}"; DestName: "THIRD_PARTY_NOTICES.txt"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\{#AppName}"; Filename: "{app}\{#AppExeName}"; Comment: "{cm:AppComment}"
Name: "{autodesktop}\{#AppName}"; Filename: "{app}\{#AppExeName}"; Comment: "{cm:AppComment}"; Tasks: desktopicon
Name: "{usersendto}\{#AppName}"; Filename: "{app}\{#AppExeName}"; Comment: "{cm:AppComment}"; Tasks: sendto

[Run]
Filename: "{app}\{#AppExeName}"; Description: "{cm:LaunchProgram,{#AppName}}"; Flags: nowait postinstall skipifsilent
