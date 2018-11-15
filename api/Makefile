all: push

# See pod.yaml for the version currently running-- bump this ahead before rebuilding!
# TAG = 0.0
TAG = latest

api: api.go
	# CGO_ENABLED=0 GOOS=linux GOARCH=amd64 godep go build -a -installsuffix cgo -ldflags '-w' -o api
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -ldflags '-w' -o api

container: api
	docker build --pull -t 661058921700.dkr.ecr.us-east-1.amazonaws.com/vegeta-api:$(TAG) .

push: container
	gcloud docker -- push 661058921700.dkr.ecr.us-east-1.amazonaws.com/vegeta-api:$(TAG)

clean:
	rm -f api
