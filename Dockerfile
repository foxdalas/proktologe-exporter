FROM golang:alpine as build

WORKDIR $GOPATH/src/github.com/foxdalas/proktologe-exporter
COPY . .

RUN apk --no-cache add git
RUN go get -u github.com/golang/dep/cmd/dep
RUN dep check || dep ensure --vendor-only -v
RUN go build -o /go/bin/proktologe-exporter .

FROM alpine:3.8
RUN apk --no-cache add ca-certificates git
COPY --from=build /go/bin/proktologe-exporter /bin/

EXPOSE 8080

ENTRYPOINT ["proktologe-exporter"]