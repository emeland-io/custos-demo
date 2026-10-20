# syntax=docker/dockerfile:1

FROM node:22-alpine AS frontend-build
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.27-alpine AS backend-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN rm -rf internal/webassets/dist && mkdir -p internal/webassets/dist
COPY --from=frontend-build /src/frontend/dist/ internal/webassets/dist/
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/notebook-server ./cmd/server

FROM gcr.io/distroless/static:nonroot AS runtime
COPY --from=backend-build /out/notebook-server /notebook-server
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/notebook-server"]
