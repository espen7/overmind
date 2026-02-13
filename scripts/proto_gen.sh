#!/bin/bash
# scripts/proto_gen.sh

# Ensure the output directory exists
mkdir -p pkg/pb/kit
mkdir -p pkg/pb/gateway

# Generate Proto files
# Use M flag to rewrite imports if needed, but since we use go_package, it should be fine.
# Let's try explicit output path relative to root.

# If we use paths=source_relative, then the output file path is determined by the input file path relative to the include path.
# input: api/proto/kit/envelope.proto
# include: .
# relative: api/proto/kit/envelope.proto
# output: api/proto/kit/envelope.pb.go
# This puts it in api/proto/kit.

# We want it in pkg/pb/kit.
# So we either copy it, or we construct the protoc call differently.

# Let's try the simple approach: Generate into source dirs, then move.
protoc -I=. \
       --go_out=. --go_opt=paths=source_relative \
       api/proto/kit/*.proto \
       api/proto/gateway/*.proto

# Move generated files to pkg/pb
# api/proto/kit/*.pb.go -> pkg/pb/kit/
# api/proto/gateway/*.pb.go -> pkg/pb/gateway/

mv api/proto/kit/*.pb.go pkg/pb/kit/
mv api/proto/gateway/*.pb.go pkg/pb/gateway/

echo "Protobuf generation complete (with move)."
