FROM golang:latest AS golang
ENV GOPATH /go
WORKDIR /go/src/github.com/cisco-sso/pvwatch
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o pvwatch .

FROM scratch
COPY --from=golang /go/src/github.com/cisco-sso/pvwatch/pvwatch /
EXPOSE 8080
ENTRYPOINT ["/pvwatch"]
