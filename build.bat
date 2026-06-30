@echo off
REM ============================================================
REM Build script - Compila o cliente tunnel para Windows com garble
REM Requer Go 1.23+ instalado no PATH
REM ============================================================

setlocal

echo [*] Verificando Go...
where go >nul 2>&1
if %ERRORLEVEL% neq 0 (
    echo [!] Go nao encontrado. Instale de https://go.dev/dl/
    exit /b 1
)

for /f "tokens=3" %%v in ('go version') do set GOVER=%%v
echo [*] Go versao: %GOVER%

echo [*] Instalando/atualizando garble...
go install mvdan.cc/garble@latest
if %ERRORLEVEL% neq 0 (
    echo [!] Falha ao instalar garble
    exit /b 1
)

echo [*] Compilando com garble (ofuscado, sem simbolos)...
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=amd64

if not exist build mkdir build

garble -literals -tiny build -trimpath -ldflags="-s -w" -o build\tunnel-windows-amd64.exe .

if %ERRORLEVEL% equ 0 (
    echo.
    echo [OK] Binario gerado: build\tunnel-windows-amd64.exe
    for %%A in (build\tunnel-windows-amd64.exe) do echo [*] Tamanho: %%~zA bytes
) else (
    echo [!] Falha na compilacao
    exit /b 1
)

endlocal
