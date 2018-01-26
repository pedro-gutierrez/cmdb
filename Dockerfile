FROM golang:1.9
WORKDIR /go/src/app
COPY cmdb.go .
RUN go get github.com/aws/aws-sdk-go && go get github.com/kataras/iris && go get github.com/mholt/archiver && go get github.com/bmatsuo/lmdb-go/lmdb
RUN go build cmdb.go
CMD ["./cmdb"]
