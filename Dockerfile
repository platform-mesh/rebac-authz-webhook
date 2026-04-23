FROM golang:1.26@sha256:1e598ea5752ae26c093b746fd73c5095af97d6f2d679c43e83e0eac484a33dc3 AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

COPY go.mod go.mod
COPY go.sum go.sum

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o rebac-authz-webhook main.go

FROM gcr.io/distroless/static:nonroot@sha256:e3f945647ffb95b5839c07038d64f9811adf17308b9121d8a2b87b6a22a80a39
WORKDIR /
COPY --from=builder /app/rebac-authz-webhook /app/rebac-authz-webhook
USER 65532:65532

ENTRYPOINT ["/app/rebac-authz-webhook"]
