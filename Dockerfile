FROM golang:1.25 AS build-stage

WORKDIR /app

ADD application ./
RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -o /league-buddy ./cmd/web

FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/

COPY --from=build-stage /league-buddy .
COPY --from=build-stage /app/sql ./sql

EXPOSE 8080

ENTRYPOINT ["/root/league-buddy"]
