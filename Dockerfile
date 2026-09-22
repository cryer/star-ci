FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN go build -o /out/star-ci ./cmd/star-ci

FROM alpine
COPY --from=build /out/star-ci /usr/local/bin/star-ci
ENTRYPOINT ["star-ci"]
