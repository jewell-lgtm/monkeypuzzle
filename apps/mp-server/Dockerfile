# Build context is the repo root (see docker-compose.yml).
# syntax=docker/dockerfile:1
FROM golang:1.25.5 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/mp-server ./apps/mp-server

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/mp-server /usr/local/bin/mp-server
EXPOSE 8080
ENTRYPOINT ["mp-server"]
CMD ["serve"]
