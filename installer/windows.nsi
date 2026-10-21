; Thin per-user wrapper. Go owns setup consent, masked input and transactions.
Unicode true
RequestExecutionLevel user
SetCompressor /SOLID zlib
CRCCheck force
Name "Xpotato Tunnel ${VERSION}"
OutFile "${OUTPUT}"
InstallDir "$LOCALAPPDATA\Programs\XpotatoTunnel\${VERSION}"
ShowInstDetails show
ShowUninstDetails show
Var StagingDir
Var PayloadFolder
!include "MUI2.nsh"
!include "x64.nsh"
!include "FileFunc.nsh"
!macro CheckCore failure
  ; Authenticate the selected executable against the build-time payload digest
  ; before invoking it, including when reopening an existing installation.
  ; Fixed code uses the OS local-app-data API; no user path enters PS source.
  StrCpy $0 1
  ClearErrors
  ExecWait `"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -NonInteractive -Command "try { if (([BitConverter]::ToString([Security.Cryptography.SHA256]::Create().ComputeHash([IO.File]::ReadAllBytes([IO.Path]::Combine([Environment]::GetFolderPath('LocalApplicationData'),'Programs\XpotatoTunnel\$PayloadFolder\xpotato-tunnel.exe')))) -replace '-','') -eq '${CORE_SHA}') { exit 0 }; exit 1 } catch { exit 1 }"` $0
  IfErrors ${failure}
  StrCmp $0 0 0 ${failure}
!macroend
!define MUI_ABORTWARNING
!define MUI_CUSTOMFUNCTION_ABORT CleanupStage
!define MUI_FINISHPAGE_RUN "$INSTDIR\xpotato-tunnel.exe"
!define MUI_FINISHPAGE_RUN_PARAMETERS "maintenance"
!define MUI_FINISHPAGE_RUN_TEXT "Open Setup / Start / Update / Repair"
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "${PAYLOAD}\licenses\LICENSE"
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

Function .onInit
  ; Reject /D overrides: file removal is restricted to the fixed per-user path.
  StrCpy $INSTDIR "$LOCALAPPDATA\Programs\XpotatoTunnel\${VERSION}"
  ${IfNot} ${RunningX64}
    MessageBox MB_OK|MB_ICONSTOP "Windows amd64 is required." /SD IDOK
    SetErrorLevel 1
    Abort
  ${EndIf}
FunctionEnd

Function CleanupStage
  StrCmp $StagingDir "" finished
  SetOutPath "$TEMP"
  ; Exact child created by GetTempFileName below, no recursive removal.
  !include "cleanup-stage.nsh"
  RMDir "$StagingDir"
finished:
FunctionEnd

Function .onInstFailed
  Call CleanupStage
FunctionEnd

Section "Application"
  SetShellVarContext current
  StrCmp $INSTDIR "$LOCALAPPDATA\Programs\XpotatoTunnel\${VERSION}" 0 invalid
  ; Same-version payloads are immutable. Reopening a complete installation
  ; offers maintenance; changed/partial files are never silently overwritten.
  IfFileExists "$INSTDIR\*.*" existing
  CreateDirectory "$LOCALAPPDATA\Programs\XpotatoTunnel"
  ClearErrors
  GetTempFileName $StagingDir "$LOCALAPPDATA\Programs\XpotatoTunnel"
  IfErrors invalid
  StrCmp $StagingDir "" invalid
  Delete "$StagingDir"
  ClearErrors
  CreateDirectory "$StagingDir"
  IfErrors invalid
  ; GetTempFileName supplies the OS-generated ASCII basename, never a user
  ; supplied path. This keeps the PowerShell code independent of user paths.
  ${GetFileName} "$StagingDir" $PayloadFolder
  !include "install-files.nsh"
  !insertmacro CheckCore invalid
  StrCpy $0 1
  ExecWait '"$StagingDir\xpotato-tunnel.exe" package-check' $0
  StrCmp $0 0 0 invalid
  SetOutPath "$TEMP"
  ClearErrors
  Rename "$StagingDir" "$INSTDIR"
  IfErrors invalid
  StrCpy $StagingDir ""
  Goto register
