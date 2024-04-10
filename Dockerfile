# builder
FROM golang:latest AS builder
RUN mkdir /app
ADD . /app/
WORKDIR /app

RUN go build -o /app/scribblers .
RUN go build -o /app/migrate /app/tools/migrate

# certificates are required in case the Go binary do HTTPS calls
# to read more about it: https://www.docker.com/blog/docker-golang/ "The special case of SSL certificates"
#FROM alpine:latest as certs
#RUN apk --no-cache add ca-certificates

# For future implementation of SSL certificate support
#COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# Selfcontained executable used as entrypoint
#COPY --from=builder /app/scribblers /scribblers
#COPY --from=builder /app/migrate /migrate
#COPY database/migrations /database/migrations

ENTRYPOINT ["/bin/sh", "-c", "/app/migrate up && /app/scribblers"]
