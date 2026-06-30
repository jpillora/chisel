@echo off
REM ============================================================
REM Build script - Compila o cliente tunnel para Windows com garble
REM Requer Go 1.23+ instalado no PATH
REM
REM Tecnicas de evasao aplicadas:
REM   1. garble -literals     : ofusca strings/literais
REM   2. garble -tiny          : remove debug info, encurta simbolos
REM   3. garble -seed=random   : seed aleatoria (binario unico a cada build)
REM   4. GOGARBLE=*            : ofusca TODOS os pacotes (inclui dependencias)
REM   5. goversioninfo         : adiciona VERSIONINFO ao PE
REM   6. -ldflags="-s -w"      : strip de simbolos e DWARF
REM   7. -buildid=             : remove build ID do linker
REM   8. -trimpath             : remove paths do filesystem
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

echo [*] Instalando/atualizando dependencias...
go install mvdan.cc/garble@latest
if %ERRORLEVEL% neq 0 (
    echo [!] Falha ao instalar garble
    exit /b 1
)
go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest
if %ERRORLEVEL% neq 0 (
    echo [!] Falha ao instalar goversioninfo
    exit /b 1
)

echo [*] Gerando metadados VERSIONINFO do Windows...
goversioninfo -64 -o resource.syso versioninfo.json

echo [*] Compilando com garble (ofuscacao total + seed aleatoria)...
set GOGARBLE=*
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=amd64

if not exist build mkdir build

garble -literals -tiny -seed=random build -trimpath -ldflags="-s -w -buildid=" -o build\tunnel-windows-amd64.exe .

if %ERRORLEVEL% equ 0 (
    echo.
    echo [OK] Binario gerado: build\tunnel-windows-amd64.exe
    for %%A in (build\tunnel-windows-amd64.exe) do echo [*] Tamanho: %%~zA bytes

    REM UPX packing opcional
    where upx >nul 2>&1
    if %ERRORLEVEL% equ 0 (
        echo [*] Aplicando UPX (compressao LZMA)...
        copy /y build\tunnel-windows-amd64.exe build\tunnel-windows-amd64-packed.exe >nul
        upx --best --lzma --force build\tunnel-windows-amd64-packed.exe
        for %%A in (build\tunnel-windows-amd64-packed.exe) do echo [*] Binario packed: build\tunnel-windows-amd64-packed.exe (%%~zA bytes)
    ) else (
        echo [!] UPX nao encontrado (opcional). Instale de https://upx.github.io/
    )
) else (
    echo [!] Falha na compilacao
    exit /b 1
)

endlocal
