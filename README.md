# cmdb
A simple KV store written in Go that uses LMDB as a backend

## Getting started 

```
docker run 
  --env "AWS_ACCESS_KEY_ID=xxx" \
  --env "AWS_ACCESS_KEY_ID=yyy" \ 
  --env "AWS_REGION=zzz" \
  --env "AWS_BUCKET=backups" \
  --env "CMDB_MAP_SIZE=10737418240" \
  --env "CMDB_NAME=countries" \
  --env "CMDB_PORT=7801" \
  --name cmdb \
  --port 7801:7801 \
  pedro-gutierrez/cmdb:latest
```

