FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/server ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /bin/server /bin/server
COPY migrations /migrations
EXPOSE 8080
ENTRYPOINT ["/bin/server"]
