# Build feature store
FROM golang:1.19-buster
WORKDIR /build

COPY . ./

RUN cd examples && \
    go mod download && \
    go install github.com/githubnemo/CompileDaemon@v1.3.0

ENTRYPOINT CompileDaemon -polling=true -build="go build ./examples/example.go" -command="./example"
