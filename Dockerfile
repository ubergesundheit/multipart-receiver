FROM golang:1.18-alpine3.16 AS build

WORKDIR /app

COPY . .

RUN ./build.sh

FROM alpine:3.16

RUN addgroup -g 101 -S multipart-receiver && \
  adduser -u 101 -S multipart-receiver -G multipart-receiver

COPY --from=build /app/multipart-receiver /usr/local/bin/multipart-receiver

USER multipart-receiver

CMD ["/usr/local/bin/multipart-receiver"]
