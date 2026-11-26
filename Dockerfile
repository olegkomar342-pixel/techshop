# Go бэкенді + фронтенд бір кішкентай контейнерде
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/shop .

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/shop /app/shop
COPY frontend/ /app/frontend/
COPY database/ /app/database/
ENV PORT=3000 FRONTEND_DIR=/app/frontend SCHEMA_FILE=/app/database/01_schema.sql
EXPOSE 3000
USER nobody
CMD ["/app/shop"]
