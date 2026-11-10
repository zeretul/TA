@echo off
setlocal EnableExtensions
cd /d "%~dp0game"
set "TA_DIR=%~dp0dev-data"
set "TA_BACKEND_PORT=19921"
set "DENOVA_DIR="
set "DENOVA_BACKEND_PORT="
set "TA_WORKSPACE="
set "DENOVA_WORKSPACE="
set "OPENAI_BASE_URL="
set "OPENAI_MODEL="
set "OPENAI_API_KEY="
TA.exe %*
