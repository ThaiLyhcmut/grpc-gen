PROTOC = protoc --go_out=. --go_opt=paths=source_relative \
               --go-grpc_out=. --go-grpc_opt=paths=source_relative
GEN_BIN = ./gen_skeleton
GEN = $(GEN_BIN)

# Build code generator binary
gen-tool:
	cd scripts && go build -o ../$(GEN_BIN) gen_skeleton.go

# Service port mappings
# Add your services here with their ports

# Proto generation
proto-common:
	$(PROTOC) proto/common/common.proto


proto-user:
	$(PROTOC) proto/user/user.proto

# Generate all protos
all: proto-common proto-user


gen-user: gen-tool proto-user
	$(GEN) user UserService 50051

# Generate all service skeletons
gen-all: gen-tool all gen-user

# Clean generated files
clean:
	rm -rf src/service/*
	rm -f gen_skeleton

# Clean everything including proto generated files
clean-all: clean
	find proto -name "*.pb.go" -delete

.PHONY: gen-tool proto-common all gen-all clean clean-all
