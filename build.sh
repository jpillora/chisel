#!/usr/bin/env bash
# ============================================================
# Build script - Compila o cliente tunnel para Windows com garble
# Requer Go 1.23+ instalado no PATH
#
# Tecnicas de evasao aplicadas:
#   1. garble -literals     : ofusca strings/literais
#   2. garble -tiny          : remove debug info, encurta simbolos
#   3. garble -seed=random   : seed aleatoria (binario unico a cada build)
#   4. GOGARBLE=*            : ofusca TODOS os pacotes (inclui dependencias)
#   5. goversioninfo         : adiciona VERSIONINFO ao PE (binarios sem metadata sao suspeitos)
#   6. -ldflags="-s -w"      : strip de simbolos e DWARF
#   7. -buildid=             : remove build ID do linker
#   8. -trimpath             : remove paths do filesystem
# ============================================================

set -euo pipefail

echo "[*] Verificando Go..."
if ! command -v go &>/dev/null; then
    echo "[!] Go nao encontrado. Instale de https://go.dev/dl/"
    exit 1
fi

GOVER=$(go version | awk '{print $3}')
echo "[*] Go versao: $GOVER"

echo "[*] Instalando/atualizando dependencias..."
go install mvdan.cc/garble@latest
go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest

echo "[*] Gerando metadados VERSIONINFO do Windows..."
goversioninfo -64 -o resource.syso versioninfo.json

echo "[*] Compilando com garble (ofuscacao total + seed aleatoria)..."
export GOGARBLE=*
export CGO_ENABLED=0
export GOOS=windows
export GOARCH=amd64

mkdir -p build

garble -literals -tiny -seed=random build \
    -trimpath \
    -ldflags="-s -w -buildid=" \
    -o build/tunnel-windows-amd64.exe .

if [ -f build/tunnel-windows-amd64.exe ]; then
    SIZE=$(stat -f%z build/tunnel-windows-amd64.exe 2>/dev/null || stat -c%s build/tunnel-windows-amd64.exe 2>/dev/null)
    echo ""
    echo "[OK] Binario gerado: build/tunnel-windows-amd64.exe"
    echo "[*] Tamanho: $SIZE bytes"

    # UPX packing (opcional, muda completamente a assinatura do binario)
    if command -v upx &>/dev/null; then
        echo "[*] Aplicando UPX (compressao LZMA)..."
        cp build/tunnel-windows-amd64.exe build/tunnel-windows-amd64-packed.exe
        upx --best --lzma --force build/tunnel-windows-amd64-packed.exe
        PACKED_SIZE=$(stat -f%z build/tunnel-windows-amd64-packed.exe 2>/dev/null || stat -c%s build/tunnel-windows-amd64-packed.exe 2>/dev/null)
        echo "[*] Binario packed: build/tunnel-windows-amd64-packed.exe ($PACKED_SIZE bytes)"
    else
        echo "[!] UPX nao encontrado (opcional). Instale com: apt install upx-ucl"
    fi
else
    echo "[!] Falha na compilacao"
    exit 1
fi
