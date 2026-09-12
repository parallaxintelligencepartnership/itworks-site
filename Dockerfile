# syntax=docker/dockerfile:1

# Build stage: golang:1.26-alpine, pinned by index digest (linux/amd64 manifest present).
FROM golang:1.26-alpine@sha256:ce864e7223ac17b1775e6fd0b4c0db580c2eb50e7953a427916379e4b92a1628 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/itworks ./cmd/itworks

# This stage still has a shell, unlike the distroless final image, so it is
# where /data is created and owned by uid 65532 (distroless nonroot's uid).
# COPY --chown below applies that ownership into the final image so a fresh
# named volume mounted at /data is writable by the nonroot user.
RUN mkdir -p /out/data && chown 65532:65532 /out/data

# Final stage: distroless static, nonroot (runs as uid 65532 by default).
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=build /out/itworks /itworks
COPY --from=build --chown=65532:65532 /out/data /data

ENTRYPOINT ["/itworks"]