existing:
  IfFileExists "$INSTDIR\xpotato-tunnel.exe" check invalid
check:
  StrCpy $PayloadFolder "${VERSION}"
  !insertmacro CheckCore invalid
  StrCpy $0 1
  ExecWait '"$INSTDIR\xpotato-tunnel.exe" package-check' $0
  StrCmp $0 0 register invalid
register:
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  CreateDirectory "$SMPROGRAMS\Xpotato Tunnel"
  CreateShortcut "$SMPROGRAMS\Xpotato Tunnel\Xpotato Tunnel.lnk" "$INSTDIR\xpotato-tunnel.exe" "maintenance"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\XpotatoTunnel" "DisplayName" "Xpotato Tunnel ${VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\XpotatoTunnel" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\XpotatoTunnel" "Publisher" "Xpotato.net / Xpotato1024"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\XpotatoTunnel" "UninstallString" '$\"$INSTDIR\Uninstall.exe$\"'
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\XpotatoTunnel" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\XpotatoTunnel" "NoRepair" 1
  Goto done
invalid:
  Call CleanupStage
  MessageBox MB_OK|MB_ICONSTOP "Installation incomplete or existing files changed. No runtime scan, process termination or protection bypass was attempted. Preserve this folder and report the error." /SD IDOK
  SetErrorLevel 1
  Abort
done:
SectionEnd

Function un.onInit
  StrCmp $INSTDIR "$LOCALAPPDATA\Programs\XpotatoTunnel\${VERSION}" pathValid
  MessageBox MB_OK|MB_ICONSTOP "Unexpected uninstall path. Nothing removed." /SD IDOK
  SetErrorLevel 1
  Abort
pathValid:
  StrCpy $PayloadFolder "${VERSION}"
FunctionEnd

Section "Uninstall"
  SetShellVarContext current
  ; Check only the small shipped payload, then remove unchanged owned wrappers.
  !insertmacro CheckCore changed
  StrCpy $0 1
  ExecWait '"$INSTDIR\xpotato-tunnel.exe" package-check' $0
  StrCmp $0 0 payloadValid
  MessageBox MB_OK|MB_ICONSTOP "Payload changed. Files and registration preserved." /SD IDOK
  SetErrorLevel 1
  Abort
payloadValid:
  ReadRegStr $1 HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\XpotatoTunnel" "DisplayVersion"
  StrCmp $1 "${VERSION}" 0 removePayload
  StrCpy $0 1
  ExecWait '"$INSTDIR\xpotato-tunnel.exe" uninstall-entrypoints' $0
  StrCmp $0 0 removePayload
  MessageBox MB_OK|MB_ICONSTOP "Command entrypoints could not be removed safely. Profiles, credentials and files preserved." /SD IDOK
  SetErrorLevel 1
  Abort
removePayload:
  ClearErrors
  Delete "$INSTDIR\xpotato-tunnel.exe"
  IfErrors blocked
  !include "uninstall-files.nsh"
  ; Only this version's registration is removed. Other versions may still be
  ; used by running processes or a rollback and are never swept recursively.
  StrCmp $1 "${VERSION}" 0 keepRegistration
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\XpotatoTunnel"
  Delete "$SMPROGRAMS\Xpotato Tunnel\Xpotato Tunnel.lnk"
  RMDir "$SMPROGRAMS\Xpotato Tunnel"
keepRegistration:
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"
  MessageBox MB_OK "Application entrypoints removed. Profiles, saved keys and runtime generations retained. Existing processes were not stopped." /SD IDOK
  Goto finished
blocked:
  MessageBox MB_OK|MB_ICONSTOP "Application file is in use or unavailable. No process was stopped; registration retained." /SD IDOK
  SetErrorLevel 1
  Abort
changed:
  MessageBox MB_OK|MB_ICONSTOP "Executable differs from the installed package. Nothing invoked or removed." /SD IDOK
  SetErrorLevel 1
  Abort
finished:
SectionEnd
