echo "Generating protobuf and gRPC code..."

if ! command -v protoc &> /dev/null; then
    echo "❌ protoc not found!"
    echo "Please install protoc first:"
    echo "  - macOS: brew install protobuf"
    echo "  - Ubuntu: apt install protobuf-compiler"
    echo "  - Download: https://github.com/protocolbuffers/protobuf/releases"
    exit 1
fi
echo "✅ protoc: $(which protoc)"

if ! command -v protoc-gen-go &> /dev/null; then
    echo ""
    echo "❌ protoc-gen-go not found!"
    echo "Please install it:"
    echo "  go install google.golang.org/protobuf/cmd/protoc-gen-go@latest"
    echo ""
    echo "Or add Go bin to PATH:"
    echo "  export PATH=\$PATH:$(go env GOPATH)/bin"
    exit 1
fi
echo "✅ protoc-gen-go: $(which protoc-gen-go)"

if ! command -v protoc-gen-go-grpc &> /dev/null; then
    echo ""
    echo "❌ protoc-gen-go-grpc not found!"
    echo "Please install it:"
    echo "  go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest"
    echo ""
    echo "Or add Go bin to PATH:"
    echo "  export PATH=\$PATH:$(go env GOPATH)/bin"
    exit 1
fi
echo "✅ protoc-gen-go-grpc: $(which protoc-gen-go-grpc)"

mkdir -p internal/common

protoc --go_out=internal/common \
       --go-grpc_out=internal/common \
       internal/proto/ingestion.proto

if [ $? -eq 0 ]; then
    echo "✅ Generation successful!"
    echo "Generated files:"
    echo "  - internal/common/ingestion.pb.go"
    echo "  - internal/common/ingestion_grpc.pb.go"
else
    echo "❌ Generation failed!"
    exit 1
fi
