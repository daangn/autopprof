# Optional: build a self-contained image if you don't want the
# volume-mount dance in run.sh.
#
# Build context must be the *repo root* so the v2 replace directive
# in examples/go.mod (replace github.com/daangn/autopprof/v2 => ../)
# resolves.
#
#   docker build -t autopprof-e2e -f examples/e2e/Dockerfile .
#   docker run --rm --cpus=1 -m=512m \
#       -e SLACK_TOKEN -e SLACK_CHANNEL_ID -e SLACK_APP -e E2E_DURATION \
#       autopprof-e2e

FROM golang:1.22-bookworm AS build
WORKDIR /src
COPY . .
RUN cd examples && CGO_ENABLED=0 go build -trimpath -o /out/e2e ./e2e

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/e2e /e2e
ENTRYPOINT ["/e2e"]
