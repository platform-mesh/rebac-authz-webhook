FROM golang:1.25 AS builder

ENV GOSUMDB=off

RUN git config --global credential.helper store
RUN --mount=type=secret,id=org_token echo "https://gha:$(cat /run/secrets/org_token)@github.com" > /root/.git-credentials

WORKDIR /app

COPY go.mod go.mod
COPY go.sum go.sum

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o rebac-authz-webhook main.go

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /app/rebac-authz-webhook /app/rebac-authz-webhook
USER 65532:65532

ENTRYPOINT ["/app/rebac-authz-webhook"]
