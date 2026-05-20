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


proto-class:
	$(PROTOC) proto/class/class.proto


proto-lesson:
	$(PROTOC) proto/lesson/lesson.proto


proto-material:
	$(PROTOC) proto/material/material.proto


proto-exam:
	$(PROTOC) proto/exam/exam.proto


proto-submission:
	$(PROTOC) proto/submission/submission.proto


proto-notification:
	$(PROTOC) proto/notification/notification.proto


proto-audit:
	$(PROTOC) proto/audit/audit.proto

# Generate all protos
all: proto-common proto-audit proto-notification proto-submission proto-exam proto-material proto-lesson proto-class proto-user


gen-user: gen-tool proto-user
	$(GEN) user UserService 50051


gen-class: gen-tool proto-class
	$(GEN) class ClassService 50052


gen-lesson: gen-tool proto-lesson
	$(GEN) lesson LessonService 50053


gen-material: gen-tool proto-material
	$(GEN) material MaterialService 50054


gen-exam: gen-tool proto-exam
	$(GEN) exam ExamService 50055


gen-submission: gen-tool proto-submission
	$(GEN) submission SubmissionService 50056


gen-notification: gen-tool proto-notification
	$(GEN) notification NotificationService 50057


gen-audit: gen-tool proto-audit
	$(GEN) audit AuditService 50058

# Generate all service skeletons
gen-all: gen-tool all gen-audit gen-notification gen-submission gen-exam gen-material gen-lesson gen-class gen-user

# Clean generated files
clean:
	rm -rf src/service/*
	rm -f gen_skeleton

# Clean everything including proto generated files
clean-all: clean
	find proto -name "*.pb.go" -delete

.PHONY: gen-tool proto-common all gen-all clean clean-all
