@echo off
REM Wrapper to run the PowerShell build helper. Usage:
REM   build_windows.cmd          -> dev build
REM   build_windows.cmd release  -> release build

SET SCRIPT_DIR=%~dp0
IF "%1"=="release" (
  powershell -NoProfile -ExecutionPolicy Bypass -File "%SCRIPT_DIR%build_windows.ps1" -Release
) ELSE (
  powershell -NoProfile -ExecutionPolicy Bypass -File "%SCRIPT_DIR%build_windows.ps1"
)
