FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod main.go ./
RUN mkdir -p /out && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/batch .

FROM gcr.io/distroless/static-debian12:nonroot
ENV BATCH_MEMORY_MB=128
COPY --from=build /out/batch /batch
ENTRYPOINT ["/batch"]
