FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/vigil ./cmd/vigil

# Static binary on distroless: ~10 MB image, no shell, runs as nonroot.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/vigil /vigil
EXPOSE 8080
ENTRYPOINT ["/vigil"]
