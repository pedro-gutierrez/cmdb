# cmdb
A simple KV store written in Go that uses LMDB as a backend

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
  pedro-gutierrez/cmdb:latest
```

## Usage

Set a value for key:

```
$ curl -X POST http://localhost:7801/keys/foo --data "bar"
```

Get a key value:

```
$ curl http://localhost:7801/keys/foo

bar
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
