FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/oma ./cmd/app && CGO_ENABLED=0 go build -trimpath -o /out/mockoma ./cmd/mockoma
FROM alpine:3.22
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 oma && mkdir -p /data/media && chown -R oma:oma /data
WORKDIR /app
COPY --from=build /out/oma /app/oma
COPY --from=build /out/mockoma /app/mockoma
COPY --from=build /src/oma-site/data /app/oma-site/data
COPY --from=build /src/oma-site/site /app/oma-site/site
USER oma
EXPOSE 8080
ENTRYPOINT ["/app/oma"]
