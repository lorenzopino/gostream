#!/bin/bash
# Script di Setup IA per GoStream (Raspberry Pi 4)
# Configura llama.cpp e scarica Qwen2.5-0.5B

set -e

AI_DIR="/home/pi/GoStream/ai"
MODELS_DIR="$AI_DIR/models"
LLAMA_CPP_DIR="$AI_DIR/llama.cpp"
MODEL_URL="https://huggingface.co/Qwen/Qwen2.5-0.5B-Instruct-GGUF/resolve/main/qwen2.5-0.5b-instruct-q4_k_m.gguf"
MODEL_FILE="$MODELS_DIR/qwen2.5-0.5b-instruct-q4_k_m.gguf"

echo "--- [1/4] Installazione dipendenze di sistema ---"
sudo apt update && sudo apt install -y build-essential cmake git git-lfs htop wget

mkdir -p "$MODELS_DIR"

echo "--- [2/4] Preparazione llama.cpp (Cortex-A72 Optimization via CMake) ---"
if [ ! -d "$LLAMA_CPP_DIR" ]; then
    git clone https://github.com/ggerganov/llama.cpp "$LLAMA_CPP_DIR"
fi

cd "$LLAMA_CPP_DIR"
mkdir -p build
cd build
cmake ..
cmake --build . --config Release -j 4

echo "--- [3/4] Download Modello Qwen2.5-0.5B (GGUF Q4_K_M) ---"
if [ ! -f "$MODEL_FILE" ]; then
    wget -O "$MODEL_FILE" "$MODEL_URL"
else
    echo "Modello già presente: $MODEL_FILE"
fi

echo "--- [4/4] Test Rapido di Performance (2 Core) ---"
# Con CMake i binari sono in build/bin/
cd "$LLAMA_CPP_DIR/build/bin"
./llama-bench -m "$MODEL_FILE" -p 128 -n 64 -t 2

echo "-------------------------------------------------------"
echo "Setup Completato!"
echo "Usa questo comando per simulare un tweak di parametri:"
echo "cd $LLAMA_CPP_DIR/build/bin && ./llama-cli -m $MODEL_FILE -t 2 --temp 0.1 -n 64 -p \"<|im_start|>system\nSei un ottimizzatore per TorrServer. Rispondi JSON.<|im_end|>\n<|im_start|>user\nDL=2MB/s, CPU=90%, Buff=20%<|im_end|>\n<|im_start|>assistant\n\""
