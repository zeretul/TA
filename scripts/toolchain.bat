@echo off
REM Local toolchain bootstrap for the Text Adventure build.
REM Source this from other scripts: call "%~dp0scripts\toolchain.bat"
REM
REM Discovery order:
REM   1. TA_GO_ROOT, when set, so a caller can pin one toolchain.
REM   2. The toolchain staged inside this package (see the note below).
REM   3. TA_DEV_TOOLS\go, the machine-wide tool root (see the note below).
REM   4. A go already on PATH (a normal system-wide install).
REM   5. The usual Windows install locations.
REM
REM On where the staged toolchain lives: this package keeps Go in
REM   <package>\.ta-toolchain\go
REM next to the other .ta-* runtime directories, which .gitignore already
REM excludes. That location is deliberate: it keeps the build reproducible on a
REM machine that has no Go at all, without depending on a user-profile path
REM that a different machine, a different user account or a cleaned temp
REM directory would not have. The old hardcoded %LOCALAPPDATA% path is still
REM probed below so an existing setup keeps working.
REM
REM Keep this file ASCII-only. Do not introduce parentheses inside an if or for
REM block: cmd ends the block at the first unescaped ")", and an expanded
REM variable such as ProgramFiles(x86) supplies exactly that character. The
REM probes below are therefore written as sequential ifs with no enclosing
REM block.

if defined TA_GO_ROOT goto pinned

REM %~dp0 ends with a backslash, so %~dp0.. is the package root.
set "TA_GO_CANDIDATE=%~dp0..\.ta-toolchain\go"
if exist "%TA_GO_CANDIDATE%\bin\go.exe" goto found

REM A machine-wide tool root keeps Go, the caches and the temp files off
REM the user profile and off the checkout. TA_DEV_TOOLS is optional.
if not defined TA_DEV_TOOLS goto no_dev_tools
set "TA_GO_CANDIDATE=%TA_DEV_TOOLS%\go"
if exist "%TA_GO_CANDIDATE%\bin\go.exe" goto found
:no_dev_tools

set "TA_GO_CANDIDATE=%LOCALAPPDATA%\denova-toolchain\go"
if exist "%TA_GO_CANDIDATE%\bin\go.exe" goto found

REM A system-wide Go works too; the build only needs version 1.26.6 or newer.
where go >nul 2>nul
if not errorlevel 1 goto onpath

set "TA_GO_CANDIDATE=%ProgramFiles%\Go"
if exist "%TA_GO_CANDIDATE%\bin\go.exe" goto found

set "TA_GO_CANDIDATE=%LOCALAPPDATA%\Programs\Go"
if exist "%TA_GO_CANDIDATE%\bin\go.exe" goto found

set "TA_GO_CANDIDATE=C:\Go"
if exist "%TA_GO_CANDIDATE%\bin\go.exe" goto found

goto missing

:found
set "TA_GO_ROOT=%TA_GO_CANDIDATE%"
set "TA_GO_CANDIDATE="
goto ready

:pinned
if exist "%TA_GO_ROOT%\bin\go.exe" goto ready
echo [toolchain] TA_GO_ROOT is set to %TA_GO_ROOT%
echo [toolchain] but %TA_GO_ROOT%\bin\go.exe does not exist.
goto missing

REM A PATH install already resolves go.exe, so PATH needs no change; only
REM GOTOOLCHAIN is pinned so a stale directive cannot trigger a download.
:onpath
set "GOTOOLCHAIN=local"
exit /b 0

:ready
set "PATH=%TA_GO_ROOT%\bin;%PATH%"
set "GOTOOLCHAIN=local"
exit /b 0

:missing
echo [toolchain] No Go toolchain found, so the backend cannot be rebuilt.
echo [toolchain] Looked at:
echo [toolchain]   TA_GO_ROOT
echo [toolchain]   %~dp0..\.ta-toolchain\go   (this package)
echo [toolchain]   %TA_DEV_TOOLS%\go   (when TA_DEV_TOOLS is set)
echo [toolchain]   PATH, %ProgramFiles%\Go, %LOCALAPPDATA%\Programs\Go, C:\Go
echo [toolchain]
echo [toolchain] The package normally ships with a staged toolchain in
echo [toolchain] .ta-toolchain. If that directory is missing, unpack a Go
echo [toolchain] 1.26.6+ toolchain into .ta-toolchain\go (so that
echo [toolchain] .ta-toolchain\go\bin\go.exe exists) and run this again.
echo [toolchain] A system-wide Go 1.26.6+ install also works.
exit /b 1
