FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN go build -o /out/api ./cmd/api

FROM alpine:3.20
COPY --from=build /out/api /usr/local/bin/redcart-api
EXPOSE 8080
CMD ["redcart-api"]
