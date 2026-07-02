docker rm -f alloy-extract
docker rmi grafana/alloy:custom
docker buildx build --platform linux/amd64 -t grafana/alloy:custom .
docker create --name alloy-extract grafana/alloy:custom
docker cp alloy-extract:/bin/alloy ~/Downloads/alloy-linux-amd64
