FROM golang:1.22 AS build

WORKDIR /app

COPY go.mod ./
COPY main.go ./

RUN CGO_ENABLED=0 go build -o /out/myservice .

EXPOSE 8080

ENTRYPOINT ["/out/myservice"]
