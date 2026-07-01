docker buildx build --platform linux/amd64 -t grafana/alloy:custom .
docker rm -f alloy-extract
docker create --name alloy-extract grafana/alloy:custom
docker cp alloy-extract:/bin/alloy ~/Downloads/alloy-linux-amd64
