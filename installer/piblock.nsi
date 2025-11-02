!define PRODUCT_NAME "PiBlock"
!define PRODUCT_VERSION "1.0.0"
!define COMPANY_NAME "OmgRod"

OutFile "piblock-installer.exe"
InstallDir "$PROGRAMFILES\\PiBlock"
RequestExecutionLevel admin

Page directory
Page instfiles

Section "Install"
  SetOutPath "$INSTDIR"
  ; Copy all files from the portable folder (the NSIS build step should run in repo root)
  File /r "release\\piblock-portable\\*"

  ; Create uninstall entry
  WriteUninstaller "$INSTDIR\\uninstall.exe"

  ; Add to Add/Remove Programs
  WriteRegStr HKLM "Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\${PRODUCT_NAME}" "DisplayName" "${PRODUCT_NAME} ${PRODUCT_VERSION}"
  WriteRegStr HKLM "Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\${PRODUCT_NAME}" "UninstallString" "$INSTDIR\\uninstall.exe"
  WriteRegStr HKLM "Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\${PRODUCT_NAME}" "Publisher" "${COMPANY_NAME}"
  WriteRegDWORD HKLM "Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\${PRODUCT_NAME}" "NoRepair" 1
  WriteRegDWORD HKLM "Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\${PRODUCT_NAME}" "NoModify" 1

  ; Create a simple registry key with installed path
  WriteRegStr HKLM "Software\\PiBlock" "InstallPath" "$INSTDIR"

  ; Optionally create a Start Menu shortcut
  CreateDirectory "$SMPROGRAMS\\PiBlock"
  CreateShortCut "$SMPROGRAMS\\PiBlock\\PiBlock.lnk" "$INSTDIR\\piblock.exe"
SectionEnd

Section "Uninstall"
  Delete "$INSTDIR\\piblock.exe"
  Delete "$INSTDIR\\server.js"
  RMDir /r "$INSTDIR\\dist"
  DeleteRegKey HKLM "Software\\PiBlock"
  DeleteRegKey HKLM "Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\${PRODUCT_NAME}"
  Delete "$SMPROGRAMS\\PiBlock\\PiBlock.lnk"
  RMDir "$SMPROGRAMS\\PiBlock"
  ; remove all files
  RMDir /r "$INSTDIR"
SectionEnd
