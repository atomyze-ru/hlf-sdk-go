PROTO_PACKAGES_PROTO := proto

proto: clean
	@for pkg in $(PROTO_PACKAGES_PROTO) ;do echo $$pkg && buf generate --template buf.gen.proto.yaml $$pkg -o ./$$(echo $$pkg | cut -d "/" -f1); done
	@go fmt ./...

clean:
	@for pkg in $(PROTO_PACKAGES_PROTO); do find $$pkg \( -name '*.pb.go' \) -delete;done

test:
	go test -mod=vendor ./...
