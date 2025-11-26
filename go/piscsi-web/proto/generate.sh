#!/usr/bin/env bash
# Generate Go protobuf bindings from piscsi_interface.proto

set -euo pipefail

PROTO_FILE="piscsi_interface.proto"
OUT_DIR="."

echo "🔧 Generating Go protobuf bindings..."

# Generate Go bindings
# Specify the Go package mapping to avoid modifying the original proto file
protoc \
  --go_out=${OUT_DIR} \
  --go_opt=paths=source_relative \
  --go_opt=Mpiscsi_interface.proto=github.com/piscsi/piscsi-web/proto \
  ${PROTO_FILE}

echo "✅ Generated Go protobuf bindings in ${OUT_DIR}"
echo "   Output file: piscsi_interface.pb.go"
