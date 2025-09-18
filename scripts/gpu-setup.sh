#!/bin/bash

# GPU Support Script for Manticore Search
# This script helps you run Manticore Search with GPU acceleration

set -e

echo "🚀 Manticore Search GPU Support Setup"
echo "======================================"

# Check if NVIDIA Docker runtime is available
check_nvidia_docker() {
    if command -v nvidia-docker >/dev/null 2>&1; then
        echo "✅ nvidia-docker found"
        return 0
    elif docker info 2>/dev/null | grep -q nvidia; then
        echo "✅ NVIDIA Docker runtime found"
        return 0
    else
        echo "❌ NVIDIA Docker runtime not found"
        return 1
    fi
}

# Check if GPU is available
check_gpu() {
    if command -v nvidia-smi >/dev/null 2>&1; then
        echo "🔍 Checking GPU availability..."
        nvidia-smi --query-gpu=name,memory.total --format=csv,noheader
        return 0
    else
        echo "❌ nvidia-smi not found - GPU may not be available"
        return 1
    fi
}

# Main execution
main() {
    echo "Checking system requirements..."
    
    if check_gpu; then
        echo "✅ GPU detected"
        
        if check_nvidia_docker; then
            echo "✅ All requirements met for GPU acceleration"
            echo ""
            echo "🚀 Starting services with GPU support..."
            docker-compose -f docker-compose.yml -f docker-compose.gpu.yml up --build
        else
            echo "⚠️  NVIDIA Docker runtime not available"
            echo "📖 Please install nvidia-docker2 or nvidia-container-toolkit"
            echo "🔗 See: https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html"
            echo ""
            echo "💡 Starting without GPU acceleration..."
            docker-compose up --build
        fi
    else
        echo "ℹ️  No GPU detected, starting without GPU acceleration..."
        docker-compose up --build
    fi
}

# Handle script arguments
case "${1:-}" in
    --force-gpu)
        echo "🔧 Force GPU mode enabled"
        docker-compose -f docker-compose.yml -f docker-compose.gpu.yml up --build
        ;;
    --no-gpu)
        echo "🔧 GPU disabled by user"
        docker-compose up --build
        ;;
    --check)
        check_gpu
        check_nvidia_docker
        ;;
    --help|-h)
        echo "Usage: $0 [OPTIONS]"
        echo ""
        echo "Options:"
        echo "  --force-gpu    Force GPU mode even if checks fail"
        echo "  --no-gpu       Disable GPU acceleration"
        echo "  --check        Check GPU and Docker requirements only"
        echo "  --help, -h     Show this help message"
        echo ""
        echo "GPU acceleration can significantly speed up Auto Embeddings:"
        echo "  • Faster vector generation (2-10x speedup typical)"
        echo "  • Reduced CPU load"
        echo "  • Better performance for large document sets"
        ;;
    *)
        main
        ;;
esac