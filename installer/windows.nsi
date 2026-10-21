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
!ifdef GUI_SHA
  !define ENTRY "xpotato-tunnel-app.exe"
  !define ENTRY_ARGS ""
!else
  !define ENTRY "xpotato-tunnel.exe"
  !define ENTRY_ARGS "maintenance"
!endif
!include "MUI2.nsh"
!include "x64.nsh"
!include "FileFunc.nsh"
!include "LogicLib.nsh"
; Attribute-only checks of fixed known parents, never a tree walk/hash pass.
; Missing directories permit a partial-uninstall retry; all other errors fail.
!macro CheckDirectory path failure
  System::Call 'kernel32::GetFileAttributesW(w "${path}") i.r2 ?e'
  Pop $3
  ${If} $2 == -1
    ${If} $3 != 2
    ${AndIf} $3 != 3
      Goto ${failure}
    ${EndIf}
  ${Else}
    IntOp $3 $2 & 0x400
    ${If} $3 != 0
      Goto ${failure}
    ${EndIf}
    IntOp $3 $2 & 0x10
    ${If} $3 == 0
      Goto ${failure}
    ${EndIf}
  ${EndIf}
!macroend
!macro CheckGUI failure
!ifdef GUI_SHA
  StrCpy $0 1
  ClearErrors
  ExecWait `"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -NonInteractive -Command "try { if (([BitConverter]::ToString([Security.Cryptography.SHA256]::Create().ComputeHash([IO.File]::ReadAllBytes([IO.Path]::Combine([Environment]::GetFolderPath('LocalApplicationData'),'Programs\XpotatoTunnel\$PayloadFolder\xpotato-tunnel-app.exe')))) -replace '-','') -eq '${GUI_SHA}') { exit 0 }; exit 1 } catch { exit 1 }"` $0
  IfErrors ${failure}
  StrCmp $0 0 0 ${failure}
!endif
!macroend
!macro CheckBase failure
  !insertmacro CheckDirectory "$LOCALAPPDATA" ${failure}
  !insertmacro CheckDirectory "$LOCALAPPDATA\Programs" ${failure}
  !insertmacro CheckDirectory "$LOCALAPPDATA\Programs\XpotatoTunnel" ${failure}
!macroend
!include "check-directories.nsh"
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
!define MUI_FINISHPAGE_RUN "$INSTDIR\${ENTRY}"
!define MUI_FINISHPAGE_RUN_PARAMETERS "${ENTRY_ARGS}"
!define MUI_FINISHPAGE_RUN_TEXT "Open Xpotato Tunnel"
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
  !insertmacro CheckBase finished
  !insertmacro CheckPayloadDirectories "$StagingDir" finished
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
  !insertmacro CheckBase invalid
  !insertmacro CheckPayloadDirectories "$INSTDIR" invalid
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
  ClearErrors
  !include "install-files.nsh"
  IfErrors invalid
  !insertmacro CheckCore invalid
  !insertmacro CheckGUI invalid
  StrCpy $0 1
  ; Extraction already checks container CRC; launch/version is the minimal
  ; post-extraction check. Do not reopen every doc/payload file here.
  ExecWait '"$StagingDir\xpotato-tunnel.exe" version' $0
  IfErrors invalid
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
  !insertmacro CheckGUI invalid
  StrCpy $0 1
  ExecWait '"$INSTDIR\xpotato-tunnel.exe" version' $0
  IfErrors invalid
  StrCmp $0 0 register invalid
register:
  !insertmacro CheckDirectory "$SMPROGRAMS" invalid
  !insertmacro CheckDirectory "$SMPROGRAMS\Xpotato Tunnel" invalid
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  CreateDirectory "$SMPROGRAMS\Xpotato Tunnel"
  CreateShortcut "$SMPROGRAMS\Xpotato Tunnel\Xpotato Tunnel.lnk" "$INSTDIR\${ENTRY}" "${ENTRY_ARGS}"
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
  !insertmacro CheckBase changed
  !insertmacro CheckPayloadDirectories "$INSTDIR" changed
  !insertmacro CheckDirectory "$SMPROGRAMS" changed
  !insertmacro CheckDirectory "$SMPROGRAMS\Xpotato Tunnel" changed
  ; Authenticate/launch only the helper, then remove unchanged owned wrappers.
  ; Do not require all docs to remain present after a partial uninstall.
  !insertmacro CheckCore changed
  StrCpy $0 1
  ExecWait '"$INSTDIR\xpotato-tunnel.exe" version' $0
  IfErrors changed
  StrCmp $0 0 payloadValid
  MessageBox MB_OK|MB_ICONSTOP "Payload changed. Files and registration preserved." /SD IDOK
  SetErrorLevel 1
  Abort
payloadValid:
  ReadRegStr $1 HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\XpotatoTunnel" "DisplayVersion"
  StrCmp $1 "${VERSION}" 0 removePayload
  StrCpy $0 1
  ExecWait '"$INSTDIR\xpotato-tunnel.exe" uninstall-entrypoints' $0
  IfErrors changed
  StrCmp $0 0 removePayload
  MessageBox MB_OK|MB_ICONSTOP "Command entrypoints could not be removed safely. Profiles, credentials and files preserved." /SD IDOK
  SetErrorLevel 1
  Abort
removePayload:
  !include "uninstall-files.nsh"
  ClearErrors
  Delete "$INSTDIR\xpotato-tunnel.exe"
  IfErrors blocked
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
