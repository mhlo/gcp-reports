from golang:1.8


ADD . /go/src/github.com/mhlo/gcp-reports
RUN go install github.com/mhlo/gcp-reports
ENTRYPOINT ["/go/bin/gcp-reports"]
