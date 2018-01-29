# CMDB

* Simple KV store 
* JSON in, JSON out
* Written in Go
* LMDB backend.
* Backup/Restore to/from S3

## Getting started 

```
docker run 
  -e "AWS_ACCESS_KEY_ID=xxx" \
  -e "AWS_ACCESS_KEY_ID=yyy" \ 
  -e "AWS_REGION=zzz" \
  -e "AWS_BUCKET=backups" \
  -e "CMDB_MAP_SIZE=10737418240" \
  -e "CMDB_NAME=countries" \
  -e "CMDB_PORT=7801" \
  -e "CMDB_DATA=/data" \
  -v "/tmp/data:/data" \ 
  -n cmdb \
  -p 7801:7801 \
  pedrogutierrez/cmdb:latest
```

## Usage

Set a value for key:

```
$ curl -X POST http://localhost:7801/foo --data "1"
```

Get a single value for a key:

```
$ curl http://localhost:7801/foo

1
```

Set multiple values for the same key

```
$ curl -X POST http://localhost:7801/foo --data "2"
$ curl -X POST http://localhost:7801/foo --data "3"
$ curl -X POST http://localhost:7801/foo --data "4"

```

Get multiple values for the same key

```
$ curl http://localhost:7801/keys/foo?count=2

[1,2]
```

```
$ curl http://localhost:7801/keys/foo?count=2&skip=2

[3,4]
```

Unique keys:

```
$ curl -X POST -i http://localhost:7801/bar?unique=true --data "1"

HTTP/1.1 200 OK
Date: Mon, 29 Jan 2018 18:43:29 GMT
Content-Length: 0
Content-Type: text/plain; charset=utf-8

$ curl -X POST -i http://localhost:7801/bar?unique=true --data "2"

HTTP/1.1 409 Conflict
Date: Mon, 29 Jan 2018 18:44:09 GMT
Content-Length: 8
Content-Type: text/plain; charset=utf-8

Conflict
```

Do a backup to S3:

```
$ curl -X POST http://localhost:7801/backups/new

{"name":"20180126174859","size":502}
```

Restore a backup from S3:

```
$ curl -X POST http://localhost:7801/backups/20180126174859/restore

{ "size": 502}
```
