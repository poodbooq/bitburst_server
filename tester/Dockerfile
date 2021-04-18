FROM golang:alpine AS builder

ENV GO111MODULE=off \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

RUN mkdir /app
ADD . /app
WORKDIR /app
RUN go build -o tester .

FROM scratch

COPY --from=builder /app/tester /
EXPOSE 9010
ENTRYPOINT [ "/tester" ]