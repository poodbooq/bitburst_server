FROM golang:alpine AS builder

ENV GO111MODULE=on \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

RUN mkdir /app
ADD . /app
WORKDIR /app
RUN go mod download
RUN go build -o server .

FROM scratch

COPY --from=builder /app/server /
EXPOSE 9090
ENTRYPOINT [ "/server" ]