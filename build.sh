#!/usr/bin/env bash
# ============================================================
# Build script - Compila o cliente tunnel para Windows com garble
# Requer Go 1.23+ instalado no PATH
# ============================================================

set -euo pipefail

echo "[*] Verificando Go..."
if ! command -v go &>/dev/null; then
    echo "[!] Go nao encontrado. Instale de https://go.dev/dl/"
    exit 1
fi

GOVER=$(go version | awk '{print $3}')
echo "[*] Go versao: $GOVER"

echo "[*] Instalando/atualizando garble..."
go install mvdan.cc/garble@latest

echo "[*] Compilando com garble (ofuscado, sem simbolos)..."
export CGO_ENABLED=0
export GOOS=windows
export GOARCH=amd64

mkdir -p build

# garble flags:
#   -literals  : ofusca strings e literais no codigo (evita deteccao heuristicas)
#   -tiny      : remove nomes de debug e encurta nomes de funcoes
garble -literals -tiny build -trimpath -ldflags="-s -w" -o build/tunnel-windows-amd64.exe .

if [ -f build/tunnel-windows-amd64.exe ]; then
    SIZE=$(stat -f%z build/tunnel-windows-amd64.exe 2>/dev/null || stat -c%s build/tunnel-windows-amd64.exe 2>/dev/null)
    echo ""
    echo "[OK] Binario gerado: build/tunnel-windows-amd64.exe"
    echo "[*] Tamanho: $SIZE bytes"
else
    echo "[!] Falha na compilacao"
    exit 1
fi
