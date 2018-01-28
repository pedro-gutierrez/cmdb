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
$ curl -X POST http://localhost:7801/keys/foo --data "1"
```

Get a single value for a key:

```
$ curl http://localhost:7801/keys/foo

1
```

Set multiple values for the same key

```
$ curl -X POST http://localhost:7801/keys/foo --data "2"
$ curl -X POST http://localhost:7801/keys/foo --data "3"
$ curl -X POST http://localhost:7801/keys/foo --data "4"

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

Do a backup to S3:

```
$ curl -X POST http://localhost:7801/backups

{"name":"20180126174859","size":502}
```

Restore a backup from S3:

```
$ curl -X POST http://localhost:7801/restore/20180126174859

{ "size": 502}
```
